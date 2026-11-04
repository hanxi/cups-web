package main

import (
	"context"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const systemCustomFontDir = "/usr/share/fonts/truetype/custom"

type customFontItem struct {
	Name        string `json:"name"`        // 物理文件名，如 "1.ttf"，用于保持删除等接口契约
	DisplayName string `json:"displayName"` // 友好展示名称，如 "Arial"、"仿宋 (FangSong)"
	Family      string `json:"family"`      // 字体族名
	Subfamily   string `json:"subfamily"`   // 样式/字重（如 Regular, Bold）
	Format      string `json:"format"`      // 格式（如 TTF, OTF, TTC）
	SizeBytes   int64  `json:"sizeBytes"`   // 文件大小
	UpdatedAt   string `json:"updatedAt"`   // 修改时间
}

type fontListResponse struct {
	FontDir             string           `json:"fontDir"`
	CustomFonts         []customFontItem `json:"customFonts"`
	TotalSystemFamilies int              `json:"totalSystemFamilies"`
}

func isAllowedFontExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".ttf" || ext == ".ttc" || ext == ".otf"
}

// adminListFontsHandler 获取自定义字体列表及系统字体信息
func adminListFontsHandler(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(fontDir)
	if err != nil && !os.IsNotExist(err) {
		writeJSONError(w, http.StatusInternalServerError, "failed to read font directory: "+err.Error())
		return
	}

	customFonts := make([]customFontItem, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !isAllowedFontExt(ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		filePath := filepath.Join(fontDir, entry.Name())
		meta, _ := ParseFontMetadataFromFile(filePath)

		customFonts = append(customFonts, customFontItem{
			Name:        entry.Name(),
			DisplayName: meta.DisplayName,
			Family:      meta.Family,
			Subfamily:   meta.Subfamily,
			Format:      meta.Format,
			SizeBytes:   info.Size(),
			UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(customFonts, func(i, j int) bool {
		di := strings.ToLower(customFonts[i].DisplayName)
		dj := strings.ToLower(customFonts[j].DisplayName)
		if di != dj {
			return di < dj
		}
		return customFonts[i].Name < customFonts[j].Name
	})

	totalFamilies := len(getSystemFontFamilies(r.Context()))

	writeJSON(w, fontListResponse{
		FontDir:             fontDir,
		CustomFonts:         customFonts,
		TotalSystemFamilies: totalFamilies,
	})
}

// adminUploadFontHandler 上传自定义字体（.ttf, .ttc, .otf）
func adminUploadFontHandler(w http.ResponseWriter, r *http.Request) {
	// 单次上传最大 64MB
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid form or file exceeds size limit (64MB): "+err.Error())
		return
	}

	var files []*multipart.FileHeader
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if headers, ok := r.MultipartForm.File["files"]; ok {
			files = append(files, headers...)
		}
		if headers, ok := r.MultipartForm.File["file"]; ok {
			files = append(files, headers...)
		}
	}

	if len(files) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no font files provided")
		return
	}

	var uploaded []string
	for _, fh := range files {
		cleanName := filepath.Base(fh.Filename)
		if cleanName == "." || cleanName == "/" || cleanName == "" {
			continue
		}
		ext := filepath.Ext(cleanName)
		if !isAllowedFontExt(ext) {
			writeJSONError(w, http.StatusBadRequest, "only .ttf, .ttc, and .otf fonts are supported: "+cleanName)
			return
		}

		src, err := fh.Open()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to read uploaded file: "+err.Error())
			return
		}

		dstPath := filepath.Join(fontDir, cleanName)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			writeJSONError(w, http.StatusInternalServerError, "failed to save font: "+err.Error())
			return
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to write font: "+err.Error())
			return
		}

		uploaded = append(uploaded, cleanName)
	}

	syncAndRefreshFonts(r.Context())

	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"uploaded":            uploaded,
		"totalSystemFamilies": len(getSystemFontFamilies(r.Context())),
	})
}

// adminDeleteFontHandler 删除指定的自定义字体
func adminDeleteFontHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	rawName := vars["name"]
	cleanName := filepath.Base(rawName)
	if cleanName == "." || cleanName == "/" || cleanName == "" || cleanName != rawName {
		writeJSONError(w, http.StatusBadRequest, "invalid font filename")
		return
	}
	if !isAllowedFontExt(filepath.Ext(cleanName)) {
		writeJSONError(w, http.StatusBadRequest, "invalid font extension")
		return
	}

	targetPath := filepath.Join(fontDir, cleanName)
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		writeJSONError(w, http.StatusNotFound, "font not found")
		return
	}

	if err := os.Remove(targetPath); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete font: "+err.Error())
		return
	}

	// 同步删除系统字体目录中的副本或软链
	sysPath := filepath.Join(systemCustomFontDir, cleanName)
	_ = os.Remove(sysPath)

	syncAndRefreshFonts(r.Context())

	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"totalSystemFamilies": len(getSystemFontFamilies(r.Context())),
	})
}

