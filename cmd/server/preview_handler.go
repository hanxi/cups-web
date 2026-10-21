package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// previewRenderDPI 让 A4 页渲染为约 1240×1754 像素，手机高分屏上足够清晰。
const previewRenderDPI = 150

// previewPageHandler 把 PDF 的单页渲染为 PNG，供浏览器端 pdf.js 无法加载时兜底显示（issue #41）。
func previewPageHandler(w http.ResponseWriter, r *http.Request) {
	applyUploadLimit(w, r)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isMaxBytesError(err) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "文件超出管理员设置的大小上限")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll()

	page := 1
	if v := r.FormValue("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "页码超出范围")
			return
		}
		page = n
	}

	file, fh, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	inPath, cleanup, err := saveTempUpload(file, filepath.Base(fh.Filename))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save file")
		return
	}
	defer cleanup()

	if !fileHasPDFHeader(inPath) {
		writeJSONError(w, http.StatusBadRequest, "不是有效的 PDF 文件")
		return
	}

	gsBin, err := exec.LookPath("gs")
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ghostscript not found, cannot render PDF to images")
		return
	}

	ctx, cancel := convertTimeoutContext(r.Context())
	defer cancel()

	// 页数不可信（estimated）时不拦截越界，交给 gs 渲染结果判断。
	pages, estimated, _ := countPDFPagesWithFallback(ctx, inPath)
	if !estimated && page > pages {
		writeJSONError(w, http.StatusBadRequest, "页码超出范围")
		return
	}

	png, err := renderPDFPagePNG(ctx, gsBin, inPath, page)
	if err != nil {
		writeJSONError(w, http.StatusUnprocessableEntity, "预览渲染失败")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if !estimated {
		w.Header().Set("X-Page-Count", strconv.Itoa(pages))
	}
	_, _ = w.Write(png)
}

func renderPDFPagePNG(ctx context.Context, gsBin, pdfPath string, page int) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "pdf-preview-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "page.png")

	args := []string{
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-dQUIET",
		"-sDEVICE=png16m",
		"-r" + strconv.Itoa(previewRenderDPI),
		"-dTextAlphaBits=4", "-dGraphicsAlphaBits=4",
		fmt.Sprintf("-dFirstPage=%d", page),
		fmt.Sprintf("-dLastPage=%d", page),
	}
	args = append(args, cidfmapPreambleArgs()...)
	args = append(args, "-sOutputFile="+outPath, pdfPath)

	cmd := exec.CommandContext(ctx, gsBin, args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("gs render page %d failed: %w - %s", page, err, firstErrorLine(string(out)))
	}
	data, err := os.ReadFile(outPath)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("gs render page %d produced no output", page)
	}
	return data, nil
}
