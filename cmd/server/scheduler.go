package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cups-web/internal/ipp"
	"cups-web/internal/store"
)

// schedulerTickInterval 是调度器扫描到期任务的心跳间隔。
// 30s 已足够精细：定时打印属于分钟级业务，扫描频次再高只会加重 SQLite 压力。
const schedulerTickInterval = 30 * time.Second

// scheduleTimeLayout 是 HH:MM 的解析格式，与前端表单一致。
const scheduleTimeLayout = "15:04"

// startScheduler 启动后台调度器 goroutine。与 startMaintenance 并列，进程
// 生命周期内常驻。执行串行以避免多个任务并发抢 CUPS 导致队列错乱。
func startScheduler(s *store.Store, uploads string) {
	go func() {
		// 首次立即扫一次：进程重启后马上处理任何过期任务（会被判为 skipped）。
		schedulerTick(s, uploads)
		ticker := time.NewTicker(schedulerTickInterval)
		defer ticker.Stop()
		for range ticker.C {
			schedulerTick(s, uploads)
		}
	}()
}

func schedulerTick(s *store.Store, uploads string) {
	nowLocal := time.Now()
	nowUTC := nowLocal.UTC().Format(time.RFC3339)

	var due []store.ScheduledPrint
	err := s.WithTx(context.Background(), true, func(tx *sql.Tx) error {
		list, err := store.ListDueScheduledPrints(context.Background(), tx, nowUTC)
		if err != nil {
			return err
		}
		due = list
		return nil
	})
	if err != nil {
		log.Printf("[scheduler] list due failed: %v", err)
		return
	}

	for _, task := range due {
		runScheduledTask(s, uploads, task, nowLocal)
	}
}

// runScheduledTask 与手动执行共用任务锁，拿锁后重新读取，防止使用过期扫描结果。
func runScheduledTask(s *store.Store, uploads string, task store.ScheduledPrint, now time.Time) {
	if !acquireScheduledTask(task.ID, true) {
		return
	}
	defer releaseScheduledTask(task.ID)
	var current store.ScheduledPrint
	if err := s.WithTx(context.Background(), true, func(tx *sql.Tx) error {
		var err error
		current, err = store.GetScheduledPrintByID(context.Background(), tx, task.ID)
		return err
	}); err != nil {
		return
	}
	if !current.Enabled || scheduledQueueComplete(current) {
		return
	}
	next, err := time.Parse(time.RFC3339, current.NextRunAt)
	if err != nil || next.After(now) {
		return
	}
	if err := executeScheduledOccurrence(s, uploads, current, now, false, executeScheduled); err != nil {
		log.Printf("[scheduler] task #%d failed: %v", current.ID, err)
	}
}

func persistScheduledResult(s *store.Store, id int64, lastRunAt, lastStatus, lastError, nextRunAt string, enabled bool) error {
	return s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
		return store.UpdateScheduledPrintAfterRun(context.Background(), tx, id, lastRunAt, lastStatus, lastError, nextRunAt, enabled)
	})
}

