package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"
)

type printResp struct {
	JobID    string `json:"jobId,omitempty"`
	OK       bool   `json:"ok"`
	Pages    int    `json:"pages"`
	IsDuplex bool   `json:"isDuplex"`
	IsColor  bool   `json:"isColor"`
	Copies   int    `json:"copies"`
}

func printHandler(w http.ResponseWriter, r *http.Request) {
	// Expect multipart form
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, fh, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	printer := r.FormValue("printer")
	if printer == "" {
		writeJSONError(w, http.StatusBadRequest, "missing printer field")
		return
	}

	isDuplex := r.FormValue("duplex") == "true"
	isColor := r.FormValue("color") == "true"

	// Extended print options
	copiesStr := r.FormValue("copies")
	copies := 1
	if copiesStr != "" {
		if n, err := strconv.Atoi(copiesStr); err == nil && n > 0 {
			copies = n
		}
	}
	orientation := r.FormValue("orientation")
	paperSize := r.FormValue("paper_size")
	paperType := r.FormValue("paper_type")
	printScaling := r.FormValue("print_scaling")
	pageRange := r.FormValue("page_range")
	pageSet := r.FormValue("page_set")
	mirror := r.FormValue("mirror") == "true"
	marginTop, marginRight, marginBottom, marginLeft := parseFormMargins(r)

	storedRel, storedAbs, err := saveUploadedFile(file, fh.Filename, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	countCtx, cancel := convertTimeoutContext(r.Context())
	defer cancel()
	printPath := storedAbs
	var printCleanup func()
	printMime := ""
	var pages int
	kind := detectFileKind(storedAbs, fh.Filename)
	switch kind {
	case fileKindPDF:
		// PDF 标准化管线：诊断日志 → normalizePDF（gs / libreoffice / passthrough）→ 读页数
		diagnosePDF(storedAbs)

		normRes, nerr := normalizePDF(countCtx, storedAbs)
		if nerr != nil {
			log.Printf("[print] normalizePDF fatal: %v", nerr)
		}
		effectivePath := storedAbs
		if normRes != nil {
			effectivePath = normRes.OutputPath
			if normRes.Method != "passthrough" {
				// 将标准化后的 PDF 持久化到 uploads/ 下，命名为 <原文件>.print.pdf，
				// 以便维护任务按保留天数一起清理；随后把打印路径切到该副本。
				if _, convertedAbs, saveErr := saveConvertedPDFToUploads(normRes.OutputPath, storedRel, uploadDir); saveErr == nil {
					effectivePath = convertedAbs
				} else {
					log.Printf("[print] save normalized pdf failed: %v", saveErr)
				}
			}
			if normRes.Cleanup != nil {
				printCleanup = normRes.Cleanup
			}
		}

		var cerr error
		pages, cerr = countPDFPages(effectivePath)
		if cerr != nil {
			log.Printf("[print] countPDFPages failed (method=%s): %v", methodOf(normRes), cerr)
			pages = 1
		}
		printPath = effectivePath
		printMime = "application/pdf"
		// passthrough 且页数仍无法读取时，MIME 降级为 octet-stream，让 CUPS/IPP 自行识别
		if cerr != nil && normRes != nil && normRes.Method == "passthrough" {
			printMime = "application/octet-stream"
		}
	case fileKindOffice:
		outPath, cleanup, err := convertOfficeToPDF(countCtx, storedAbs)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "conversion failed")
			return
		}
		pages, err = countPDFPages(outPath)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "failed to read pages")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "failed to save converted file")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	case fileKindOFD:
		outPath, cleanup, err := convertOFDToPDF(countCtx, storedAbs)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "conversion failed")
			return
		}
		pages, err = countPDFPages(outPath)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "failed to read pages")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "failed to save converted file")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	case fileKindImage:
		outPath, cleanup, err := convertImageToPDF(storedAbs, orientation, paperSize, marginTop, marginRight, marginBottom, marginLeft)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "conversion failed")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "failed to save converted file")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
		pages = 1
	case fileKindText:
		var err error
		pages, err = estimateTextPages(storedAbs)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "failed to read pages")
			return
		}
		outPath, cleanup, err := convertTextToPDF(storedAbs, orientation, paperSize, marginTop, marginRight, marginBottom, marginLeft)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "conversion failed")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "failed to save converted file")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	default:
		var err error
		pages, _, err = countPages(countCtx, storedAbs, fh.Filename)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "failed to read pages")
			return
		}
	}
	if pages < 1 {
		pages = 1
	}
	if printCleanup != nil {
		defer printCleanup()
	}

	sess, _ := auth.GetSession(r)
	var recordID int64

	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}

		rec := store.PrintRecord{
			UserID:     user.ID,
			PrinterURI: printer,
			Filename:   fh.Filename,
			StoredPath: storedRel,
			Pages:      pages,
			Status:     "queued",
			IsDuplex:   isDuplex,
			IsColor:    isColor,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		id, err := store.InsertPrintRecord(r.Context(), tx, &rec)
		if err != nil {
			return err
		}
		recordID = id
		return nil
	})
	if err != nil {
		_ = os.Remove(storedAbs)
		writeJSONError(w, http.StatusInternalServerError, "failed to create print record")
		return
	}

	f, err := os.Open(printPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer f.Close()

	mime := printMime
	if mime == "" {
		mime = fh.Header.Get("Content-Type")
	}
	if mime == "" {
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			mime = http.DetectContentType(buf[:n])
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to read file")
				return
			}
		}
	}

	printOpts := ipp.PrintJobOptions{
		IsDuplex:     isDuplex,
		IsColor:      isColor,
		Copies:       copies,
		Orientation:  orientation,
		PaperSize:    paperSize,
		PaperType:    paperType,
		PrintScaling: printScaling,
		PageRange:    pageRange,
		PageSet:      pageSet,
		Mirror:       mirror,
		Pages:        pages,
	}

	job, err := ipp.SendPrintJob(printer, f, mime, sess.Username, fh.Filename, printOpts)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "print error: "+err.Error())
		return
	}

	_ = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.UpdatePrintStatus(r.Context(), tx, recordID, "printed", job)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printResp{
		JobID:    job,
		OK:       true,
		Pages:    pages,
		IsDuplex: isDuplex,
		IsColor:  isColor,
		Copies:   copies,
	})
}
