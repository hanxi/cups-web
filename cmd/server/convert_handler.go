package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func convertHandler(w http.ResponseWriter, r *http.Request) {
	// Expect multipart form
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	// 读取方向和纸张大小参数
	orientation := r.FormValue("orientation")
	paperSize := r.FormValue("paper_size")
	marginTop, marginRight, marginBottom, marginLeft := parseFormMargins(r)

	var outPath string
	var outCleanup func()
	var outFilename string
	var err error

	// 优先处理多文件字段（图片合并场景）
	if r.MultipartForm != nil {
		if headers, ok := r.MultipartForm.File["files"]; ok && len(headers) > 0 {
			outPath, outCleanup, err = convertImagesMultiToPDF(headers, orientation, paperSize, marginTop, marginRight, marginBottom, marginLeft)
			if err != nil {
				http.Error(w, "conversion failed: "+err.Error(), http.StatusInternalServerError)
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
		outPath, outCleanup, err = convertImageToPDF(inPath, orientation, paperSize, marginTop, marginRight, marginBottom, marginLeft)
	case fileKindText:
		outPath, outCleanup, err = convertTextToPDF(inPath, orientation, paperSize, marginTop, marginRight, marginBottom, marginLeft)
	case fileKindOFD:
		outPath, outCleanup, err = convertOFDToPDF(ctx, inPath)
	case fileKindPDF:
		// 预览路径也走 normalizePDF，确保预览与打印看到完全相同的字节流。
		// gs pdfwrite 会将空壳 CID 字体重建为嵌入 TrueType 子集，
		// 配合 pdf.js 的 CMap + 嵌入字体优先配置，渲染效果与打印一致。
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
	default:
		outPath, outCleanup, err = convertOfficeToPDF(ctx, inPath)
	}
	if err != nil {
		http.Error(w, "conversion failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer outCleanup()

	base := filepath.Base(fh.Filename)
	ext := filepath.Ext(base)
	name := base[0 : len(base)-len(ext)]
	outFilename = name + ".pdf"

	streamPDF(w, outPath, outFilename)
}

// streamPDF 以 application/pdf 的 Content-Type 把 PDF 文件流式写回响应
func streamPDF(w http.ResponseWriter, path string, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
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