// executeScheduled 复用 reprint 相同的转换/水印/缩放/IPP 提交链路，但独立成一个函数
// 避免依赖 HTTP request 上下文。返回值 status ∈ {printed, failed}。
func executeScheduled(s *store.Store, uploads string, task store.ScheduledPrint) (string, error) {
	ctx := context.Background()
	sides, err := ipp.ResolveSides(task.Sides, task.IsDuplex)
	if err != nil {
		return "failed", err
	}
	sides = ipp.SidesForPageSet(sides, task.PageSet)
	driverOpts, err := ipp.ResolveDriverOptions(ctx, task.PrinterURI, task.DriverOptions)
	if err != nil {
		return "failed", err
	}
	task.DriverOptions = driverOpts
	task.PaperSize = driverOpts.PaperSize(task.PaperSize)

	origPath, err := os.OpenInRoot(uploads, filepath.FromSlash(task.StoredPath))
	if err != nil {
		return "failed", fmt.Errorf("原始文件缺失：%w", err)
	}
	orientation, orientationErr := resolveScheduledOrientation(task, origPath)
	_ = origPath.Close()
	if orientationErr != nil {
		return "failed", fmt.Errorf("判断照片方向失败：%w", orientationErr)
	}
	// 只修改本张执行的副本；队列继续保存 auto，打印历史保存实际方向。
	task.Orientation = orientation
	absPath := filepath.Join(uploads, filepath.FromSlash(task.StoredPath))

	// 独立超时：转换 + 打印链路加起来预留 5 分钟。
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	printPath := absPath
	var printCleanup func()
	printMime := ""
	var pages int
	kind := detectFileKind(absPath, task.Filename)
	switch kind {
	case fileKindPDF:
		p, cerr := countPDFPages(absPath)
		if cerr != nil {
			log.Printf("[scheduler] countPDFPages failed: %v", cerr)
			p = 1
			printMime = "application/octet-stream"
		} else {
			printMime = "application/pdf"
		}
		pages = p
	case fileKindOffice:
		outPath, cleanup, err := convertOfficeToPDF(ctx, absPath)
		if err != nil {
			return "failed", fmt.Errorf("office 转换失败：%w", err)
		}
		printCleanup = cleanup
		printPath = outPath
		printMime = "application/pdf"
		pages, err = countPDFPages(outPath)
		if err != nil {
			return "failed", fmt.Errorf("解析页数失败：%w", err)
		}
	case fileKindOFD:
		outPath, cleanup, err := convertOFDToPDF(ctx, absPath)
		if err != nil {
			return "failed", fmt.Errorf("ofd 转换失败：%w", err)
		}
		printCleanup = cleanup
		printPath = outPath
		printMime = "application/pdf"
		pages, err = countPDFPages(outPath)
		if err != nil {
			return "failed", fmt.Errorf("解析页数失败：%w", err)
		}
	case fileKindImage:
		outPath, cleanup, err := convertImageToPDF(absPath, task.Orientation, task.PaperSize, false, false, imagePDFOptions{
			PhotoDateWatermark: task.PhotoDateWatermark,
			Borderless:         task.DriverOptions.PageSize != nil && task.DriverOptions.PageSize.Borderless, PrintScaling: task.PrintScaling,
		})
		if err != nil {
			return "failed", fmt.Errorf("图片转换失败：%w", err)
		}
		printCleanup = cleanup
		printPath = outPath
		printMime = "application/pdf"
		pages = 1
	case fileKindText:
		p, err := estimateTextPages(absPath)
		if err != nil {
			return "failed", fmt.Errorf("读取页数失败：%w", err)
		}
		outPath, cleanup, err := convertTextToPDF(absPath, task.Orientation, task.PaperSize)
		if err != nil {
			return "failed", fmt.Errorf("文本转换失败：%w", err)
		}
		printCleanup = cleanup
		printPath = outPath
		printMime = "application/pdf"
		pages = p
	default:
		p, _, err := countPages(ctx, absPath, task.Filename)
		if err != nil {
			return "failed", fmt.Errorf("解析页数失败：%w", err)
		}
		pages = p
	}
	if pages < 1 {
		pages = 1
	}
	if printCleanup != nil {
		defer printCleanup()
	}

	printScaling := photoPrintScaling(printPath, task.PrintScaling, printMime, task.Orientation, task.PaperSize)
	if watermark := strings.TrimSpace(task.WatermarkText); watermark != "" && printMime == "application/pdf" {
		wmPath, wmCleanup, wmErr := applyWatermarkToPDF(printPath, watermark)
		if wmErr != nil {
			log.Printf("[scheduler] watermark failed: %v", wmErr)
		} else {
			defer wmCleanup()
			printPath = wmPath
		}
	}

	scaledPath, scaleCleanup, effectiveScaling := resolveCustomScaling(printPath, printScaling, printMime, "scheduler")
	printPath = scaledPath
	if scaleCleanup != nil {
		defer scaleCleanup()
	}

	pageSet := task.PageSet
	if pageSet == "even-reverse" && printMime == "application/pdf" && pages > 1 {
		reorderedPath, reorderCleanup, rerr := reorderPDFForManualDuplex(printPath, pages, task.PaperSize)
		if rerr != nil {
			log.Printf("[scheduler] even-reverse reorder failed: %v", rerr)
			pageSet = "even"
		} else {
			defer reorderCleanup()
			printPath = reorderedPath
			if rp, _ := countPDFPages(reorderedPath); rp > 0 {
				pages = rp
			}
			pageSet = ""
		}
	}

	isDuplex := sides != ipp.SidesOneSided

	recordStoredPath := task.StoredPath
	var recordPhotoLayout store.PhotoLayout
	var renderedRel string
	keepHistoryAssets := false
	var historyAssets []string
	defer func() {
		if !keepHistoryAssets {
			removePrintAssets(uploads, historyAssets)
		}
	}()
	if kind == fileKindImage {
		original, openErr := os.OpenInRoot(uploads, filepath.FromSlash(task.StoredPath))
		if openErr != nil {
			return "failed", openErr
		}
		recordStoredPath, _, err = saveUploadedFile(original, task.Filename, uploads)
		original.Close()
		if err != nil {
			return "failed", err
		}
		historyAssets = append(historyAssets, recordStoredPath)
		recordPhotoLayout = store.PhotoLayout{PerPage: 1, Sources: []store.PhotoSource{{Filename: task.Filename, StoredPath: recordStoredPath}}}
		renderedRel, err = saveRenderedPDF(printPath, recordStoredPath, uploads)
		if err != nil {
			return "failed", err
		}
		historyAssets = append(historyAssets, renderedRel)
	}
	// 打印前先在 print_jobs 里落一条历史，让用户在打印历史页看到"来自定时任务"的记录。
	var recordID int64
	if err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		rec := store.PrintRecord{
			UserID:      task.UserID,
			PrinterURI:  task.PrinterURI,
			Filename:    task.Filename,
			StoredPath:  recordStoredPath,
			PhotoLayout: recordPhotoLayout, RenderedPath: renderedRel,
			Pages:    pages,
			Status:   "queued",
			Sides:    sides,
			IsDuplex: isDuplex,
			IsColor:  task.IsColor,

			Copies:             task.Copies,
			Orientation:        task.Orientation,
			DriverOptions:      task.DriverOptions,
			PaperSize:          task.PaperSize,
			PaperType:          task.PaperType,
			MediaSource:        task.MediaSource,
			PrintScaling:       task.PrintScaling,
			PageRange:          task.PageRange,
			PageSet:            task.PageSet,
			Mirror:             task.Mirror,
			PhotoDateWatermark: task.PhotoDateWatermark,
			WatermarkText:      task.WatermarkText,
			NumberUp:           task.NumberUp,
			NumberUpLayout:     task.NumberUpLayout,
			PageBorder:         task.PageBorder,

			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		id, err := store.InsertPrintRecord(ctx, tx, &rec)
		if err != nil {
			return err
		}
		recordID = id
		return nil
	}); err != nil {
		return "failed", fmt.Errorf("落打印历史失败：%w", err)
	}

	keepHistoryAssets = true
	f, err := os.Open(printPath)
	if err != nil {
		return "failed", fmt.Errorf("打开待打印文件失败：%w", err)
	}
	defer f.Close()

	mimeType := printMime
	if mimeType == "" {
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			mimeType = http.DetectContentType(buf[:n])
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return "failed", fmt.Errorf("回退文件读取失败：%w", err)
			}
		}
	}

	printOpts := ipp.PrintJobOptions{
		DriverOptions: task.DriverOptions,
		Sides:         sides,
		IsDuplex:      isDuplex,
		IsColor:       task.IsColor,
		Copies:        task.Copies,
		Orientation:   task.Orientation,
		PaperSize:     task.PaperSize,
		PaperType:     task.PaperType,
		MediaSource:   task.MediaSource,
		PrintScaling:  effectiveScaling,
		PageRange:     task.PageRange,
		PageSet:       pageSet,
		Mirror:        task.Mirror,

		NumberUp:       task.NumberUp,
		NumberUpLayout: task.NumberUpLayout,
		PageBorder:     task.PageBorder,
	}

	job, err := ipp.SendPrintJob(task.PrinterURI, f, mimeType, task.Username, task.Filename, printOpts)
	if err != nil {
		if ipp.IsSubmissionUncertain(err) {
			// Keep the history record queued; the printer may have accepted it.
			return "running", fmt.Errorf("IPP 提交结果未确认：%w", err)
		}
		_ = s.WithTx(ctx, false, func(tx *sql.Tx) error {
			return store.UpdatePrintStatus(ctx, tx, recordID, "failed", "")
		})
		return "failed", fmt.Errorf("IPP 提交失败：%w", err)
	}
	_ = s.WithTx(ctx, false, func(tx *sql.Tx) error {
		return store.UpdatePrintStatus(ctx, tx, recordID, "printed", job)
	})
	return "printed", nil
}

