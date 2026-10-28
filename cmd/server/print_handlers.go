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
	Sides    string `json:"sides"`
	IsDuplex bool   `json:"isDuplex"`
	IsColor  bool   `json:"isColor"`
	Copies   int    `json:"copies"`
}

func printHandler(w http.ResponseWriter, r *http.Request) {
	maxPages, _ := getPrintLimits(r.Context())
	applyUploadLimit(w, r)
	// Expect multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isMaxBytesError(err) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "文件超出管理员设置的大小上限")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	defer r.MultipartForm.RemoveAll()
	headers := r.MultipartForm.File["file"]
	multiInput := len(r.MultipartForm.File["files"]) > 0
	if multiInput {
		if len(headers) > 0 {
			writeJSONError(w, 400, "file 和 files 不能混用")
			return
		}
		headers = r.MultipartForm.File["files"]
	}
	if len(headers) < 1 || len(headers) > 100 || (!multiInput && len(headers) != 1) {
		writeJSONError(w, 400, "文件数量无效")
		return
	}
	fh := headers[0]
	file, err := fh.Open()
	if err != nil {
		writeJSONError(w, 400, "failed to open file")
		return
	}
	defer file.Close()
	displayFilename := fh.Filename

	printer := r.FormValue("printer")
	if printer == "" {
		writeJSONError(w, http.StatusBadRequest, "missing printer field")
		return
	}

	sides, err := ipp.ResolveSides(r.FormValue("sides"), r.FormValue("duplex") == "true")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	isDuplex := sides != ipp.SidesOneSided
	isColor := r.FormValue("color") == "true"
	photoDateWatermark := r.FormValue("photo_date_watermark") == "true"

	// Extended print options
	copiesStr := r.FormValue("copies")
	copies := 1
	if copiesStr != "" {
		if n, err := strconv.Atoi(copiesStr); err == nil && n > 0 {
			copies = n
		}
	}
	orientation := r.FormValue("orientation")
	driverOpts, err := driverOptionsFromForm(r, printer)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	paperSize := driverOpts.PaperSize(r.FormValue("paper_size"))
	paperType := r.FormValue("paper_type")
	printScaling := r.FormValue("print_scaling")
	mediaSource := r.FormValue("media_source")
	pageRange := r.FormValue("page_range")
	pageSet := r.FormValue("page_set")
	// even-reverse / custom-scale 分支下方会改写 pageSet/printScaling，落库要保留用户的原始选择。
	origPageSet := pageSet
	origPrintScaling := printScaling
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

	var photoLayout store.PhotoLayout
	var renderedRel string
	pendingPaths := []string{storedRel, convertedRelPath(storedRel)}
	keepAssets := false
	defer func() {
		if !keepAssets {
			removePrintAssets(uploadDir, pendingPaths)
		}
	}()
	countCtx, cancel := convertTimeoutContext(r.Context())
	defer cancel()
	printPath := storedAbs
	var printCleanup func()
	printMime := ""
	var pages int
	kind := detectFileKind(storedAbs, fh.Filename)
	if multiInput && kind != fileKindImage {
		writeJSONError(w, 400, "files 仅支持照片")
		return
	}

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
			writeJSONError(w, http.StatusBadRequest, convertErrMsg(err))
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
			writeJSONError(w, http.StatusBadRequest, convertErrMsg(err))
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
		photoLayout, err = photoLayoutFromForm(r, headers)
		if err != nil {
			writeJSONError(w, 400, err.Error())
			return
		}
		photoLayout.Sources[0].StoredPath = storedRel
		for i, header := range headers[1:] {
			src, openErr := header.Open()
			if openErr != nil {
				writeJSONError(w, 400, openErr.Error())
				return
			}
			rel, _, saveErr := saveUploadedFile(src, header.Filename, uploadDir)
			src.Close()
			if saveErr != nil {
				writeJSONError(w, 500, "failed to save photo")
				return
			}
			pendingPaths = append(pendingPaths, rel)
			photoLayout.Sources[i+1].StoredPath = rel
		}
		outPath, cleanup, convertErr := renderPhotoSources(uploadDir, photoLayout, orientation, paperSize, imagePDFOptions{
			PhotoDateWatermark: photoDateWatermark,
			Borderless:         driverOpts.PageSize != nil && driverOpts.PageSize.Borderless, PrintScaling: printScaling,
		})
		if convertErr != nil {
			writeJSONError(w, 400, "conversion failed: "+convertErr.Error())
			return
		}
		convertedRel, convertedAbs, saveErr := saveConvertedPDFToUploads(outPath, storedRel, uploadDir)
		if saveErr != nil {
			cleanup()
			writeJSONError(w, 500, "failed to save converted file")
			return
		}
		printPath, printCleanup, printMime = convertedAbs, cleanup, "application/pdf"
		pages, err = countPDFPages(outPath)
		if err != nil {
			cleanup()
			writeJSONError(w, 400, "failed to read photo pages")
			return
		}
		if len(headers) > 1 {
			storedRel, storedAbs = convertedRel, convertedAbs
			displayFilename = strings.TrimSpace(r.FormValue("name"))
			if displayFilename == "" {
				displayFilename = "合并照片.pdf"
			}
			if !strings.HasSuffix(strings.ToLower(displayFilename), ".pdf") {
				displayFilename += ".pdf"
			}
		}
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

	// 页数硬上限（Issue #72）：0=不限；超过则拒绝，422 表示"内容合法但被业务规则拒绝"。
	if maxPages > 0 && int64(pages) > maxPages {
		_ = os.Remove(storedAbs)
		writeJSONError(w, http.StatusUnprocessableEntity,
			"文件页数超出管理员设置的上限")
		return
	}

	printScaling = photoPrintScaling(printPath, printScaling, printMime, orientation, paperSize)
	if watermarkText != "" && printMime == "application/pdf" {
		wmPath, wmCleanup, wmErr := applyWatermarkToPDF(printPath, watermarkText)
		if wmErr != nil {
			log.Printf("[print] watermark failed: %v", wmErr)
		} else {
			defer wmCleanup()
			printPath = wmPath
		}
	}

	// 自定义百分比缩放：先用 gs 把 PDF 内容预缩放，再把 print-scaling 换成合法 keyword
	scaledPath, scaleCleanup, effectiveScaling := resolveCustomScaling(printPath, printScaling, printMime, "print")
	printPath = scaledPath
	printScaling = effectiveScaling
	if scaleCleanup != nil {
		defer scaleCleanup()
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

	// 手动双面（奇/偶 或 偶数倒序）语义上必然单面输出：如果用户没在前端切回单面，
	// 这里强制回落，避免 CUPS pdftopdf 用空白页填补被过滤掉的另一面（issue #109）。
	sides = ipp.SidesForPageSet(sides, origPageSet)
	isDuplex = sides != ipp.SidesOneSided

	if saveHistory && len(photoLayout.Sources) > 0 {
		renderedRel, err = saveRenderedPDF(printPath, storedRel, uploadDir)
		if err != nil {
			writeJSONError(w, 500, "failed to save print PDF")
			return
		}
		pendingPaths = append(pendingPaths, renderedRel)
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
				UserID:      user.ID,
				PrinterURI:  printer,
				Filename:    displayFilename,
				PhotoLayout: photoLayout, RenderedPath: renderedRel,
				StoredPath: storedRel,
				Pages:      pages,
				Status:     "queued",
				Sides:      sides,
				IsDuplex:   isDuplex,
				IsColor:    isColor,

				Copies:             copies,
				Orientation:        orientation,
				DriverOptions:      driverOpts,
				PaperSize:          paperSize,
				PaperType:          paperType,
				MediaSource:        mediaSource,
				PrintScaling:       origPrintScaling,
				PageRange:          pageRange,
				PageSet:            origPageSet,
				Mirror:             mirror,
				PhotoDateWatermark: photoDateWatermark,
				WatermarkText:      watermarkText,
				NumberUp:           numberUp,
				NumberUpLayout:     numberUpLayout,
				PageBorder:         pageBorder,

				CreatedAt: time.Now().UTC().Format(time.RFC3339),
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

	if saveHistory && recordID > 0 {
		keepAssets = true
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
		DriverOptions: driverOpts,
		Sides:         sides,
		IsDuplex:      isDuplex,
		IsColor:       isColor,
		Copies:        copies,
		Orientation:   orientation,
		PaperSize:     paperSize,
		PaperType:     paperType,
		PrintScaling:  printScaling,
		MediaSource:   mediaSource,
		PageRange:     pageRange,
		PageSet:       pageSet,
		Mirror:        mirror,

		NumberUp:       numberUp,
		NumberUpLayout: numberUpLayout,
		PageBorder:     pageBorder,
	}

	job, err := ipp.SendPrintJob(printer, f, mime, sess.Username, displayFilename, printOpts)
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
		Sides:    sides,
		IsDuplex: isDuplex,
		IsColor:  isColor,
		Copies:   copies,
	})
}
