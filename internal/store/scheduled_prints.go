package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"cups-web/internal/ipp"
)

// ScheduleType 枚举
const (
	ScheduleOnce    = "once"
	ScheduleDaily   = "daily"
	ScheduleWeekly  = "weekly"
	ScheduleMonthly = "monthly"
)

// ScheduledFile 保存固定顺序的照片和缩略图，路径均相对 uploads。
type ScheduledFile struct {
	Filename      string `json:"filename"`
	StoredPath    string `json:"storedPath"`
	ThumbnailPath string `json:"thumbnailPath"`
}

// ScheduledRun 只保留最近 20 次执行；printed 表示 CUPS 接受，非物理出纸。
type ScheduledRun struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Error    string `json:"error"`
	RunAt    string `json:"runAt"`
}

// ScheduledPrint 定时打印任务：文件 + 完整打印参数 + 触发策略。
// 与 print_jobs 的字段一一对应，避免执行时再猜默认值。
type ScheduledPrint struct {
	DriverOptions ipp.DriverOptions
	PhotosPerRun  int
	ID            int64
	UserID        int64
	Username      string
	Name          string
	PrinterURI    string
	Filename      string
	StoredPath    string

	Sides              string
	IsDuplex           bool
	IsColor            bool
	Copies             int
	Orientation        string
	PaperSize          string
	PaperType          string
	MediaSource        string
	PrintScaling       string
	PageRange          string
	PageSet            string
	Mirror             bool
	PhotoDateWatermark bool
	WatermarkText      string
	NumberUp           int
	NumberUpLayout     string
	PageBorder         string

	ScheduleType    string
	ScheduleTime    string
	ScheduleWeekday int
	ScheduleDay     int
	RunAt           string
	NextRunAt       string
	LastRunAt       string
	LastStatus      string
	LastError       string
	Enabled         bool

	Files          []ScheduledFile
	NextFileIndex  int
	SubmittedCount int
	Loop           bool
	StartDate      string
	History        []ScheduledRun

	CreatedAt string
	UpdatedAt string
}

const scheduledPrintColumns = `s.id, s.user_id, u.username, s.name, s.printer_uri, s.filename, s.stored_path,
	s.is_duplex, s.sides, s.is_color, s.copies, s.orientation, s.paper_size, s.paper_type, s.media_source, s.print_scaling,
	s.page_range, s.page_set, s.mirror, s.watermark_text, s.photo_date_watermark, s.number_up, s.number_up_layout, s.page_border,
	s.schedule_type, s.schedule_time, s.schedule_weekday, s.schedule_day,
	s.run_at, s.next_run_at, s.last_run_at, s.last_status, s.last_error, s.enabled,
	s.created_at, s.updated_at, s.files_json, s.next_file_index, s.submitted_count, s.loop, s.start_date, s.history_json, s.driver_options_json, s.photos_per_run`

func scanScheduledPrint(sc scanner) (ScheduledPrint, error) {
	var rec ScheduledPrint
	var filesJSON, historyJSON, driverJSON string
	err := sc.Scan(
		&rec.ID, &rec.UserID, &rec.Username, &rec.Name, &rec.PrinterURI, &rec.Filename, &rec.StoredPath,
		&rec.IsDuplex, &rec.Sides, &rec.IsColor, &rec.Copies, &rec.Orientation, &rec.PaperSize, &rec.PaperType, &rec.MediaSource, &rec.PrintScaling,
		&rec.PageRange, &rec.PageSet, &rec.Mirror, &rec.WatermarkText, &rec.PhotoDateWatermark, &rec.NumberUp, &rec.NumberUpLayout, &rec.PageBorder,
		&rec.ScheduleType, &rec.ScheduleTime, &rec.ScheduleWeekday, &rec.ScheduleDay,
		&rec.RunAt, &rec.NextRunAt, &rec.LastRunAt, &rec.LastStatus, &rec.LastError, &rec.Enabled,
		&rec.CreatedAt, &rec.UpdatedAt, &filesJSON, &rec.NextFileIndex, &rec.SubmittedCount, &rec.Loop, &rec.StartDate, &historyJSON, &driverJSON, &rec.PhotosPerRun,
	)
	if err != nil {
		return rec, err
	}
	rec.Sides, err = ipp.ResolveSides(rec.Sides, rec.IsDuplex)
	if err != nil {
		return rec, err
	}
	rec.IsDuplex = rec.Sides != ipp.SidesOneSided
	if err := json.Unmarshal([]byte(driverJSON), &rec.DriverOptions); err != nil {
		return rec, err
	}
	if err := json.Unmarshal([]byte(filesJSON), &rec.Files); err != nil {
		return rec, err
	}
	if err := json.Unmarshal([]byte(historyJSON), &rec.History); err != nil {
		return rec, err
	}
	return rec, nil
}

