package main

import (
	"log"
	"net/http"

	"cups-web/internal/ipp"
)

func printerInfoHandler(w http.ResponseWriter, r *http.Request) {
	uri := r.URL.Query().Get("uri")
	log.Printf("[printer-info] request received, uri=%q", uri)

	if uri == "" {
		log.Printf("[printer-info] error: missing uri parameter")
		writeJSONError(w, http.StatusBadRequest, "отсутствует параметр uri")
		return
	}

	log.Printf("[printer-info] calling GetPrinterAttributes for uri=%q", uri)
	info, err := ipp.GetPrinterAttributes(uri)
	if err != nil {
		log.Printf("[printer-info] GetPrinterAttributes error: %v", err)
		writeJSONError(w, http.StatusBadGateway, "не удалось получить информацию о принтере")
		return
	}

	log.Printf("[printer-info] success: name=%q state=%q jobs=%d", info.Name, info.State, info.QueuedJobs)
	writeJSON(w, info)
}
