package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/gorilla/mux"
)

// scheduledSubDir 保存定时任务源文件的相对目录。放在 uploads/scheduled/ 下
// 是为了让 cleanupOldPrints 天然扫不到（它只查 print_jobs.stored_path），
// 避免用户设的保留期把仍在使用的源文件误删。
const scheduledSubDir = "scheduled"

type scheduledFileResponse struct {
	Filename     string `json:"filename"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

type scheduledPrintResponse struct {
	PhotosPerRun   int                     `json:"photosPerRun"`
	DriverOptions  ipp.DriverOptions       `json:"driverOptions"`
	Files          []scheduledFileResponse `json:"files"`
	NextFileIndex  int                     `json:"nextFileIndex"`
	SubmittedCount int                     `json:"submittedCount"`
	Loop           bool                    `json:"loop"`
	StartDate      string                  `json:"startDate"`
	Completed      bool                    `json:"completed"`
	Running        bool                    `json:"running"`
	History        []store.ScheduledRun    `json:"history"`

	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	PrinterURI string `json:"printerUri"`
	Filename   string `json:"filename"`

	Sides              string `json:"sides"`
	IsDuplex           bool   `json:"isDuplex"`
	IsColor            bool   `json:"isColor"`
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

	ScheduleType    string `json:"scheduleType"`
	ScheduleTime    string `json:"scheduleTime"`
	ScheduleWeekday int    `json:"scheduleWeekday"`
	ScheduleDay     int    `json:"scheduleDay"`
	RunAt           string `json:"runAt"`
	NextRunAt       string `json:"nextRunAt"`
	LastRunAt       string `json:"lastRunAt"`
	LastStatus      string `json:"lastStatus"`
	LastError       string `json:"lastError"`
	Enabled         bool   `json:"enabled"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func mapScheduledPrint(rec store.ScheduledPrint) scheduledPrintResponse {
	files := make([]scheduledFileResponse, 0, len(rec.Files))
	for i, file := range rec.Files {
		files = append(files, scheduledFileResponse{Filename: file.Filename,
			ThumbnailURL: fmt.Sprintf("/api/scheduled-prints/%d/files/%d", rec.ID, i)})
	}
	history := rec.History
	if history == nil {
		history = []store.ScheduledRun{}
	}
	return scheduledPrintResponse{
		PhotosPerRun: rec.PhotosPerRun,
		Files:        files, NextFileIndex: rec.NextFileIndex, SubmittedCount: rec.SubmittedCount,
		Loop: rec.Loop, StartDate: rec.StartDate, Completed: scheduledQueueComplete(rec),
		Running: scheduledTaskRunning(rec.ID), History: history,
		ID:         rec.ID,
		UserID:     rec.UserID,
		Username:   rec.Username,
		Name:       rec.Name,
		PrinterURI: rec.PrinterURI,
		Filename:   rec.Filename,

		Sides:              rec.Sides,
		IsDuplex:           rec.IsDuplex,
		IsColor:            rec.IsColor,
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

		ScheduleType:    rec.ScheduleType,
		ScheduleTime:    rec.ScheduleTime,
		ScheduleWeekday: rec.ScheduleWeekday,
		ScheduleDay:     rec.ScheduleDay,
		RunAt:           rec.RunAt,
		NextRunAt:       rec.NextRunAt,
		LastRunAt:       rec.LastRunAt,
		LastStatus:      rec.LastStatus,
		LastError:       rec.LastError,
		Enabled:         rec.Enabled,

		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}

func scheduledListHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var records []store.ScheduledPrint
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		list, err := store.ListScheduledPrints(r.Context(), tx, store.ScheduledPrintFilter{
			Username: sess.Username,
		})
		if err != nil {
			return err
		}
		records = list
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load scheduled prints")
		return
	}
	resp := make([]scheduledPrintResponse, 0, len(records))
	for _, rec := range records {
		resp = append(resp, mapScheduledPrint(rec))
	}
	writeJSON(w, resp)
}