// adminRefreshFontsHandler 手动触发字体同步与缓存刷新
func adminRefreshFontsHandler(w http.ResponseWriter, r *http.Request) {
	syncAndRefreshFonts(r.Context())
	totalFamilies := len(getSystemFontFamilies(r.Context()))
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"totalSystemFamilies": totalFamilies,
	})
}

// syncAndRefreshFonts 将 fontDir 中的字体同步到系统目录，刷新 fontconfig 与 cidfmap，并清理缓存
func syncAndRefreshFonts(ctx context.Context) {
	// 1. 若系统字体目录存在或可创建，将 fontDir 下的字体同步至系统目录
	if err := os.MkdirAll(systemCustomFontDir, 0755); err == nil {
		entries, err := os.ReadDir(fontDir)
		if err == nil {
			validFonts := make(map[string]bool)
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if !isAllowedFontExt(filepath.Ext(entry.Name())) {
					continue
				}
				validFonts[entry.Name()] = true
				src := filepath.Join(fontDir, entry.Name())
				dst := filepath.Join(systemCustomFontDir, entry.Name())

				if _, err := os.Lstat(dst); err == nil {
					_ = os.Remove(dst)
				}
				if err := os.Symlink(src, dst); err != nil {
					_ = copyFontFile(src, dst)
				}
			}

			// 清理系统目录中已在 fontDir 中被移除的孤立文件/软链
			sysEntries, err := os.ReadDir(systemCustomFontDir)
			if err == nil {
				for _, se := range sysEntries {
					if !validFonts[se.Name()] {
						_ = os.Remove(filepath.Join(systemCustomFontDir, se.Name()))
					}
				}
			}
		}
	}

	// 2. 执行 fc-cache -f 刷新 fontconfig（有超时保护，环境无 fc-cache 时不阻断）
	fcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(fcCtx, "fc-cache", "-f", systemCustomFontDir).Run(); err != nil {
		_ = exec.CommandContext(fcCtx, "fc-cache", "-f").Run()
	}

	// 3. 检查是否有核心 Windows 中文字体，若有则修补 Ghostscript cidfmap
	patchGhostscriptCidfmap()

	// 4. 重置内存缓存
	InvalidateSystemFontCache()
}

// patchGhostscriptCidfmap 修补 Ghostscript cidfmap 映射
func patchGhostscriptCidfmap() {
	gsCidfmap := "/etc/ghostscript/cidfmap.local"
	data, err := os.ReadFile(gsCidfmap)
	if err != nil {
		return
	}
	content := string(data)
	changed := false

	if _, err := os.Stat(filepath.Join(systemCustomFontDir, "simsun.ttf")); err == nil {
		newContent := strings.ReplaceAll(content,
			"/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0",
			"/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0",
		)
		if newContent != content {
			content = newContent
			changed = true
		}
	}
	if _, err := os.Stat(filepath.Join(systemCustomFontDir, "simhei.ttf")); err == nil {
		newContent := strings.ReplaceAll(content,
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc) /SubfontID 0",
			"/usr/share/fonts/truetype/custom/simhei.ttf) /SubfontID 0",
		)
		if newContent != content {
			content = newContent
			changed = true
		}
	}
	if _, err := os.Stat(filepath.Join(systemCustomFontDir, "simkai.ttf")); err == nil {
		newContent := strings.ReplaceAll(content,
			"/usr/share/fonts/truetype/arphic/ukai.ttc) /SubfontID 0",
			"/usr/share/fonts/truetype/custom/simkai.ttf) /SubfontID 0",
		)
		if newContent != content {
			content = newContent
			changed = true
		}
	}
	if _, err := os.Stat(filepath.Join(systemCustomFontDir, "simfang.ttf")); err == nil {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "/#b7#c2#cb#ce") {
				line = strings.ReplaceAll(line, "/usr/share/fonts/truetype/custom/simsun.ttf) /SubfontID 0", "/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0")
				line = strings.ReplaceAll(line, "/usr/share/fonts/truetype/arphic/uming.ttc) /SubfontID 0", "/usr/share/fonts/truetype/custom/simfang.ttf) /SubfontID 0")
				lines[i] = line
				changed = true
			}
		}
		content = strings.Join(lines, "\n")
	}

	if changed {
		if err := os.WriteFile(gsCidfmap, []byte(content), 0644); err != nil {
			log.Printf("[fonts] failed to write patched cidfmap: %v", err)
			return
		}
		_ = filepath.Walk("/usr/share/ghostscript", func(p string, info os.FileInfo, err error) error {
			if err == nil && info != nil && info.IsDir() && strings.HasSuffix(p, filepath.Join("Resource", "Init")) {
				_ = copyFontFile(gsCidfmap, filepath.Join(p, "cidfmap"))
			}
			return nil
		})
	}
}

func copyFontFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
