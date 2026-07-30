package main

import (
	"mime/multipart"
	"net/http"
)

func composeHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверная форма multipart")
		return
	}

	mode := r.FormValue("mode")
	if mode == "" {
		writeJSONError(w, http.StatusBadRequest, "отсутствует поле mode")
		return
	}

	var headers []*multipart.FileHeader
	if r.MultipartForm != nil {
		if fhs, ok := r.MultipartForm.File["files"]; ok {
			headers = fhs
		}
	}
	if len(headers) == 0 {
		writeJSONError(w, http.StatusBadRequest, "не предоставлено ни одного файла")
		return
	}

	ctx, cancel := convertTimeoutContext(r.Context())
	defer cancel()

	var outPath string
	var cleanup func()
	var err error

	switch mode {
	case "invoice":
		outPath, cleanup, err = composeInvoice2Up(ctx, headers)
	case "id_card":
		paper := r.FormValue("paper")
		if paper == "" {
			paper = "A4"
		}
		outPath, cleanup, err = composeIdCard(ctx, headers, paper)
	default:
		writeJSONError(w, http.StatusBadRequest, "неподдерживаемый режим: "+mode)
		return
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "ошибка компоновки: "+err.Error())
		return
	}
	defer cleanup()

	streamPDF(w, outPath, "скомпонованный.pdf")
}