// advanceNextRun 根据调度类型计算下一次触发时间；对 once 任务归档（enabled=false，
// next_run_at 保留最后一次执行点用于展示）。返回值均使用 UTC RFC3339。
func advanceNextRun(task store.ScheduledPrint, from time.Time) (string, bool) {
	if task.ScheduleType == store.ScheduleOnce {
		return from.UTC().Format(time.RFC3339), false
	}
	next, ok := computeNextRun(task, from.Add(time.Minute))
	if !ok {
		return from.UTC().Format(time.RFC3339), false
	}
	return next.UTC().Format(time.RFC3339), true
}

// computeNextRun 从 after（本地时间）之后寻找下一个触发时间点。
// 返回本地时间；调用方负责转 UTC 落库。
func computeNextRun(task store.ScheduledPrint, after time.Time) (time.Time, bool) {
	loc := time.Local
	after = after.In(loc)
	if task.StartDate != "" {
		start, err := time.ParseInLocation("2006-01-02", task.StartDate, loc)
		if err != nil {
			return time.Time{}, false
		}
		if after.Before(start) {
			after = start.Add(-time.Nanosecond)
		}
	}

	switch task.ScheduleType {
	case store.ScheduleOnce:
		if task.RunAt == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339, task.RunAt)
		if err != nil {
			return time.Time{}, false
		}
		return t.In(loc), true

	case store.ScheduleDaily:
		h, m, ok := parseScheduleTime(task.ScheduleTime)
		if !ok {
			return time.Time{}, false
		}
		cand := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, loc)
		if !cand.After(after) {
			cand = cand.AddDate(0, 0, 1)
		}
		return cand, true

	case store.ScheduleWeekly:
		h, m, ok := parseScheduleTime(task.ScheduleTime)
		if !ok {
			return time.Time{}, false
		}
		target := (task.ScheduleWeekday%7 + 7) % 7
		cand := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, loc)
		daysAhead := (target - int(cand.Weekday()) + 7) % 7
		cand = cand.AddDate(0, 0, daysAhead)
		if !cand.After(after) {
			cand = cand.AddDate(0, 0, 7)
		}
		return cand, true

	case store.ScheduleMonthly:
		h, m, ok := parseScheduleTime(task.ScheduleTime)
		if !ok {
			return time.Time{}, false
		}
		day := task.ScheduleDay
		if day < 1 {
			day = 1
		}
		if day > 31 {
			day = 31
		}
		year, month := after.Year(), after.Month()
		// 尝试当月对应的日期；若当月不存在该日（比如 2 月 30 日），跳到下一个存在
		// 的月份，避免"某月不触发"和 Go 时间归一化把 2/30 变成 3/2 的坑。
		for i := 0; i < 12; i++ {
			cand := makeMonthDay(year, month, day, h, m, loc)
			if cand.After(after) {
				return cand, true
			}
			// 推进到下个月
			month++
			if month > 12 {
				month = 1
				year++
			}
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}