// scheduledCreateHandler 上传文件 + 打印参数 + 触发策略。走 multipart form；
// 文件独立存放到 uploads/scheduled/<userId>/ 避开保留期清理。
func scheduledCreateHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	applyUploadLimit(w, r)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isMaxBytesError(err) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "文件超出管理员设置的大小上限")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	defer r.MultipartForm.RemoveAll()
	headers := r.MultipartForm.File["files"]
	queue := len(headers) > 0
	if queue && len(r.MultipartForm.File["file"]) > 0 {
		writeJSONError(w, http.StatusBadRequest, "file 与 files 不能混用")
		return
	}
	if !queue {
		headers = r.MultipartForm.File["file"]
	}
	if len(headers) == 0 || len(headers) > 100 || (!queue && len(headers) != 1) {
		writeJSONError(w, http.StatusBadRequest, "请选择文件；照片队列最多 100 张")
		return
	}
	pending := make([]store.ScheduledFile, len(headers))
	for i, fh := range headers {
		pending[i] = store.ScheduledFile{Filename: fh.Filename, StoredPath: "pending"}
	}
	if _, err := buildScheduledRecordFromForm(r, sess.UserID, pending); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var saved []string
	keep := false
	defer func() {
		if !keep {
			removeScheduledFiles(uploadDir, saved)
		}
	}()
	baseDir := scheduledBaseDir(uploadDir, sess.UserID)
	files := make([]store.ScheduledFile, 0, len(headers))
	for _, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "无法读取上传文件")
			return
		}
		storedRel, absPath, saveErr := saveUploadedFile(f, fh.Filename, baseDir)
		_ = f.Close()
		if saveErr != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save file")
			return
		}
		rel := filepath.ToSlash(filepath.Join(scheduledSubDir, strconv.FormatInt(sess.UserID, 10), storedRel))
		saved = append(saved, rel)
		item := store.ScheduledFile{Filename: fh.Filename, StoredPath: rel}
		if queue {
			_, err := createScheduledThumbnail(absPath)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			item.ThumbnailPath = rel + ".thumb.jpg"
			saved = append(saved, item.ThumbnailPath)
		}
		files = append(files, item)
	}
	rec, err := buildScheduledRecordFromForm(r, sess.UserID, files)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var newID int64
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		id, err := store.InsertScheduledPrint(r.Context(), tx, &rec)
		if err != nil {
			return err
		}
		newID = id
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save scheduled print")
		return
	}

	keep = true
	var created store.ScheduledPrint
	if err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		c, err := store.GetScheduledPrintByID(r.Context(), tx, newID)
		if err != nil {
			return err
		}
		created = c
		return nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to reload scheduled print")
		return
	}
	writeJSON(w, mapScheduledPrint(created))
}

