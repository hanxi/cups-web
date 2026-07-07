package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

type printRecordResponse struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	PrinterURI string `json:"printerUri"`
	Filename   string `json:"filename"`
	Pages      int    `json:"pages"`
	JobID      string `json:"jobId"`
	Status     string `json:"status"`
	IsDuplex   bool   `json:"isDuplex"`
	IsColor    bool   `json:"isColor"`
	CreatedAt  string `json:"createdAt"`
}

func printRecordsHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid date range")
		return
	}

	var resp []printRecordResponse
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		records, err := store.ListPrintRecords(r.Context(), tx, store.PrintFilter{
			Username: user.Username,
			StartAt:  startAt,
			EndAt:    endAt,
		})
		if err != nil {
			return err
		}
		resp = mapPrintRecords(records)
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load records")
		return
	}
	writeJSON(w, resp)
}

func adminPrintRecordsHandler(w http.ResponseWriter, r *http.Request) {
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid date range")
		return
	}
	username := r.URL.Query().Get("username")

	var resp []printRecordResponse
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		records, err := store.ListPrintRecords(r.Context(), tx, store.PrintFilter{
			Username: username,
			StartAt:  startAt,
			EndAt:    endAt,
		})
		if err != nil {
			return err
		}
		resp = mapPrintRecords(records)
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load records")
		return
	}
	writeJSON(w, resp)
}

func printRecordFileHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid record id")
		return
	}

	var record store.PrintRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		rec, err := store.GetPrintRecordByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		record = rec
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "record not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to load record")
		return
	}
	if sess.Role != store.RoleAdmin && record.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	absPath := filepath.Join(uploadDir, filepath.FromSlash(record.StoredPath))
	f, err := os.Open(absPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to stat file")
		return
	}

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": record.Filename})
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, record.Filename, stat.ModTime(), f)
}

func parseDateRange(r *http.Request) (string, string, error) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" && end == "" {
		return "", "", nil
	}
	var startAt string
	var endAt string
	if start != "" {
		t, err := time.ParseInLocation("2006-01-02", start, time.Local)
		if err != nil {
			return "", "", err
		}
		startAt = t.UTC().Format(time.RFC3339)
	}
	if end != "" {
		t, err := time.ParseInLocation("2006-01-02", end, time.Local)
		if err != nil {
			return "", "", err
		}
		t = t.AddDate(0, 0, 1).Add(-time.Second)
		endAt = t.UTC().Format(time.RFC3339)
	}
	return startAt, endAt, nil
}

func mapPrintRecords(records []store.PrintRecord) []printRecordResponse {
	resp := make([]printRecordResponse, 0, len(records))
	for _, rec := range records {
		jobID := ""
		if rec.JobID.Valid {
			jobID = rec.JobID.String
		}
		resp = append(resp, printRecordResponse{
			ID:         rec.ID,
			UserID:     rec.UserID,
			Username:   rec.Username,
			PrinterURI: rec.PrinterURI,
			Filename:   rec.Filename,
			Pages:      rec.Pages,
			JobID:      jobID,
			Status:     rec.Status,
			IsDuplex:   rec.IsDuplex,
			IsColor:    rec.IsColor,
			CreatedAt:  rec.CreatedAt,
		})
	}
	return resp
}

type reprintRequest struct {
	Printer      string `json:"printer"`
	Duplex       bool   `json:"duplex"`
	Color        bool   `json:"color"`
	Copies       int    `json:"copies"`
	Orientation  string `json:"orientation"`
	PaperSize    string `json:"paperSize"`
	PaperType    string `json:"paperType"`
	PrintScaling string `json:"printScaling"`
	PageRange    string `json:"pageRange"`
	PageSet      string `json:"pageSet"`
	Mirror       bool   `json:"mirror"`

	NumberUp       int    `json:"numberUp"`
	NumberUpLayout string `json:"numberUpLayout"`
	PageBorder     string `json:"pageBorder"`
}

func reprintHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid record id")
		return
	}

	var req reprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Printer == "" {
		writeJSONError(w, http.StatusBadRequest, "missing printer field")
		return
	}
	if req.Copies < 1 {
		req.Copies = 1
	}
	switch req.NumberUp {
	case 1, 2, 4, 6, 9, 16:
		// valid
	default:
		req.NumberUp = 1
	}

	var record store.PrintRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		rec, err := store.GetPrintRecordByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		record = rec
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "record not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to load record")
		return
	}

	if sess.Role != store.RoleAdmin && record.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	origPath := filepath.Join(uploadDir, filepath.FromSlash(record.StoredPath))
	origFile, err := os.Open(origPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "original file not found, may have been cleaned up")
		return
	}
	defer origFile.Close()

	storedRel, storedAbs, err := saveUploadedFile(origFile, record.Filename, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to copy file")
		return
	}

	countCtx, cancel := convertTimeoutContext(r.Context())
	defer cancel()
	printPath := storedAbs
	var printCleanup func()
	printMime := ""
	var pages int
	kind := detectFileKind(storedAbs, record.Filename)
	switch kind {
	case fileKindPDF:
		var cerr error
		pages, cerr = countPDFPages(storedAbs)
		if cerr != nil {
			log.Printf("[reprint] countPDFPages failed: %v", cerr)
			pages = 1
		}
		printPath = storedAbs
		printMime = "application/pdf"
		if cerr != nil {
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
		outPath, cleanup, err := convertImageToPDF(storedAbs, req.Orientation, req.PaperSize)
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
		pages, err = estimateTextPages(storedAbs)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "failed to read pages")
			return
		}
		outPath, cleanup, err := convertTextToPDF(storedAbs, req.Orientation, req.PaperSize)
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
		pages, _, err = countPages(countCtx, storedAbs, record.Filename)
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

	pageSet := req.PageSet
	if pageSet == "even-reverse" && printMime == "application/pdf" && pages > 1 {
		reorderedPath, reorderCleanup, rerr := reorderPDFForManualDuplex(printPath, pages, req.PaperSize)
		if rerr != nil {
			log.Printf("[reprint] even-reverse reorder failed: %v, falling back to normal even", rerr)
			pageSet = "even"
		} else {
			defer reorderCleanup()
			printPath = reorderedPath
			reorderedPages, _ := countPDFPages(reorderedPath)
			if reorderedPages > 0 {
				pages = reorderedPages
			}
			pageSet = ""
		}
	}

	var recordID int64
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		rec := store.PrintRecord{
			UserID:     sess.UserID,
			PrinterURI: req.Printer,
			Filename:   record.Filename,
			StoredPath: storedRel,
			Pages:      pages,
			Status:     "queued",
			IsDuplex:   req.Duplex,
			IsColor:    req.Color,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		rid, err := store.InsertPrintRecord(r.Context(), tx, &rec)
		if err != nil {
			return err
		}
		recordID = rid
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

	mimeType := printMime
	if mimeType == "" {
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			mimeType = http.DetectContentType(buf[:n])
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to read file")
				return
			}
		}
	}

	printOpts := ipp.PrintJobOptions{
		IsDuplex:     req.Duplex,
		IsColor:      req.Color,
		Copies:       req.Copies,
		Orientation:  req.Orientation,
		PaperSize:    req.PaperSize,
		PaperType:    req.PaperType,
		PrintScaling: req.PrintScaling,
		PageRange:    req.PageRange,
		PageSet:      pageSet,
		Mirror:       req.Mirror,
		Pages:        pages,

		NumberUp:       req.NumberUp,
		NumberUpLayout: req.NumberUpLayout,
		PageBorder:     req.PageBorder,
	}

	job, err := ipp.SendPrintJob(req.Printer, f, mimeType, sess.Username, record.Filename, printOpts)
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
		IsDuplex: req.Duplex,
		IsColor:  req.Color,
		Copies:   req.Copies,
	})
}