// makeMonthDay 返回 year-month 里第 day 天 h:m；若 day 超过月末则回退到月末。
func makeMonthDay(year int, month time.Month, day, h, m int, loc *time.Location) time.Time {
	// 用 day=0 拿到 month 的最后一天：Go 会把 (year, month, 0) 归一化为上月末日。
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, h, m, 0, 0, loc)
}

func parseScheduleTime(s string) (int, int, bool) {
	t, err := time.Parse(scheduleTimeLayout, strings.TrimSpace(s))
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// resolveScheduledOrientation 按原图的实际像素宽高判断，不读取 EXIF 方向。
// 调用方提供已在 uploads 根目录内安全打开的原图。
func resolveScheduledOrientation(task store.ScheduledPrint, source io.Reader) (string, error) {
	if task.Orientation != "auto" {
		return task.Orientation, nil
	}
	if len(task.Files) == 0 {
		return "", errors.New("自动方向仅支持照片定时队列")
	}
	cfg, format, err := image.DecodeConfig(source)
	if err != nil {
		return "", fmt.Errorf("读取图片尺寸失败：%w", err)
	}
	if (format != "jpeg" && format != "png" && format != "gif") || cfg.Width <= 0 || cfg.Height <= 0 {
		return "", errors.New("照片队列仅支持有效的 JPG、PNG、GIF 图片")
	}
	if cfg.Width > cfg.Height {
		return "landscape", nil
	}
	return "portrait", nil
}

// ensureScheduleDefaults 修剪 store.ScheduledPrint 字段，保证枚举合法。
// 保持业务逻辑集中在一处，handler 与 scheduler 都可以调用。
func ensureScheduleDefaults(rec *store.ScheduledPrint) error {
	if rec.PhotosPerRun == 0 {
		rec.PhotosPerRun = 1
	}
	if rec.PhotosPerRun < 1 || rec.PhotosPerRun > 100 {
		return fmt.Errorf("每次打印张数应为 1–100")
	}
	if rec.StartDate != "" {
		if _, err := time.Parse("2006-01-02", rec.StartDate); err != nil {
			return errors.New("start_date 必须是 YYYY-MM-DD")
		}
	}
	if len(rec.Files) > 0 && rec.ScheduleType == store.ScheduleOnce {
		return errors.New("照片队列请选择每天、每周或每月")
	}
	if scheduledQueueComplete(*rec) && rec.Enabled {
		return errors.New("照片已全部提交，请新建任务；或开启循环后继续")
	}
	if rec.PrinterURI == "" {
		return errors.New("printer 不能为空")
	}
	if rec.Filename == "" || rec.StoredPath == "" {
		return errors.New("文件缺失")
	}
	if rec.Copies < 1 {
		rec.Copies = 1
	}
	switch rec.NumberUp {
	case 1, 2, 4, 6, 9, 16:
	default:
		rec.NumberUp = 1
	}
	if rec.Orientation == "" {
		rec.Orientation = "portrait"
	}
	switch rec.Orientation {
	case "portrait", "landscape":
	case "auto":
		if len(rec.Files) == 0 {
			return errors.New("自动方向仅支持照片定时队列")
		}
	default:
		return fmt.Errorf("非法的 orientation: %q", rec.Orientation)
	}
	if rec.PaperSize == "" {
		rec.PaperSize = "A4"
	}
	if rec.PaperType == "" {
		rec.PaperType = "plain"
	}
	if rec.MediaSource == "" {
		rec.MediaSource = "auto"
	}
	if rec.PrintScaling == "" {
		rec.PrintScaling = "fit"
	}
	if rec.PageSet == "" {
		rec.PageSet = "all"
	}
	if rec.NumberUpLayout == "" {
		rec.NumberUpLayout = "lrtb"
	}
	if rec.PageBorder == "" {
		rec.PageBorder = "none"
	}
	switch rec.ScheduleType {
	case store.ScheduleOnce, store.ScheduleDaily, store.ScheduleWeekly, store.ScheduleMonthly:
	default:
		return fmt.Errorf("非法的 schedule_type: %q", rec.ScheduleType)
	}
	if rec.ScheduleType != store.ScheduleOnce {
		if _, _, ok := parseScheduleTime(rec.ScheduleTime); !ok {
			return errors.New("schedule_time 必须是 HH:MM")
		}
	}
	if rec.ScheduleType == store.ScheduleWeekly {
		if rec.ScheduleWeekday < 0 || rec.ScheduleWeekday > 6 {
			return errors.New("schedule_weekday 必须在 0-6")
		}
	}
	if rec.ScheduleType == store.ScheduleMonthly {
		if rec.ScheduleDay < 1 || rec.ScheduleDay > 31 {
			return errors.New("schedule_day 必须在 1-31")
		}
	}
	return nil
}