// scheduledUpdateHandler 只改调度参数与打印参数，不允许换文件；换文件请重新创建任务。
func scheduledUpdateHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var payload scheduledUpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	payload.Sides, err = ipp.ResolveSides(payload.Sides, payload.IsDuplex)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload.IsDuplex = payload.Sides != ipp.SidesOneSided
	if payload.PhotosPerRun != nil && (*payload.PhotosPerRun < 1 || *payload.PhotosPerRun > 100) {
		writeJSONError(w, http.StatusBadRequest, "每次打印张数应为 1–100")
		return
	}
	current, err := accessibleScheduledTask(r, id)
	if err != nil {
		writeScheduledAccessError(w, err)
		return
	}
	if !acquireScheduledTask(id, false) {
		writeJSONError(w, http.StatusConflict, "任务正在执行，请稍后再试")
		return
	}
	defer releaseScheduledTask(id)
	// Refresh after acquiring the shared execution/edit lock before validating
	// choices. Preserve offline edits, but validate retained choices on a new URI.
	current, err = accessibleScheduledTask(r, id)
	if err != nil {
		writeScheduledAccessError(w, err)
		return
	}
	if payload.DriverOptions == nil && payload.PrinterURI != current.PrinterURI {
		retained := current.DriverOptions
		payload.DriverOptions = &retained
	}
	if payload.DriverOptions != nil {
		if payload.PrinterURI == current.PrinterURI && payload.DriverOptions.SameChoices(current.DriverOptions) {
			// Pausing an offline printer must not require another capability probe.
			canonical := current.DriverOptions
			payload.DriverOptions = &canonical
		} else {
			canonical, err := ipp.ResolveDriverOptions(r.Context(), payload.PrinterURI, *payload.DriverOptions)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			payload.DriverOptions = &canonical
		}
	}

	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		rec, err := store.GetScheduledPrintByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if sess.Role != store.RoleAdmin && rec.UserID != sess.UserID {
			return errForbidden
		}
		previous := rec
		applyScheduledPayload(&rec, payload)
		if err := ensureScheduleDefaults(&rec); err != nil {
			return err
		}
		if err := refreshScheduledNextRun(&rec, previous, time.Now()); err != nil {
			return err
		}
		rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return store.UpdateScheduledPrint(r.Context(), tx, &rec)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, errForbidden) {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var updated store.ScheduledPrint
	if err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		u, err := store.GetScheduledPrintByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		updated = u
		return nil
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to reload scheduled print")
		return
	}
	writeJSON(w, mapScheduledPrint(updated))
}

func scheduledDeleteHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var stored []string
	if _, err := accessibleScheduledTask(r, id); err != nil {
		writeScheduledAccessError(w, err)
		return
	}
	if !acquireScheduledTask(id, false) {
		writeJSONError(w, http.StatusConflict, "任务正在执行，请稍后再试")
		return
	}
	defer releaseScheduledTask(id)

	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		rec, err := store.GetScheduledPrintByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if sess.Role != store.RoleAdmin && rec.UserID != sess.UserID {
			return errForbidden
		}
		stored = append(stored, rec.StoredPath)
		for _, file := range rec.Files {
			stored = append(stored, file.StoredPath, file.ThumbnailPath)
		}
		return store.DeleteScheduledPrint(r.Context(), tx, id)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, errForbidden) {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete scheduled print")
		return
	}
	removeScheduledFiles(uploadDir, stored)
	writeJSON(w, map[string]bool{"ok": true})
}

// 手动执行异步返回 202，转换与打印独立于 HTTP 120s WriteTimeout。
func scheduledRunNowHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := accessibleScheduledTask(r, id); err != nil {
		writeScheduledAccessError(w, err)
		return
	}
	if !acquireScheduledTask(id, true) {
		writeJSONError(w, http.StatusConflict, "任务正在执行，请勿重复提交")
		return
	}
	task, err := accessibleScheduledTask(r, id)
	if err != nil {
		releaseScheduledTask(id)
		writeScheduledAccessError(w, err)
		return
	}
	if scheduledQueueComplete(task) {
		releaseScheduledTask(id)
		writeJSONError(w, http.StatusConflict, "照片已全部提交")
		return
	}
	go func() {
		defer releaseScheduledTask(id)
		if err := executeScheduledOccurrence(appStore, uploadDir, task, time.Now(), true, executeScheduled); err != nil {
			log.Printf("[scheduler] manual task #%d failed: %v", id, err)
		}
	}()
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"ok": true, "status": "running"})
}

func accessibleScheduledTask(r *http.Request, id int64) (store.ScheduledPrint, error) {
	sess, err := auth.GetSession(r)
	if err != nil {
		return store.ScheduledPrint{}, errUnauthenticatedSchedule
	}
	var task store.ScheduledPrint
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		var err error
		task, err = store.GetScheduledPrintByID(r.Context(), tx, id)
		return err
	})
	if err == nil && sess.Role != store.RoleAdmin && task.UserID != sess.UserID {
		err = errForbidden
	}
	return task, err
}

var errUnauthenticatedSchedule = errors.New("unauthorized")

