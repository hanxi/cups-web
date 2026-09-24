package main

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func convertHandler(w http.ResponseWriter, r *http.Request) {
	applyUploadLimit(w, r)
	// Expect multipart form
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		if isMaxBytesError(err) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "文件超出管理员设置的大小上限")
			return
		}
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	// 读取方向和纸张大小参数
	orientation := r.FormValue("orientation")
	paperSize := r.FormValue("paper_size")
	// 黑白反转（issue #87）：仅对图片转 PDF 生效
	invert := r.FormValue("invert") == "true"

	var outPath string
	var outCleanup func()
	var outFilename string
	var err error

	// 优先处理多文件字段（图片合并场景）
	if r.MultipartForm != nil {
		if headers, ok := r.MultipartForm.File["files"]; ok && len(headers) > 0 {
			rotations := parseRotations(r.FormValue("rotations"))
			// per_page(issue #37):把多张图片排到同一页的网格布局。默认 1(每张一页)。
			perPage := 1
			if n, e := strconv.Atoi(r.FormValue("per_page")); e == nil {
				switch n {
				case 1, 2, 4, 6, 9:
					perPage = n
				}
			}
			outPath, outCleanup, err = convertImagesMultiToPDF(headers, orientation, paperSize, rotations, invert, perPage)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "文件转换失败："+err.Error())
				return
			}
			defer outCleanup()

			// 输出文件名：优先用前端传入的 name，否则用默认的
			outFilename = r.FormValue("name")
			if outFilename == "" {
				outFilename = "合并图片.pdf"
			}

			streamPDF(w, outPath, outFilename)
			return
		}
	}

	// 单文件分支
	file, fh, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	inPath, cleanup, err := saveTempUpload(file, fh.Filename)
	if err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	ctx, cancel := convertTimeoutContext(r.Context())
	defer cancel()

	kind := detectFileKind(inPath, fh.Filename)
	switch kind {
	case fileKindImage:
		outPath, outCleanup, err = convertImageToPDF(inPath, orientation, paperSize, invert)
	case fileKindText:
		outPath, outCleanup, err = convertTextToPDF(inPath, orientation, paperSize)
	case fileKindOFD:
		outPath, outCleanup, err = convertOFDToPDF(ctx, inPath)
	case fileKindPDF:
		// 默认不再对上传 PDF 走 gs：客户端在 UI 点击"应用 GS 规范化"时
		// 才会带上 normalize=true 显式触发，用于修复 CJK 字体乱码等问题。
		// 否则原样回传，预览端使用原始字节，打印端也读同一份字节，预览/打印一致。
		if r.FormValue("normalize") == "true" {
			diagnosePDF(inPath)
			res, normErr := normalizePDF(ctx, inPath)
			if normErr != nil {
				err = normErr
			} else {
				outPath = res.OutputPath
				if res.Cleanup != nil {
					outCleanup = res.Cleanup
				} else {
					outCleanup = func() {}
				}
			}
		} else {
			outPath = inPath
			outCleanup = func() {}
		}
	default:
		outPath, outCleanup, err = convertOfficeToPDF(ctx, inPath)
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, convertErrMsg(err))
		return
	}
	defer outCleanup()

	base := filepath.Base(fh.Filename)
	ext := filepath.Ext(base)
	name := base[0 : len(base)-len(ext)]
	outFilename = name + ".pdf"

	streamPDF(w, outPath, outFilename)
}

func convertErrMsg(err error) string {
	if errors.Is(err, errBinaryNotInstalled) {
		return "服务器未安装 LibreOffice，无法转换 Office 文档。请联系管理员安装 LibreOffice。"
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "服务器缺少文件转换所需的工具，请联系管理员安装相关依赖。"
	}
	return "文件转换失败：" + err.Error()
}

// parseRotations 解析多图合一的逐图旋转参数（issue #87）。
// 输入形如 "90,0,270"，按顺序对应每个 files 字段；缺项或非法按 0（不旋转）。
func parseRotations(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, 0)
			continue
		}
		// 归一化到 0/90/180/270
		switch ((n % 360) + 360) % 360 {
		case 90, 180, 270:
			out = append(out, n)
		default:
			out = append(out, 0)
		}
	}
	return out
}

// streamPDF 以 application/pdf 的 Content-Type 把 PDF 文件流式写回响应
func streamPDF(w http.ResponseWriter, path string, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	// mime.FormatMediaType 对非 ASCII 文件名自动输出 RFC 5987 形式(issue #114)。
	// filename 来自用户输入(FormValue / 上传名),含控制字符等非法值时返回空串,
	// 兜底为不带文件名的 attachment,保证仍是下载而非内联展示。
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition)
	pdfFile, err := os.Open(path)
	if err != nil {
		http.Error(w, "failed to open converted file", http.StatusInternalServerError)
		return
	}
	defer pdfFile.Close()
	if _, err := io.Copy(w, pdfFile); err != nil {
		// nothing more we can do
		return
	}
}