// InsertScheduledPrint 插入一条定时任务，返回新 ID。
func InsertScheduledPrint(ctx context.Context, tx *sql.Tx, rec *ScheduledPrint) (int64, error) {
	sides, err := ipp.ResolveSides(rec.Sides, rec.IsDuplex)
	if err != nil {
		return 0, err
	}
	rec.Sides, rec.IsDuplex = sides, sides != ipp.SidesOneSided
	driverJSON, err := json.Marshal(rec.DriverOptions)
	if err != nil {
		return 0, err
	}
	photosPerRun := rec.PhotosPerRun
	if photosPerRun == 0 {
		photosPerRun = 1
	}
	filesJSON, err := json.Marshal(rec.Files)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO scheduled_prints (
		user_id, name, printer_uri, filename, stored_path,
		is_duplex, sides, is_color, copies, orientation, paper_size, paper_type, media_source, print_scaling,
		page_range, page_set, mirror, watermark_text, photo_date_watermark, number_up, number_up_layout, page_border,
		schedule_type, schedule_time, schedule_weekday, schedule_day,
		run_at, next_run_at, last_run_at, last_status, last_error, enabled,
		created_at, updated_at, files_json, loop, start_date, driver_options_json, photos_per_run
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.Name, rec.PrinterURI, rec.Filename, rec.StoredPath,
		rec.IsDuplex, rec.Sides, rec.IsColor, rec.Copies, rec.Orientation, rec.PaperSize, rec.PaperType, rec.MediaSource, rec.PrintScaling,
		rec.PageRange, rec.PageSet, rec.Mirror, rec.WatermarkText, rec.PhotoDateWatermark, rec.NumberUp, rec.NumberUpLayout, rec.PageBorder,
		rec.ScheduleType, rec.ScheduleTime, rec.ScheduleWeekday, rec.ScheduleDay,
		rec.RunAt, rec.NextRunAt, rec.LastRunAt, rec.LastStatus, rec.LastError, rec.Enabled,
		rec.CreatedAt, rec.UpdatedAt, string(filesJSON), rec.Loop, rec.StartDate, string(driverJSON), photosPerRun,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateScheduledPrint 覆盖打印参数与调度策略。next_run_at 也一并更新，
// 避免旧调度点仍在数据库里被扫到。
func UpdateScheduledPrint(ctx context.Context, tx *sql.Tx, rec *ScheduledPrint) error {
	sides, err := ipp.ResolveSides(rec.Sides, rec.IsDuplex)
	if err != nil {
		return err
	}
	rec.Sides, rec.IsDuplex = sides, sides != ipp.SidesOneSided
	driverJSON, err := json.Marshal(rec.DriverOptions)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE scheduled_prints SET
		name = ?, printer_uri = ?,
		is_duplex = ?, sides = ?, is_color = ?, copies = ?, orientation = ?, paper_size = ?, paper_type = ?, media_source = ?, print_scaling = ?,
		page_range = ?, page_set = ?, mirror = ?, watermark_text = ?, photo_date_watermark = ?, number_up = ?, number_up_layout = ?, page_border = ?,
		schedule_type = ?, schedule_time = ?, schedule_weekday = ?, schedule_day = ?,
		run_at = ?, next_run_at = ?, enabled = ?, updated_at = ?, loop = ?, start_date = ?, next_file_index = ?, driver_options_json = ?, photos_per_run = ?
		WHERE id = ?`,
		rec.Name, rec.PrinterURI,
		rec.IsDuplex, rec.Sides, rec.IsColor, rec.Copies, rec.Orientation, rec.PaperSize, rec.PaperType, rec.MediaSource, rec.PrintScaling,
		rec.PageRange, rec.PageSet, rec.Mirror, rec.WatermarkText, rec.PhotoDateWatermark, rec.NumberUp, rec.NumberUpLayout, rec.PageBorder,
		rec.ScheduleType, rec.ScheduleTime, rec.ScheduleWeekday, rec.ScheduleDay,
		rec.RunAt, rec.NextRunAt, rec.Enabled, rec.UpdatedAt, rec.Loop, rec.StartDate, rec.NextFileIndex, string(driverJSON), rec.PhotosPerRun,
		rec.ID,
	)
	return err
}

// UpdateScheduledPrintAfterRun 记录一次执行结果并推进 next_run_at。once 类型
// 执行完把 enabled 置 0，作为归档；周期类型仅推进 next_run_at。
func UpdateScheduledPrintAfterRun(ctx context.Context, tx *sql.Tx, id int64, lastRunAt, lastStatus, lastError, nextRunAt string, enabled bool) error {
	_, err := tx.ExecContext(ctx, `UPDATE scheduled_prints SET
		last_run_at = ?, last_status = ?, last_error = ?, next_run_at = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		lastRunAt, lastStatus, lastError, nextRunAt, enabled, nowUTC(),
		id,
	)
	return err
}

// SetScheduledPrintEnabled 只切换启用/禁用状态。禁用时保留 next_run_at，
// 恢复启用时可以直接被调度器扫到。
func SetScheduledPrintEnabled(ctx context.Context, tx *sql.Tx, id int64, enabled bool) error {
	_, err := tx.ExecContext(ctx, `UPDATE scheduled_prints SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, nowUTC(), id,
	)
	return err
}

// DeleteScheduledPrint 删除任务；调用方需要另行处理关联文件的清理。
func DeleteScheduledPrint(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM scheduled_prints WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

// GetScheduledPrintByID 单条获取。
func GetScheduledPrintByID(ctx context.Context, tx *sql.Tx, id int64) (ScheduledPrint, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+scheduledPrintColumns+`
		FROM scheduled_prints s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, id)
	return scanScheduledPrint(row)
}

// ScheduledPrintFilter 用户名为空时列全表；主要用于管理员视图与用户视图区分。
type ScheduledPrintFilter struct {
	Username string
}

// ListScheduledPrints 按 next_run_at 升序返回，禁用的任务也一并返回但排到后面。
func ListScheduledPrints(ctx context.Context, tx *sql.Tx, filter ScheduledPrintFilter) ([]ScheduledPrint, error) {
	args := []interface{}{}
	conds := []string{"1=1"}
	if filter.Username != "" {
		conds = append(conds, "u.username = ?")
		args = append(args, filter.Username)
	}
	query := fmt.Sprintf(`SELECT `+scheduledPrintColumns+`
		FROM scheduled_prints s
		JOIN users u ON u.id = s.user_id
		WHERE %s
		ORDER BY s.enabled DESC, s.next_run_at ASC`, strings.Join(conds, " AND "))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ScheduledPrint
	for rows.Next() {
		rec, err := scanScheduledPrint(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// ListDueScheduledPrints 返回所有到期（enabled=1 AND next_run_at <= now）的任务，
// 由调度器逐个执行。
func ListDueScheduledPrints(ctx context.Context, tx *sql.Tx, nowUTC string) ([]ScheduledPrint, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+scheduledPrintColumns+`
		FROM scheduled_prints s
		JOIN users u ON u.id = s.user_id
		WHERE s.enabled = 1 AND s.next_run_at <= ?
		ORDER BY s.next_run_at ASC`, nowUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ScheduledPrint
	for rows.Next() {
		rec, err := scanScheduledPrint(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// SaveScheduledExecution 原子保存结果、队列进度和最近执行历史。
func SaveScheduledExecution(ctx context.Context, tx *sql.Tx, rec *ScheduledPrint) error {
	historyJSON, err := json.Marshal(rec.History)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE scheduled_prints SET
 last_run_at = ?, last_status = ?, last_error = ?, next_run_at = ?, enabled = ?,
 next_file_index = ?, submitted_count = ?, history_json = ?, updated_at = ? WHERE id = ?`,
		rec.LastRunAt, rec.LastStatus, rec.LastError, rec.NextRunAt, rec.Enabled,
		rec.NextFileIndex, rec.SubmittedCount, string(historyJSON), nowUTC(), rec.ID)
	return err
}
