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
	"strings"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

type photoSourceResponse struct {
	Filename string `json:"filename"`
	Rotation int    `json:"rotation"`
}

type printRecordResponse struct {
	PhotoSources   []photoSourceResponse `json:"photoSources,omitempty"`
	ImagesPerPage  int                   `json:"imagesPerPage,omitempty"`
	Invert         bool                  `json:"invert"`
	CompressImages bool                  `json:"compressImages"`
	HasRenderedPDF bool                  `json:"hasRenderedPDF"`
	DriverOptions  ipp.DriverOptions     `json:"driverOptions"`
	ID             int64                 `json:"id"`
	UserID         int64                 `json:"userId"`
	Username       string                `json:"username"`
	PrinterURI     string                `json:"printerUri"`
	Filename       string                `json:"filename"`
	Pages          int                   `json:"pages"`
	JobID          string                `json:"jobId"`
	Status         string                `json:"status"`
	Sides          string                `json:"sides"`
	IsDuplex       bool                  `json:"isDuplex"`
	IsColor        bool                  `json:"isColor"`

	// 完整打印参数快照，供前端「重新打印」预填第一次的设置（Issue #68）。
	Copies             int    `json:"copies"`
	Orientation        string `json:"orientation"`
	PaperSize          string `json:"paperSize"`
	PaperType          string `json:"paperType"`
	MediaSource        string `json:"mediaSource"`
	PrintScaling       string `json:"printScaling"`
	PageRange          string `json:"pageRange"`
	PageSet            string `json:"pageSet"`
	Mirror             bool   `json:"mirror"`
	PhotoDateWatermark bool   `json:"photoDateWatermark"`
	WatermarkText      string `json:"watermarkText"`
	NumberUp           int    `json:"numberUp"`
	NumberUpLayout     string `json:"numberUpLayout"`
	PageBorder         string `json:"pageBorder"`

	CreatedAt string `json:"createdAt"`
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

	path, filename := record.StoredPath, record.Filename
	if raw := r.URL.Query().Get("source"); raw != "" {
		index, parseErr := strconv.Atoi(raw)
		if parseErr != nil || index < 0 || index >= len(record.PhotoLayout.Sources) {
			writeJSONError(w, 400, "invalid source index")
			return
		}
		source := record.PhotoLayout.Sources[index]
		path, filename = source.StoredPath, source.Filename
	} else if r.URL.Query().Get("variant") == "rendered" {
		if record.RenderedPath == "" {
			writeJSONError(w, 404, "print PDF not available")
			return
		}
		path, filename = record.RenderedPath, strings.TrimSuffix(record.Filename, filepath.Ext(record.Filename))+".pdf"
	}
	// os.OpenInRoot 将文件访问限制在 uploadDir 目录树内，即便 StoredPath 被污染
	// 成 ../ 逃逸路径也会被 OS 层拒绝（纵深防御，Go 1.24+）。
	f, err := os.OpenInRoot(uploadDir, filepath.FromSlash(path))
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

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, filename, stat.ModTime(), f)
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
		sources := make([]photoSourceResponse, 0, len(rec.PhotoLayout.Sources))
		for _, source := range rec.PhotoLayout.Sources {
			sources = append(sources, photoSourceResponse{Filename: source.Filename, Rotation: source.Rotation})
		}
		jobID := ""
		if rec.JobID.Valid {
			jobID = rec.JobID.String
		}
		resp = append(resp, printRecordResponse{
			PhotoSources: sources, ImagesPerPage: rec.PhotoLayout.PerPage, Invert: rec.PhotoLayout.Invert, CompressImages: rec.PhotoLayout.Compress, HasRenderedPDF: rec.RenderedPath != "",
			ID:         rec.ID,
			UserID:     rec.UserID,
			Username:   rec.Username,
			PrinterURI: rec.PrinterURI,
			Filename:   rec.Filename,
			Pages:      rec.Pages,
			JobID:      jobID,
			Status:     rec.Status,
			Sides:      rec.Sides,
			IsDuplex:   rec.IsDuplex,
			IsColor:    rec.IsColor,

			Copies:             rec.Copies,
			Orientation:        rec.Orientation,
			DriverOptions:      rec.DriverOptions,
			PaperSize:          rec.PaperSize,
			PaperType:          rec.PaperType,
			MediaSource:        rec.MediaSource,
			PrintScaling:       rec.PrintScaling,
			PageRange:          rec.PageRange,
			PageSet:            rec.PageSet,
			Mirror:             rec.Mirror,
			PhotoDateWatermark: rec.PhotoDateWatermark,
			WatermarkText:      rec.WatermarkText,
			NumberUp:           rec.NumberUp,
			NumberUpLayout:     rec.NumberUpLayout,
			PageBorder:         rec.PageBorder,

			CreatedAt: rec.CreatedAt,
		})
	}
	return resp
}

