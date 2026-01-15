package main

import (
	"bufio"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/phpdave11/gofpdf"
)

const pdfPageMarginMM = 10.0

func convertImageToPDF(inputPath string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "convert-img-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	f, err := os.Open(inputPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	cfg, _, err := image.DecodeConfig(f)
	_ = f.Close()
	if err != nil {
		cleanup()
		return "", nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		cleanup()
		return "", nil, errors.New("invalid image dimensions")
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfPageMarginMM, pdfPageMarginMM, pdfPageMarginMM)
	pdf.SetAutoPageBreak(false, pdfPageMarginMM)
	pdf.AddPage()

	pageW, pageH := pdf.GetPageSize()
	maxW := pageW - 2*pdfPageMarginMM
	maxH := pageH - 2*pdfPageMarginMM
	scale := math.Min(maxW/float64(cfg.Width), maxH/float64(cfg.Height))
	if scale <= 0 {
		scale = 1
	}
	w := float64(cfg.Width) * scale
	h := float64(cfg.Height) * scale
	x := (pageW - w) / 2
	y := (pageH - h) / 2

	opts := gofpdf.ImageOptions{ImageType: "", ReadDpi: true}
	pdf.ImageOptions(inputPath, x, y, w, h, false, opts, 0, "")

	outPath := filepath.Join(tmpDir, "image.pdf")
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		cleanup()
		return "", nil, err
	}
	return outPath, cleanup, nil
}

func convertTextToPDF(inputPath string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "convert-text-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	f, err := os.Open(inputPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer f.Close()

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfPageMarginMM, pdfPageMarginMM, pdfPageMarginMM)
	pdf.SetAutoPageBreak(false, pdfPageMarginMM)
	pdf.AddPage()
	if err := setPdfTextFont(pdf, 10); err != nil {
		cleanup()
		return "", nil, err
	}

	_, pageH := pdf.GetPageSize()
	lineHeight := (pageH - 2*pdfPageMarginMM) / float64(textLinesPerPage)
	if lineHeight <= 0 {
		lineHeight = 4
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineIndex := 0
	for scanner.Scan() {
		if lineIndex >= textLinesPerPage {
			pdf.AddPage()
			lineIndex = 0
		}
		y := pdfPageMarginMM + lineHeight*float64(lineIndex+1)
		pdf.Text(pdfPageMarginMM, y, scanner.Text())
		lineIndex++
	}
	if err := scanner.Err(); err != nil {
		cleanup()
		return "", nil, err
	}

	outPath := filepath.Join(tmpDir, "text.pdf")
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		cleanup()
		return "", nil, err
	}
	return outPath, cleanup, nil
}