func writeScheduledAccessError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "无法读取定时任务"
	switch {
	case errors.Is(err, errUnauthenticatedSchedule):
		status, message = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, sql.ErrNoRows):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, errForbidden):
		status, message = http.StatusForbidden, "forbidden"
	}
	writeJSONError(w, status, message)
}

// 缩略图沿用 Session/API Key 鉴权，并只允许读本人或管理员可见的任务。
func scheduledThumbnailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	task, err := accessibleScheduledTask(r, id)
	if err != nil {
		writeScheduledAccessError(w, err)
		return
	}
	index, err := strconv.Atoi(mux.Vars(r)["index"])
	if err != nil || index < 0 || index >= len(task.Files) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	f, err := os.OpenInRoot(uploadDir, filepath.FromSlash(task.Files[index].ThumbnailPath))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "无法读取缩略图")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "thumbnail.jpg", stat.ModTime(), f)
}

// scheduledUpdatePayload 是 PUT 允许覆盖的字段。文件与用户不变。
type scheduledUpdatePayload struct {
	PhotosPerRun  *int               `json:"photosPerRun"`
	DriverOptions *ipp.DriverOptions `json:"driverOptions"`
	Loop          *bool              `json:"loop"`
	StartDate     *string            `json:"startDate"`
	Name          string             `json:"name"`
	PrinterURI    string             `json:"printerUri"`
	Enabled       *bool              `json:"enabled"`

	Sides              string `json:"sides"`
	IsDuplex           bool   `json:"isDuplex"`
	IsColor            bool   `json:"isColor"`
	Copies             int    `json:"copies"`
	Orientation        string `json:"orientation"`
	PaperSize          string `json:"paperSize"`
	PaperType          string `json:"paperType"`
	MediaSource        string `json:"mediaSource"`
	PrintScaling       string `json:"printScaling"`
	PageRange          string `json:"pageRange"`
	PageSet            string `json:"pageSet"`
	Mirror             bool   `json:"mirror"`
	PhotoDateWatermark *bool  `json:"photoDateWatermark"`
	WatermarkText      string `json:"watermarkText"`
	NumberUp           int    `json:"numberUp"`
	NumberUpLayout     string `json:"numberUpLayout"`
	PageBorder         string `json:"pageBorder"`

	ScheduleType    string `json:"scheduleType"`
	ScheduleTime    string `json:"scheduleTime"`
	ScheduleWeekday int    `json:"scheduleWeekday"`
	ScheduleDay     int    `json:"scheduleDay"`
	RunAt           string `json:"runAt"`
}

func applyScheduledPayload(rec *store.ScheduledPrint, p scheduledUpdatePayload) {
	if p.StartDate != nil {
		rec.StartDate = *p.StartDate
	}
	if p.Loop != nil {
		if *p.Loop && scheduledQueueComplete(*rec) {
			rec.NextFileIndex = 0
		}
		rec.Loop = *p.Loop
	}
	rec.Name = strings.TrimSpace(p.Name)
	rec.PrinterURI = p.PrinterURI
	rec.Sides = p.Sides
	rec.IsDuplex = p.IsDuplex
	rec.IsColor = p.IsColor
	rec.Copies = p.Copies
	rec.Orientation = p.Orientation
	if p.DriverOptions != nil {
		rec.DriverOptions = *p.DriverOptions
	}
	if p.PhotosPerRun != nil {
		rec.PhotosPerRun = *p.PhotosPerRun
	}
	rec.PaperSize = rec.DriverOptions.PaperSize(p.PaperSize)
	rec.PaperType = p.PaperType
	rec.MediaSource = p.MediaSource
	rec.PrintScaling = p.PrintScaling
	rec.PageRange = p.PageRange
	rec.PageSet = p.PageSet
	rec.Mirror = p.Mirror
	if p.PhotoDateWatermark != nil {
		rec.PhotoDateWatermark = *p.PhotoDateWatermark
	}
	rec.WatermarkText = strings.TrimSpace(p.WatermarkText)
	rec.NumberUp = p.NumberUp
	rec.NumberUpLayout = p.NumberUpLayout
	rec.PageBorder = p.PageBorder
	rec.ScheduleType = p.ScheduleType
	rec.ScheduleTime = p.ScheduleTime
	rec.ScheduleWeekday = p.ScheduleWeekday
	rec.ScheduleDay = p.ScheduleDay
	rec.RunAt = p.RunAt
	if p.Enabled != nil {
		rec.Enabled = *p.Enabled
	}
}

