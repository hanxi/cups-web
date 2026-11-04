package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/xml"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cups-web/internal/store"

	"rsc.io/pdf"
)

var (
	systemFontsMutex   sync.RWMutex
	systemFontsCached  map[string]struct{}
	systemFontsExpires time.Time
)

// normalizeFontName 归一化字体名：小写、移除空格、下划线、短横线
func normalizeFontName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// 常见中文字体别名与关联映射
var knownFontAliases = map[string][]string{
	"simsun":               {"simsun", "宋体", "nsimsun", "新宋体", "stsong", "华文宋体"},
	"宋体":                   {"simsun", "宋体", "nsimsun", "新宋体", "stsong", "华文宋体"},
	"simhei":               {"simhei", "黑体", "stheiti", "华文黑体"},
	"黑体":                   {"simhei", "黑体", "stheiti", "华文黑体"},
	"kaiti":                {"kaiti", "楷体", "simkai", "kaiti_gb2312", "楷体_gb2312", "stkaiti", "华文楷体"},
	"楷体":                   {"kaiti", "楷体", "simkai", "kaiti_gb2312", "楷体_gb2312", "stkaiti", "华文楷体"},
	"kaiti_gb2312":         {"kaiti", "楷体", "simkai", "kaiti_gb2312", "楷体_gb2312", "stkaiti", "华文楷体"},
	"楷体_gb2312":          {"kaiti", "楷体", "simkai", "kaiti_gb2312", "楷体_gb2312", "stkaiti", "华文楷体"},
	"fangsong":             {"fangsong", "仿宋", "simfang", "fangsong_gb2312", "仿宋_gb2312", "stfangsong", "华文仿宋"},
	"仿宋":                   {"fangsong", "仿宋", "simfang", "fangsong_gb2312", "仿宋_gb2312", "stfangsong", "华文仿宋"},
	"fangsong_gb2312":      {"fangsong", "仿宋", "simfang", "fangsong_gb2312", "仿宋_gb2312", "stfangsong", "华文仿宋"},
	"仿宋_gb2312":          {"fangsong", "仿宋", "simfang", "fangsong_gb2312", "仿宋_gb2312", "stfangsong", "华文仿宋"},
	"microsoftyahei":       {"microsoftyahei", "微软雅黑", "msyahei", "microsoftyaheiui"},
	"微软雅黑":                 {"microsoftyahei", "微软雅黑", "msyahei", "microsoftyaheiui"},
	"fzxiaoBiaosong-b05s":  {"fzxiaoBiaosongb05s", "方正小标宋简体", "方正小标宋", "fzxiaoBiaosong"},
	"方正小标宋简体":              {"fzxiaoBiaosongb05s", "方正小标宋简体", "方正小标宋", "fzxiaoBiaosong"},
	"方正小标宋":                {"fzxiaoBiaosongb05s", "方正小标宋简体", "方正小标宋", "fzxiaoBiaosong"},
	"fzdaBiaosong-b06s":    {"fzdaBiaosongb06s", "方正大标宋简体", "方正大标宋"},
	"方正大标宋简体":              {"fzdaBiaosongb06s", "方正大标宋简体", "方正大标宋"},
	"fzfangsong-z02s":      {"fzfangsongz02s", "方正仿宋简体", "方正仿宋"},
	"方正仿宋简体":                {"fzfangsongz02s", "方正仿宋简体", "方正仿宋"},
	"fzkaiti-z03s":         {"fzkaitiz03s", "方正楷体简体", "方正楷体"},
	"方正楷体简体":                {"fzkaitiz03s", "方正楷体简体", "方正楷体"},
	"fzheiti-z01s":         {"fzheitiz01s", "方正黑体简体", "方正黑体"},
	"方正黑体简体":                {"fzheitiz01s", "方正黑体简体", "方正黑体"},
	"sourcehansans":        {"sourcehansans", "思源黑体", "sourcehansanssc", "notosanscjksc", "notosanscjk"},
	"思源黑体":                 {"sourcehansans", "思源黑体", "sourcehansanssc", "notosanscjksc", "notosanscjk"},
	"sourcehanserif":       {"sourcehanserif", "思源宋体", "sourcehanserifsc", "notoserifcjksc", "notoserifcjk"},
	"思源宋体":                 {"sourcehanserif", "思源宋体", "sourcehanserifsc", "notoserifcjksc", "notoserifcjk"},
	"pingfangsc":           {"pingfangsc", "苹方", "pingfang"},
	"苹方":                   {"pingfangsc", "苹方", "pingfang"},
}

