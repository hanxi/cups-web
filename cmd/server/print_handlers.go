package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"
)

type printResp struct {
	JobID           string `json:"jobId,omitempty"`
	OK              bool   `json:"ok"`
	Pages           int    `json:"pages"`
	CostCents       int64  `json:"costCents"`
	BalanceCents    int64  `json:"balanceCents"`
	MonthSpentCents int64  `json:"monthSpentCents"`
	YearSpentCents  int64  `json:"yearSpentCents"`
}

var (
	errInsufficientBalance = errors.New("insufficient balance")
	errMonthlyLimit        = errors.New("monthly limit exceeded")
	errYearlyLimit         = errors.New("yearly limit exceeded")
)

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
	case fileKindImage:
		outPath, cleanup, err := convertImageToPDF(storedAbs)
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
		outPath, cleanup, err := convertTextToPDF(storedAbs)
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
	var balanceAfter int64
	var monthSpent int64
	var yearSpent int64
	var costCents int64

	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		if err := normalizeUserPeriods(r.Context(), tx, &user, time.Now()); err != nil {
			return err
		}
		perPage, err := store.GetSettingInt(r.Context(), tx, store.SettingPerPageCents, store.DefaultPerPageCents)
		if err != nil {
			return err
		}
		costCents = int64(pages) * perPage
		if user.BalanceCents < costCents {
			return errInsufficientBalance
		}
		if user.MonthlyLimitCents > 0 && user.MonthSpentCents+costCents > user.MonthlyLimitCents {
			return errMonthlyLimit
		}
		if user.YearlyLimitCents > 0 && user.YearSpentCents+costCents > user.YearlyLimitCents {
			return errYearlyLimit
		}

		before := user.BalanceCents
		balanceAfter = before - costCents
		monthSpent = user.MonthSpentCents + costCents
		yearSpent = user.YearSpentCents + costCents
		if _, err := tx.ExecContext(r.Context(), `UPDATE users SET
            balance_cents = ?, month_spent_cents = ?, year_spent_cents = ?, updated_at = ?
            WHERE id = ?`, balanceAfter, monthSpent, yearSpent, time.Now().UTC().Format(time.RFC3339), user.ID,
		); err != nil {
			return err
		}

		rec := store.PrintRecord{
			UserID:             user.ID,
			PrinterURI:         printer,
			Filename:           fh.Filename,
			StoredPath:         storedRel,
			Pages:              pages,
			CostCents:          costCents,
			BalanceBeforeCents: before,
			BalanceAfterCents:  balanceAfter,
			MonthTotalCents:    monthSpent,
			YearTotalCents:     yearSpent,
			Status:             "queued",
			CreatedAt:          time.Now().UTC().Format(time.RFC3339),
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
		switch {
		case errors.Is(err, errInsufficientBalance):
			writeJSONError(w, http.StatusBadRequest, "余额不足以支付本次打印")
		case errors.Is(err, errMonthlyLimit):
			writeJSONError(w, http.StatusBadRequest, "超过月度限额")
		case errors.Is(err, errYearlyLimit):
			writeJSONError(w, http.StatusBadRequest, "超过年度限额")
		default:
			writeJSONError(w, http.StatusInternalServerError, "failed to create print record")
		}
		return
	}

	f, err := os.Open(printPath)
	if err != nil {
		_ = refundPrint(r.Context(), recordID, sess.UserID, costCents)
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
				_ = refundPrint(r.Context(), recordID, sess.UserID, costCents)
				writeJSONError(w, http.StatusInternalServerError, "failed to read file")
				return
			}
		}
	}

	job, err := ipp.SendPrintJob(printer, f, mime, sess.Username, fh.Filename)
	if err != nil {
		_ = refundPrint(r.Context(), recordID, sess.UserID, costCents)
		writeJSONError(w, http.StatusInternalServerError, "print error: "+err.Error())
		return
	}

	_ = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.UpdatePrintStatus(r.Context(), tx, recordID, "printed", job)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(printResp{
		JobID:           job,
		OK:              true,
		Pages:           pages,
		CostCents:       costCents,
		BalanceCents:    balanceAfter,
		MonthSpentCents: monthSpent,
		YearSpentCents:  yearSpent,
	})
}
