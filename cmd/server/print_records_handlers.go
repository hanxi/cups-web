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

	// 完整打印参数快照，供前端「重新打印」预填第一次的设置（Issue #68）。
	Copies         int    `json:"copies"`
	Orientation    string `json:"orientation"`
	PaperSize      string `json:"paperSize"`
	PaperType      string `json:"paperType"`
	MediaSource    string `json:"mediaSource"`
	PrintScaling   string `json:"printScaling"`
	PageRange      string `json:"pageRange"`
	PageSet        string `json:"pageSet"`
	Mirror         bool   `json:"mirror"`
	WatermarkText  string `json:"watermarkText"`
	NumberUp       int    `json:"numberUp"`
	NumberUpLayout string `json:"numberUpLayout"`
	PageBorder     string `json:"pageBorder"`

	CreatedAt string `json:"createdAt"`
}

func printRecordsHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		return
	}
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный диапазон дат")
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
		writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить записи")
		return
	}
	writeJSON(w, resp)
}

func adminPrintRecordsHandler(w http.ResponseWriter, r *http.Request) {
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный диапазон дат")
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
		writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить записи")
		return
	}
	writeJSON(w, resp)
}

func printRecordFileHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный идентификатор записи")
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
			writeJSONError(w, http.StatusNotFound, "запись не найдена")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить запись")
		return
	}
	if sess.Role != store.RoleAdmin && record.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "доступ запрещен")
		return
	}

	f, err := os.OpenInRoot(uploadDir, filepath.FromSlash(record.StoredPath))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "файл не найден")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось получить информацию о файле")
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

			Copies:         rec.Copies,
			Orientation:    rec.Orientation,
			PaperSize:      rec.PaperSize,
			PaperType:      rec.PaperType,
			MediaSource:    rec.MediaSource,
			PrintScaling:   rec.PrintScaling,
			PageRange:      rec.PageRange,
			PageSet:        rec.PageSet,
			Mirror:         rec.Mirror,
			WatermarkText:  rec.WatermarkText,
			NumberUp:       rec.NumberUp,
			NumberUpLayout: rec.NumberUpLayout,
			PageBorder:     rec.PageBorder,

			CreatedAt: rec.CreatedAt,
		})
	}
	return resp
}

type reprintRequest struct {
	Printer      string `json:"printer"`
	Duplex       bool   `json:"duplex"`
	Color        bool   `json:"color"`
	Copies       int    `json:"copies"`
	Orientation   string `json:"orientation"`
	PaperSize     string `json:"paperSize"`
	PaperType     string `json:"paperType"`
	MediaSource   string `json:"mediaSource"`
	PrintScaling  string `json:"printScaling"`
	PageRange     string `json:"pageRange"`
	PageSet       string `json:"pageSet"`
	Mirror        bool   `json:"mirror"`
	WatermarkText string `json:"watermarkText"`

	NumberUp       int    `json:"numberUp"`
	NumberUpLayout string `json:"numberUpLayout"`
	PageBorder     string `json:"pageBorder"`
}

func reprintHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "не авторизован")
		return
	}

	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверный идентификатор записи")
		return
	}

	var req reprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "неверные данные запроса")
		return
	}
	if req.Printer == "" {
		writeJSONError(w, http.StatusBadRequest, "отсутствует поле printer")
		return
	}
	if req.Copies < 1 {
		req.Copies = 1
	}
	switch req.NumberUp {
	case 1, 2, 4, 6, 9, 16:
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
			writeJSONError(w, http.StatusNotFound, "запись не найдена")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "не удалось загрузить запись")
		return
	}

	if sess.Role != store.RoleAdmin && record.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "доступ запрещен")
		return
	}

	origFile, err := os.OpenInRoot(uploadDir, filepath.FromSlash(record.StoredPath))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "исходный файл не найден, возможно был удален при очистке")
		return
	}
	defer origFile.Close()

	storedRel, storedAbs, err := saveUploadedFile(origFile, record.Filename, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось скопировать файл")
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
			writeJSONError(w, http.StatusBadRequest, "ошибка преобразования")
			return
		}
		pages, err = countPDFPages(outPath)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "не удалось определить количество страниц")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "не удалось сохранить преобразованный файл")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	case fileKindOFD:
		outPath, cleanup, err := convertOFDToPDF(countCtx, storedAbs)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "ошибка преобразования")
			return
		}
		pages, err = countPDFPages(outPath)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "не удалось определить количество страниц")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "не удалось сохранить преобразованный файл")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	case fileKindImage:
		outPath, cleanup, err := convertImageToPDF(storedAbs, req.Orientation, req.PaperSize)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "ошибка преобразования")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "не удалось сохранить преобразованный файл")
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
			writeJSONError(w, http.StatusBadRequest, "не удалось определить количество страниц")
			return
		}
		outPath, cleanup, err := convertTextToPDF(storedAbs, req.Orientation, req.PaperSize)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "ошибка преобразования")
			return
		}
		_, convertedAbs, err := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if err != nil {
			cleanup()
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusInternalServerError, "не удалось сохранить преобразованный файл")
			return
		}
		printPath = convertedAbs
		printCleanup = cleanup
		printMime = "application/pdf"
	default:
		pages, _, err = countPages(countCtx, storedAbs, record.Filename)
		if err != nil {
			_ = os.Remove(storedAbs)
			writeJSONError(w, http.StatusBadRequest, "не удалось определить количество страниц")
			return
		}
	}
	if pages < 1 {
		pages = 1
	}
	if printCleanup != nil {
		defer printCleanup()
	}

	if watermark := strings.TrimSpace(req.WatermarkText); watermark != "" && printMime == "application/pdf" {
		wmPath, wmCleanup, wmErr := applyWatermarkToPDF(printPath, watermark)
		if wmErr != nil {
			log.Printf("[reprint] watermark failed: %v", wmErr)
		} else {
			defer wmCleanup()
			printPath = wmPath
		}
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

			Copies:         req.Copies,
			Orientation:    req.Orientation,
			PaperSize:      req.PaperSize,
			PaperType:      req.PaperType,
			MediaSource:    req.MediaSource,
			PrintScaling:   req.PrintScaling,
			PageRange:      req.PageRange,
			PageSet:        req.PageSet,
			Mirror:         req.Mirror,
			WatermarkText:  req.WatermarkText,
			NumberUp:       req.NumberUp,
			NumberUpLayout: req.NumberUpLayout,
			PageBorder:     req.PageBorder,

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
		writeJSONError(w, http.StatusInternalServerError, "не удалось создать запись печати")
		return
	}

	f, err := os.Open(printPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "не удалось открыть файл")
		return
	}
	defer f.Close()

	mimeType := printMime
	if mimeType == "" {
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			mimeType = http.DetectContentType(buf[:n])
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "не удалось прочитать файл")
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
		MediaSource:  req.MediaSource,
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
		writeJSONError(w, http.StatusInternalServerError, "ошибка печати: "+err.Error())
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