// 符号字体与无需检查的系统基础字体
var ignoredFontSet = map[string]struct{}{
	"symbol":      {},
	"wingdings":   {},
	"wingdings2":  {},
	"wingdings3":  {},
	"webdings":    {},
	"mtextra":     {},
	"marlett":     {},
	"courier":     {},
	"helvetica":   {},
	"times":       {},
	"zapfdingbats": {},
}

// PDF GBK 字节 BaseFont 十六进制转中文名
var gbkHexToFontName = map[string]string{
	"#cb#ce#cc#e5": "宋体 (SimSun)",
	"#ba#da#cc#e5": "黑体 (SimHei)",
	"#bf#ac#cc#e5": "楷体 (SimKai)",
	"#b7#c2#cb#ce": "仿宋 (SimFang)",
}

// getSystemFontFamilies 获取并缓存系统已安装字体族列表
func getSystemFontFamilies(ctx context.Context) map[string]struct{} {
	systemFontsMutex.RLock()
	if systemFontsCached != nil && time.Now().Before(systemFontsExpires) {
		res := systemFontsCached
		systemFontsMutex.RUnlock()
		return res
	}
	systemFontsMutex.RUnlock()

	systemFontsMutex.Lock()
	defer systemFontsMutex.Unlock()

	// 再次双检缓存
	if systemFontsCached != nil && time.Now().Before(systemFontsExpires) {
		return systemFontsCached
	}

	families := make(map[string]struct{})

	// 尝试运行 fc-list : family 获取
	cmd := exec.CommandContext(ctx, "fc-list", ":", "family")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					families[normalizeFontName(part)] = struct{}{}
					families[strings.ToLower(part)] = struct{}{}
				}
			}
		}
	} else {
		// fc-list 不可用时扫描常见目录
		scanDirs := []string{
			"/usr/share/fonts/truetype/custom",
			"/data/fonts",
			"/usr/share/fonts",
		}
		for _, dir := range scanDirs {
			_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
				if err != nil || info == nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(info.Name()))
				if ext == ".ttf" || ext == ".ttc" || ext == ".otf" {
					base := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
					families[normalizeFontName(base)] = struct{}{}
					families[strings.ToLower(base)] = struct{}{}
					if meta, err := ParseFontMetadataFromFile(p); err == nil {
						for _, fam := range meta.AllFamilies {
							families[normalizeFontName(fam)] = struct{}{}
							families[strings.ToLower(fam)] = struct{}{}
						}
					}
				}
				return nil
			})
		}
	}

	systemFontsCached = families
	systemFontsExpires = time.Now().Add(60 * time.Second)
	return families
}

// InvalidateSystemFontCache 强制刷新系统字体缓存（如挂载或上传字体后）
func InvalidateSystemFontCache() {
	systemFontsMutex.Lock()
	systemFontsCached = nil
	systemFontsExpires = time.Time{}
	systemFontsMutex.Unlock()
}

// isFontAvailable 判断给定字体名在系统字体库中是否存在（支持别名比对）
func isFontAvailable(fontName string, systemFonts map[string]struct{}) bool {
	norm := normalizeFontName(fontName)
	if _, ok := ignoredFontSet[norm]; ok {
		return true
	}

	// 直接完全匹配
	if _, ok := systemFonts[norm]; ok {
		return true
	}
	if _, ok := systemFonts[strings.ToLower(fontName)]; ok {
		return true
	}

	// 别名匹配
	for key, aliases := range knownFontAliases {
		keyNorm := normalizeFontName(key)
		matchKey := norm == keyNorm
		if !matchKey {
			for _, a := range aliases {
				if norm == normalizeFontName(a) {
					matchKey = true
					break
				}
			}
		}
		if matchKey {
			for _, a := range aliases {
				if _, ok := systemFonts[normalizeFontName(a)]; ok {
					return true
				}
				if _, ok := systemFonts[strings.ToLower(a)]; ok {
					return true
				}
			}
		}
	}

	return false
}

// docxFontTable 对应 word/fontTable.xml 结构
type docxFontTable struct {
	XMLName xml.Name `xml:"fonts"`
	Fonts   []struct {
		Name    string `xml:"name,attr"`
		AltName *struct {
			Val string `xml:"val,attr"`
		} `xml:"altName"`
	} `xml:"font"`
}

// extractDocxFonts 从 .docx 文件中读取声明使用的所有字体名称
func extractDocxFonts(filePath string) ([]string, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == "word/fontTable.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			var ft docxFontTable
			if err := xml.Unmarshal(data, &ft); err != nil {
				return nil, err
			}

			seen := make(map[string]bool)
			var fonts []string
			for _, font := range ft.Fonts {
				name := strings.TrimSpace(font.Name)
				if name != "" && !seen[name] {
					seen[name] = true
					fonts = append(fonts, name)
				}
			}
			return fonts, nil
		}
	}
	return nil, nil
}

