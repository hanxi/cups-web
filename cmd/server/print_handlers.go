package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if err := r.ParseMultipartForm(512 << 20); err != nil {
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
	mediaSource := r.FormValue("media_source")
	pageRange := r.FormValue("page_range")
	pageSet := r.FormValue("page_set")
	mirror := r.FormValue("mirror") == "true"
	watermarkText := strings.TrimSpace(r.FormValue("watermark_text"))

	// N-up (multiple pages per sheet). Default numberUp=1 means off; invalid or
	// unsupported values fall back to 1 (Issue #78).
	numberUp := 1
	if n, err := strconv.Atoi(r.FormValue("number_up")); err == nil {
		switch n {
		case 1, 2, 4, 6, 9, 16:
			numberUp = n
		}
	}
	numberUpLayout := r.FormValue("number_up_layout")
	pageBorder := r.FormValue("page_border")

	var saveHistory bool
	if err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		v, err := store.GetSettingInt(r.Context(), tx, store.SettingSaveHistory, 1)
		if err != nil {
			return err
		}
		saveHistory = v != 0
		return nil
	}); err != nil {
		saveHistory = true
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
	case fileKindPDF:
		// 默认不再对上传 PDF 走 gs 规范化，直接打印原始字节。
		// 如客户端有需要（例如 CJK 字体乱码），可先调用 /api/convert?normalize=true
		// 拿到规范化后的字节再回传到本接口。
		var cerr error
		pages, cerr = countPDFPages(storedAbs)
		if cerr != nil {
			log.Printf("[print] countPDFPages failed: %v", cerr)
			pages = 1
		}
		printPath = storedAbs
		printMime = "application/pdf"
		// 解析失败时降级 MIME，让 CUPS/IPP 自行识别
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
		outPath, cleanup, err := convertImageToPDF(storedAbs, orientation, paperSize)
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
		outPath, cleanup, err := convertTextToPDF(storedAbs, orientation, paperSize)
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

	if watermarkText != "" && printMime == "application/pdf" {
		wmPath, wmCleanup, wmErr := applyWatermarkToPDF(printPath, watermarkText)
		if wmErr != nil {
			log.Printf("[print] watermark failed: %v", wmErr)
		} else {
			defer wmCleanup()
			printPath = wmPath
		}
	}

	// Handle even-reverse: reorder PDF pages for manual duplex (even pages
	// in reverse order, with a blank page prepended when total is odd).
	if pageSet == "even-reverse" && printMime == "application/pdf" && pages > 1 {
		reorderedPath, reorderCleanup, err := reorderPDFForManualDuplex(printPath, pages, paperSize)
		if err != nil {
			log.Printf("[print] even-reverse reorder failed: %v, falling back to normal even", err)
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

	sess, _ := auth.GetSession(r)
	var recordID int64

	if saveHistory {
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
		MediaSource:  mediaSource,
		PageRange:    pageRange,
		PageSet:      pageSet,
		Mirror:       mirror,
		Pages:        pages,

		NumberUp:       numberUp,
		NumberUpLayout: numberUpLayout,
		PageBorder:     pageBorder,
	}

	job, err := ipp.SendPrintJob(printer, f, mime, sess.Username, fh.Filename, printOpts)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "print error: "+err.Error())
		return
	}

	if saveHistory && recordID > 0 {
		_ = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
			return store.UpdatePrintStatus(r.Context(), tx, recordID, "printed", job)
		})
	}
	if !saveHistory {
		_ = os.Remove(storedAbs)
		cRel := convertedRelPath(storedRel)
		if cRel != "" {
			_ = os.Remove(filepath.Join(uploadDir, filepath.FromSlash(cRel)))
		}
	}

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