func buildScheduledRecordFromForm(r *http.Request, userID int64, files []store.ScheduledFile) (store.ScheduledPrint, error) {
	f := r.MultipartForm.Value
	getStr := func(key string) string {
		if vs, ok := f[key]; ok && len(vs) > 0 {
			return vs[0]
		}
		return ""
	}
	getInt := func(key string, def int) int {
		if s := getStr(key); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				return n
			}
		}
		return def
	}
	getBool := func(key string) bool {
		return getStr(key) == "true"
	}

	sides, err := ipp.ResolveSides(getStr("sides"), getBool("duplex"))
	if err != nil {
		return store.ScheduledPrint{}, err
	}
	driverOpts, err := driverOptionsFromForm(r, getStr("printer"))
	if err != nil {
		return store.ScheduledPrint{}, err
	}
	photosPerRun := 1
	if raw := getStr("photos_per_run"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return store.ScheduledPrint{}, fmt.Errorf("每次打印张数应为 1–100")
		}
		photosPerRun = n
	}
	rec := store.ScheduledPrint{
		DriverOptions: driverOpts, PhotosPerRun: photosPerRun,
		UserID: userID,
		Loop:   getBool("loop"), StartDate: getStr("start_date"),
		Name:       strings.TrimSpace(getStr("name")),
		PrinterURI: getStr("printer"),
		Filename:   files[0].Filename,
		StoredPath: files[0].StoredPath,

		Sides:              sides,
		IsDuplex:           sides != ipp.SidesOneSided,
		IsColor:            getBool("color"),
		Copies:             getInt("copies", 1),
		Orientation:        getStr("orientation"),
		PaperSize:          driverOpts.PaperSize(getStr("paper_size")),
		PaperType:          getStr("paper_type"),
		MediaSource:        getStr("media_source"),
		PrintScaling:       getStr("print_scaling"),
		PageRange:          strings.TrimSpace(getStr("page_range")),
		PageSet:            getStr("page_set"),
		Mirror:             getBool("mirror"),
		PhotoDateWatermark: getBool("photo_date_watermark"),
		WatermarkText:      strings.TrimSpace(getStr("watermark_text")),
		NumberUp:           getInt("number_up", 1),
		NumberUpLayout:     getStr("number_up_layout"),
		PageBorder:         getStr("page_border"),

		ScheduleType:    getStr("schedule_type"),
		ScheduleTime:    getStr("schedule_time"),
		ScheduleWeekday: getInt("schedule_weekday", 0),
		ScheduleDay:     getInt("schedule_day", 1),
		RunAt:           getStr("run_at"),
		Enabled:         true,
	}
	if len(r.MultipartForm.File["files"]) > 0 {
		rec.Files = files
	}

	if err := ensureScheduleDefaults(&rec); err != nil {
		return store.ScheduledPrint{}, err
	}

	next, ok := computeNextRun(rec, time.Now())
	if !ok {
		return store.ScheduledPrint{}, fmt.Errorf("无法计算下一次执行时间")
	}
	rec.NextRunAt = next.UTC().Format(time.RFC3339)
	if rec.ScheduleType == store.ScheduleOnce {
		rec.RunAt = rec.NextRunAt
	} else {
		rec.RunAt = ""
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec.CreatedAt = now
	rec.UpdatedAt = now
	return rec, nil
}

func scheduledBaseDir(base string, userID int64) string {
	return filepath.Join(base, scheduledSubDir, strconv.FormatInt(userID, 10))
}

var errForbidden = errors.New("forbidden")