// extractPDFUnembeddedFonts 从 PDF 文件中提取所有未内嵌（Non-embedded）的字体名称
func extractPDFUnembeddedFonts(filePath string) ([]string, error) {
	doc, err := pdf.Open(filePath)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var unembedded []string

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[font_preflight] pdf inspection recovered from panic: %v", r)
		}
	}()

	numPages := doc.NumPage()
	for i := 1; i <= numPages; i++ {
		page := doc.Page(i)
		fonts := page.V.Key("Resources").Key("Font")
		if fonts.Kind() == 0 {
			continue
		}
		for _, fontKey := range fonts.Keys() {
			f := fonts.Key(fontKey)
			if f.Kind() == 0 {
				continue
			}

			baseFont := f.Key("BaseFont").Name()
			if baseFont == "" {
				continue
			}

			// 检查是否包含子集前缀，如 ABCDEF+FangSong -> FangSong
			cleanFont := baseFont
			if idx := strings.Index(baseFont, "+"); idx >= 0 && idx < 10 {
				cleanFont = baseFont[idx+1:]
			}

			// 检查是否内嵌
			isEmbedded := false
			desc := f.Key("FontDescriptor")
			if desc.Kind() != 0 {
				if desc.Key("FontFile").Kind() != 0 ||
					desc.Key("FontFile2").Kind() != 0 ||
					desc.Key("FontFile3").Kind() != 0 {
					isEmbedded = true
				}
			}

			// Type0 字体检查其 DescendantFonts
			descendantFonts := f.Key("DescendantFonts")
			if descendantFonts.Kind() != 0 && descendantFonts.Len() > 0 {
				df := descendantFonts.Index(0)
				dfDesc := df.Key("FontDescriptor")
				if dfDesc.Kind() != 0 {
					if dfDesc.Key("FontFile").Kind() != 0 ||
						dfDesc.Key("FontFile2").Kind() != 0 ||
						dfDesc.Key("FontFile3").Kind() != 0 {
						isEmbedded = true
					}
				}
			}

			// 如果未内嵌
			if !isEmbedded {
				// 检查 GBK 转义字体名
				lowerFont := strings.ToLower(cleanFont)
				displayName := cleanFont
				for hexPrefix, mappedName := range gbkHexToFontName {
					if strings.Contains(lowerFont, hexPrefix) {
						displayName = mappedName
						break
					}
				}

				if !seen[displayName] {
					seen[displayName] = true
					unembedded = append(unembedded, displayName)
				}
			}
		}
	}

	return unembedded, nil
}

// checkDocumentMissingFonts 检查文档中是否存在系统未安装的字体
func checkDocumentMissingFonts(ctx context.Context, filePath string, filename string) ([]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	var docFonts []string
	var err error

	if ext == ".docx" {
		docFonts, err = extractDocxFonts(filePath)
		if err != nil {
			log.Printf("[font_preflight] extract docx fonts failed: %v", err)
			return nil, nil // 解析失败时不阻断流程
		}
	} else if ext == ".pdf" {
		docFonts, err = extractPDFUnembeddedFonts(filePath)
		if err != nil {
			log.Printf("[font_preflight] extract pdf fonts failed: %v", err)
			return nil, nil // 解析失败时不阻断流程
		}
	} else {
		// 其他格式暂不预检
		return nil, nil
	}

	if len(docFonts) == 0 {
		return nil, nil
	}

	systemFonts := getSystemFontFamilies(ctx)
	var missing []string
	seen := make(map[string]bool)

	for _, fontName := range docFonts {
		trimmed := strings.TrimSpace(fontName)
		if trimmed == "" {
			continue
		}
		if !isFontAvailable(trimmed, systemFonts) {
			if !seen[trimmed] {
				seen[trimmed] = true
				missing = append(missing, trimmed)
			}
		}
	}

	return missing, nil
}

// isFontFallbackAllowed 检查当前请求是否允许缺少字体时自动回退替换
func isFontFallbackAllowed(ctx context.Context, formAllow string) bool {
	if formAllow == "true" {
		return true
	}
	// 查询全局设置
	var allowGlobal int64
	_ = appStore.WithTx(ctx, true, func(tx *sql.Tx) error {
		val, err := store.GetSettingInt(ctx, tx, store.SettingAllowFontFallback, 0)
		if err == nil {
			allowGlobal = val
		}
		return nil
	})
	return allowGlobal != 0
}