type reprintRequest struct {
	CompressImages     *bool             `json:"compressImages"`
	Invert             *bool             `json:"invert"`
	ImagesPerPage      *int              `json:"imagesPerPage"`
	DriverOptions      ipp.DriverOptions `json:"driverOptions"`
	Printer            string            `json:"printer"`
	Sides              string            `json:"sides"`
	Duplex             bool              `json:"duplex"`
	Color              bool              `json:"color"`
	Copies             int               `json:"copies"`
	Orientation        string            `json:"orientation"`
	PaperSize          string            `json:"paperSize"`
	PaperType          string            `json:"paperType"`
	MediaSource        string            `json:"mediaSource"`
	PrintScaling       string            `json:"printScaling"`
	PageRange          string            `json:"pageRange"`
	PageSet            string            `json:"pageSet"`
	Mirror             bool              `json:"mirror"`
	PhotoDateWatermark bool              `json:"photoDateWatermark"`
	WatermarkText      string            `json:"watermarkText"`

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
	var optionErr error
	req.Sides, optionErr = ipp.ResolveSides(req.Sides, req.Duplex)
	if optionErr != nil {
		writeJSONError(w, http.StatusBadRequest, optionErr.Error())
		return
	}
	req.Sides = ipp.SidesForPageSet(req.Sides, req.PageSet)
	req.Duplex = req.Sides != ipp.SidesOneSided
	req.DriverOptions, optionErr = ipp.ResolveDriverOptions(r.Context(), req.Printer, req.DriverOptions)
	if optionErr != nil {
		writeJSONError(w, http.StatusBadRequest, optionErr.Error())
		return
	}
	req.PaperSize = req.DriverOptions.PaperSize(req.PaperSize)
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

	if len(record.PhotoLayout.Sources) > 0 {
		reprintPhotos(w, r, record, req)
		return
	}
	origFile, err := os.OpenInRoot(uploadDir, filepath.FromSlash(record.StoredPath))
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
		outPath, cleanup, err := convertImageToPDF(storedAbs, req.Orientation, req.PaperSize, false, false, imagePDFOptions{
			PhotoDateWatermark: req.PhotoDateWatermark,
			Borderless:         req.DriverOptions.PageSize != nil && req.DriverOptions.PageSize.Borderless, PrintScaling: req.PrintScaling,
		})
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

	printScaling := photoPrintScaling(printPath, req.PrintScaling, printMime, req.Orientation, req.PaperSize)
	if watermark := strings.TrimSpace(req.WatermarkText); watermark != "" && printMime == "application/pdf" {
		wmPath, wmCleanup, wmErr := applyWatermarkToPDF(printPath, watermark)
		if wmErr != nil {
			log.Printf("[reprint] watermark failed: %v", wmErr)
		} else {
			defer wmCleanup()
			printPath = wmPath
		}
	}

	// 重打同样要走自定义百分比缩放：记录里存的是用户原始选择（自定义时为纯数字）。
	scaledPath, scaleCleanup, printScaling := resolveCustomScaling(printPath, printScaling, printMime, "reprint")
	printPath = scaledPath
	if scaleCleanup != nil {
		defer scaleCleanup()
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
			Sides:      req.Sides,
			IsDuplex:   req.Duplex,
			IsColor:    req.Color,

			Copies:             req.Copies,
			Orientation:        req.Orientation,
			DriverOptions:      req.DriverOptions,
			PaperSize:          req.PaperSize,
			PaperType:          req.PaperType,
			MediaSource:        req.MediaSource,
			PrintScaling:       req.PrintScaling,
			PageRange:          req.PageRange,
			PageSet:            req.PageSet,
			Mirror:             req.Mirror,
			PhotoDateWatermark: req.PhotoDateWatermark,
			WatermarkText:      req.WatermarkText,
			NumberUp:           req.NumberUp,
			NumberUpLayout:     req.NumberUpLayout,
			PageBorder:         req.PageBorder,

			CreatedAt: time.Now().UTC().Format(time.RFC3339),
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
		DriverOptions: req.DriverOptions,
		Sides:         req.Sides,
		IsDuplex:      req.Duplex,
		IsColor:       req.Color,
		Copies:        req.Copies,
		Orientation:   req.Orientation,
		PaperSize:     req.PaperSize,
		PaperType:     req.PaperType,
		MediaSource:   req.MediaSource,
		PrintScaling:  printScaling,
		PageRange:     req.PageRange,
		PageSet:       pageSet,
		Mirror:        req.Mirror,

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
		Sides:    req.Sides,
		IsDuplex: req.Duplex,
		IsColor:  req.Color,
		Copies:   req.Copies,
	})
}
