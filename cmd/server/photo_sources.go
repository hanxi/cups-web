package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cups-web/internal/store"
)

func photoLayoutFromForm(r *http.Request, headers []*multipart.FileHeader) (store.PhotoLayout, error) {
	layout := store.PhotoLayout{PerPage: 1, Invert: r.FormValue("invert") == "true", Compress: r.FormValue("compress_images") == "true"}
	if len(headers) < 1 || len(headers) > 100 {
		return layout, errors.New("照片数量必须为 1–100 张")
	}
	if raw := r.FormValue("per_page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || (n != 1 && n != 2 && n != 4 && n != 6 && n != 9) {
			return layout, errors.New("不支持的每页照片张数")
		}
		layout.PerPage = n
	}
	rotations := make([]int, len(headers))
	if raw := r.FormValue("rotations"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) != len(headers) {
			return layout, errors.New("旋转参数必须与照片一一对应")
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || (n != 0 && n != 90 && n != 180 && n != 270) {
				return layout, errors.New("旋转角度必须为 0、90、180 或 270")
			}
			rotations[i] = n
		}
	}
	for i, header := range headers {
		layout.Sources = append(layout.Sources, store.PhotoSource{Filename: header.Filename, Rotation: rotations[i]})
	}
	return layout, nil
}

func renderPhotoSources(uploads string, layout store.PhotoLayout, orientation, paper string, options imagePDFOptions) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "photo-sources-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmp)
	var paths []string
	var rotations []int
	for i, source := range layout.Sources {
		f, err := os.OpenInRoot(uploads, filepath.FromSlash(source.StoredPath))
		if err != nil {
			return "", nil, err
		}
		path := filepath.Join(tmp, fmt.Sprintf("%d%s", i, filepath.Ext(source.Filename)))
		out, err := os.Create(path)
		if err != nil {
			f.Close()
			return "", nil, err
		}
		_, copyErr := io.Copy(out, f)
		closeErr := out.Close()
		f.Close()
		if copyErr != nil {
			return "", nil, copyErr
		}
		if closeErr != nil {
			return "", nil, closeErr
		}
		if detectFileKind(path, source.Filename) != fileKindImage {
			return "", nil, errors.New("照片队列仅支持图片")
		}
		paths = append(paths, path)
		rotations = append(rotations, source.Rotation)
	}
	return convertImagePathsToPDF(paths, orientation, paper, rotations, layout.Invert, layout.PerPage, layout.Compress, options)
}

func photoSourcePaths(layout store.PhotoLayout) []string {
	paths := make([]string, 0, len(layout.Sources))
	for _, source := range layout.Sources {
		paths = append(paths, source.StoredPath)
	}
	return paths
}

// Save the actual PDF submitted to CUPS, including application watermarks and
// percentage scaling. CUPS may still apply driver options and page selection.
func saveRenderedPDF(path, storedRel, uploads string) (string, error) {
	// Reuse the stored conversion when no later application transform changed
	// it, rather than keeping two identical copies of a full-resolution PDF.
	for _, cached := range []string{convertedRelPath(storedRel), storedRel} {
		if filepath.Clean(path) == filepath.Join(uploads, filepath.FromSlash(cached)) {
			return cached, nil
		}
	}
	rel := storedRel + ".submitted.pdf"
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	root, err := os.OpenRoot(uploads)
	if err != nil {
		return "", err
	}
	defer root.Close()
	out, err := root.Create(filepath.FromSlash(rel))
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove(filepath.FromSlash(rel))
		return "", err
	}
	return rel, nil
}

func printRecordPaths(record store.PrintRecord) []string {
	paths := []string{record.StoredPath, convertedRelPath(record.StoredPath), record.RenderedPath}
	return append(paths, photoSourcePaths(record.PhotoLayout)...)
}

func removePrintAssets(uploads string, paths []string) {
	root, err := os.OpenRoot(uploads)
	if err != nil {
		return
	}
	defer root.Close()
	for _, rel := range paths {
		if rel == "" || isScheduledRel(rel) {
			continue
		}
		_ = root.Remove(filepath.FromSlash(rel))
	}
}

// Reuse the upload/print pipeline, including validation, snapshots and rollback.
// Streaming originals through a temporary multipart file avoids keeping large
// photos or a second complete request body in memory.
func reprintPhotos(w http.ResponseWriter, r *http.Request, record store.PrintRecord, req reprintRequest) {
	layout := record.PhotoLayout
	if req.CompressImages != nil {
		layout.Compress = *req.CompressImages
	}
	if req.Invert != nil {
		layout.Invert = *req.Invert
	}
	if req.ImagesPerPage != nil {
		layout.PerPage = *req.ImagesPerPage
	}
	body, err := os.CreateTemp("", "photo-reprint-*")
	if err != nil {
		writeJSONError(w, 500, "failed to prepare reprint")
		return
	}
	defer os.Remove(body.Name())
	defer body.Close()
	writer := multipart.NewWriter(body)
	var rotations []string
	field := "file"
	if len(layout.Sources) > 1 {
		field = "files"
	}
	for _, source := range layout.Sources {
		f, err := os.OpenInRoot(uploadDir, filepath.FromSlash(source.StoredPath))
		if err != nil {
			writeJSONError(w, 404, "original photo not found")
			return
		}
		part, err := writer.CreateFormFile(field, source.Filename)
		if err == nil {
			_, err = io.Copy(part, f)
		}
		f.Close()
		if err != nil {
			writeJSONError(w, 500, "failed to prepare photo")
			return
		}
		rotations = append(rotations, strconv.Itoa(source.Rotation))
	}
	driverJSON, err := json.Marshal(req.DriverOptions)
	if err != nil {
		writeJSONError(w, 400, "invalid driver options")
		return
	}
	for key, value := range map[string]string{
		"sides": req.Sides, "printer": req.Printer, "duplex": strconv.FormatBool(req.Duplex), "color": strconv.FormatBool(req.Color), "copies": strconv.Itoa(req.Copies),
		"orientation": req.Orientation, "paper_size": req.PaperSize, "paper_type": req.PaperType, "media_source": req.MediaSource,
		"print_scaling": req.PrintScaling, "page_range": req.PageRange, "page_set": req.PageSet, "mirror": strconv.FormatBool(req.Mirror),
		"photo_date_watermark": strconv.FormatBool(req.PhotoDateWatermark), "watermark_text": req.WatermarkText,
		"number_up": strconv.Itoa(req.NumberUp), "number_up_layout": req.NumberUpLayout, "page_border": req.PageBorder,
		"driver_options": string(driverJSON), "rotations": strings.Join(rotations, ","), "per_page": strconv.Itoa(max(1, layout.PerPage)),
		"invert": strconv.FormatBool(layout.Invert), "compress_images": strconv.FormatBool(layout.Compress), "name": record.Filename,
	} {
		if err := writer.WriteField(key, value); err != nil {
			writeJSONError(w, 500, "failed to prepare options")
			return
		}
	}
	if err := writer.Close(); err != nil {
		writeJSONError(w, 500, "failed to prepare options")
		return
	}
	size, err := body.Seek(0, io.SeekCurrent)
	if err != nil {
		writeJSONError(w, 500, "failed to prepare reprint")
		return
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		writeJSONError(w, 500, "failed to prepare reprint")
		return
	}
	forwarded := r.Clone(r.Context())
	forwarded.Header.Set("Content-Type", writer.FormDataContentType())
	forwarded.Body, forwarded.ContentLength = io.NopCloser(body), size
	forwarded.Form, forwarded.PostForm, forwarded.MultipartForm = nil, nil, nil
	printHandler(w, forwarded)
}
