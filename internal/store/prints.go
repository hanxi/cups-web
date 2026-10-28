package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"cups-web/internal/ipp"
)

type PrintRecord struct {
	DriverOptions ipp.DriverOptions
	PhotoLayout   PhotoLayout
	RenderedPath  string

	ID         int64
	UserID     int64
	Username   string
	PrinterURI string
	Filename   string
	StoredPath string
	Pages      int
	JobID      sql.NullString
	Status     string
	Sides      string
	IsDuplex   bool
	IsColor    bool

	// 完整打印参数快照，供「重新打印」精确预填（Issue #68）。
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

	CreatedAt string
}

// Photo sources are owned by their print record; reprints copy them so history
// cleanup can remove one record without damaging another record's originals.
type PhotoSource struct {
	Filename   string `json:"filename"`
	StoredPath string `json:"storedPath"`
	Rotation   int    `json:"rotation"`
}

type PhotoLayout struct {
	Sources  []PhotoSource `json:"sources,omitempty"`
	PerPage  int           `json:"perPage,omitempty"`
	Invert   bool          `json:"invert,omitempty"`
	Compress bool          `json:"compress,omitempty"`
}

// printRecordColumns 是 SELECT 时的统一列清单（带 p./u. 前缀），
// 与 scanPrintRecord 的字段顺序严格对应。
const printRecordColumns = `p.id, p.user_id, u.username, p.printer_uri, p.filename, p.stored_path, p.pages,
	p.job_id, p.status, p.is_duplex, p.sides, p.is_color,
	p.copies, p.orientation, p.paper_size, p.paper_type, p.media_source, p.print_scaling,
	p.page_range, p.page_set, p.mirror, p.watermark_text, p.photo_date_watermark, p.number_up, p.number_up_layout, p.page_border,
	p.created_at, p.driver_options_json, p.photo_layout_json, p.rendered_path`

// scanPrintRecord 与 printRecordColumns 的列顺序严格对应。
// 复用 users.go 中定义的 scanner 接口（*sql.Row / *sql.Rows 通用）。
func scanPrintRecord(s scanner) (PrintRecord, error) {
	var rec PrintRecord
	var driverJSON string
	var photoJSON string
	err := s.Scan(
		&rec.ID, &rec.UserID, &rec.Username, &rec.PrinterURI, &rec.Filename, &rec.StoredPath,
		&rec.Pages, &rec.JobID, &rec.Status, &rec.IsDuplex, &rec.Sides, &rec.IsColor,
		&rec.Copies, &rec.Orientation, &rec.PaperSize, &rec.PaperType, &rec.MediaSource, &rec.PrintScaling,
		&rec.PageRange, &rec.PageSet, &rec.Mirror, &rec.WatermarkText, &rec.PhotoDateWatermark, &rec.NumberUp, &rec.NumberUpLayout, &rec.PageBorder,
		&rec.CreatedAt, &driverJSON, &photoJSON, &rec.RenderedPath,
	)
	if err == nil {
		rec.Sides, err = ipp.ResolveSides(rec.Sides, rec.IsDuplex)
		rec.IsDuplex = rec.Sides != ipp.SidesOneSided
	}
	if err == nil {
		err = json.Unmarshal([]byte(driverJSON), &rec.DriverOptions)
	}
	if err == nil {
		err = json.Unmarshal([]byte(photoJSON), &rec.PhotoLayout)
	}
	return rec, err
}

type PrintFilter struct {
	Username string
	StartAt  string
	EndAt    string
	Limit    int
}

func InsertPrintRecord(ctx context.Context, tx *sql.Tx, rec *PrintRecord) (int64, error) {
	sides, err := ipp.ResolveSides(rec.Sides, rec.IsDuplex)
	if err != nil {
		return 0, err
	}
	rec.Sides, rec.IsDuplex = sides, sides != ipp.SidesOneSided
	driverJSON, err := json.Marshal(rec.DriverOptions)
	if err != nil {
		return 0, err
	}
	photoJSON, err := json.Marshal(rec.PhotoLayout)
	if err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO print_jobs (
		user_id, printer_uri, filename, stored_path, pages,
		job_id, status, is_duplex, sides, is_color,
		copies, orientation, paper_size, paper_type, media_source, print_scaling,
		page_range, page_set, mirror, watermark_text, photo_date_watermark, number_up, number_up_layout, page_border,
		created_at, driver_options_json, photo_layout_json, rendered_path
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.PrinterURI, rec.Filename, rec.StoredPath, rec.Pages,
		rec.JobID, rec.Status, rec.IsDuplex, rec.Sides, rec.IsColor,
		rec.Copies, rec.Orientation, rec.PaperSize, rec.PaperType, rec.MediaSource, rec.PrintScaling,
		rec.PageRange, rec.PageSet, rec.Mirror, rec.WatermarkText, rec.PhotoDateWatermark, rec.NumberUp, rec.NumberUpLayout, rec.PageBorder,
		rec.CreatedAt, string(driverJSON), string(photoJSON), rec.RenderedPath,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdatePrintStatus(ctx context.Context, tx *sql.Tx, id int64, status string, jobID string) error {
	_, err := tx.ExecContext(ctx, "UPDATE print_jobs SET status = ?, job_id = ? WHERE id = ?", status, jobID, id)
	return err
}

func GetPrintRecordByID(ctx context.Context, tx *sql.Tx, id int64) (PrintRecord, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+printRecordColumns+`
		FROM print_jobs p
		JOIN users u ON u.id = p.user_id
		WHERE p.id = ?`, id)
	return scanPrintRecord(row)
}

func ListPrintRecords(ctx context.Context, tx *sql.Tx, filter PrintFilter) ([]PrintRecord, error) {
	args := []interface{}{}
	conds := []string{"1=1"}
	if filter.Username != "" {
		conds = append(conds, "u.username = ?")
		args = append(args, filter.Username)
	}
	if filter.StartAt != "" {
		conds = append(conds, "p.created_at >= ?")
		args = append(args, filter.StartAt)
	}
	if filter.EndAt != "" {
		conds = append(conds, "p.created_at <= ?")
		args = append(args, filter.EndAt)
	}
	query := fmt.Sprintf(`SELECT `+printRecordColumns+`
		FROM print_jobs p
		JOIN users u ON u.id = p.user_id
		WHERE %s
		ORDER BY p.created_at DESC`, strings.Join(conds, " AND "))
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []PrintRecord
	for rows.Next() {
		rec, err := scanPrintRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
