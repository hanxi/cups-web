package main

import (
	_ "embed"
	"errors"

	"github.com/phpdave11/gofpdf"
)

//go:embed assets/fonts/NotoSansSC.ttf
var notoSansSCFont []byte

const pdfTextFontFamily = "NotoSansSC"

func setPdfTextFont(pdf *gofpdf.Fpdf, size float64) error {
	if len(notoSansSCFont) == 0 {
		return errors.New("embedded font is missing")
	}
	pdf.AddUTF8FontFromBytes(pdfTextFontFamily, "", notoSansSCFont)
	if pdf.Err() {
		return pdf.Error()
	}
	pdf.SetFont(pdfTextFontFamily, "", size)
	if pdf.Err() {
		return pdf.Error()
	}
	return nil
}
