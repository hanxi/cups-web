package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func convertHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, "неверная форма multipart", http.StatusBadRequest)
		return
	}

	orientation := r.FormValue("orientation")
	paperSize := r.FormValue("paper_size")

	var outPath string
	var outCleanup func()
	var outFilename string
	var err error

	if r.MultipartForm != nil {
		if headers, ok := r.MultipartForm.File["files"]; ok && len(headers) > 0 {
			outPath, outCleanup, err = convertImagesMultiToPDF(headers, orientation, paperSize)
			if err != nil {
				http.Error(w, "ошибка преобразования: "+err.Error(), http.StatusInternalServerError)
				return
			}
			defer outCleanup()

			outFilename = r.FormValue("name")
			if outFilename == "" {
				outFilename = "объединенные_изображения.pdf"
			}

			streamPDF(w, outPath, outFilename)
			return
		}
	}

	file, fh, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "отсутствует поле file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	inPath, cleanup, err := saveTempUpload(file, fh.Filename)
	if err != nil {
		http.Error(w, "не удалось сохранить файл", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	ctx, cancel := convertTimeoutContext(r.Context())
	defer cancel()

	kind := detectFileKind(inPath, fh.Filename)
	switch kind {
	case fileKindImage:
		outPath, outCleanup, err = convertImageToPDF(inPath, orientation, paperSize)
	case fileKindText:
		outPath, outCleanup, err = convertTextToPDF(inPath, orientation, paperSize)
	case fileKindOFD:
		outPath, outCleanup, err = convertOFDToPDF(ctx, inPath)
	case fileKindPDF:
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
		http.Error(w, "ошибка преобразования: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer outCleanup()

	base := filepath.Base(fh.Filename)
	ext := filepath.Ext(base)
	name := base[0 : len(base)-len(ext)]
	outFilename = name + ".pdf"

	streamPDF(w, outPath, outFilename)
}

func streamPDF(w http.ResponseWriter, path string, filename string) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	pdfFile, err := os.Open(path)
	if err != nil {
		http.Error(w, "не удалось открыть преобразованный файл", http.StatusInternalServerError)
		return
	}
	defer pdfFile.Close()
	if _, err := io.Copy(w, pdfFile); err != nil {
		return
	}
}
