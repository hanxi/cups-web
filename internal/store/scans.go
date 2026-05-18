package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ScanJob struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"userId"`
	ScannerDevice string     `json:"scannerDevice"`
	Filename      string     `json:"filename"`
	StoredPath    string     `json:"storedPath"`
	Status        string     `json:"status"`
	Resolution    int        `json:"resolution"`
	ColorMode     string     `json:"colorMode"`
	PaperSize     string     `json:"paperSize"`
	ScanArea      string     `json:"scanArea,omitempty"`
	JobID         string     `json:"jobId,omitempty"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

func (s *Store) CreateScanJob(ctx context.Context, job *ScanJob) error {
	now := nowUTC()
	result, err := s.DB.ExecContext(ctx, `
		INSERT INTO scan_jobs (user_id, scanner_device, filename, stored_path, status, resolution, color_mode, paper_size, scan_area, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.UserID, job.ScannerDevice, job.Filename, job.StoredPath, job.Status, job.Resolution, job.ColorMode, job.PaperSize, job.ScanArea, now)
	if err != nil {
		return fmt.Errorf("create scan job: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get scan job id: %w", err)
	}
	job.ID = id
	job.CreatedAt, _ = time.Parse(time.RFC3339, now)
	return nil
}

func (s *Store) GetScanJob(ctx context.Context, id int64) (*ScanJob, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, user_id, scanner_device, filename, stored_path, status, resolution, color_mode, paper_size, scan_area, job_id, error_message, created_at, completed_at
		FROM scan_jobs WHERE id = ?
	`, id)
	return scanScanJob(row)
}

func (s *Store) GetScanJobByUserID(ctx context.Context, userID, id int64) (*ScanJob, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, user_id, scanner_device, filename, stored_path, status, resolution, color_mode, paper_size, scan_area, job_id, error_message, created_at, completed_at
		FROM scan_jobs WHERE id = ? AND user_id = ?
	`, id, userID)
	return scanScanJob(row)
}

func (s *Store) UpdateScanJobStatus(ctx context.Context, id int64, status string, errorMessage string) error {
	var completedAt string
	if status == "completed" || status == "failed" {
		completedAt = nowUTC()
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE scan_jobs SET status = ?, error_message = ?, completed_at = ? WHERE id = ?
	`, status, errorMessage, completedAt, id)
	if err != nil {
		return fmt.Errorf("update scan job status: %w", err)
	}
	return nil
}

func (s *Store) UpdateScanJobFilePath(ctx context.Context, id int64, storedPath string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE scan_jobs SET stored_path = ? WHERE id = ?
	`, storedPath, id)
	if err != nil {
		return fmt.Errorf("update scan job file path: %w", err)
	}
	return nil
}

func (s *Store) ListScanJobsByUserID(ctx context.Context, userID int64) ([]ScanJob, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, user_id, scanner_device, filename, stored_path, status, resolution, color_mode, paper_size, scan_area, job_id, error_message, created_at, completed_at
		FROM scan_jobs WHERE user_id = ? ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list scan jobs: %w", err)
	}
	defer rows.Close()

	var jobs []ScanJob
	for rows.Next() {
		job, err := scanScanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

func scanScanJob(row *sql.Row) (*ScanJob, error) {
	var job ScanJob
	var completedAt sql.NullString
	var createdAt sql.NullString
	var jobID sql.NullString
	err := row.Scan(
		&job.ID, &job.UserID, &job.ScannerDevice, &job.Filename, &job.StoredPath,
		&job.Status, &job.Resolution, &job.ColorMode, &job.PaperSize, &job.ScanArea,
		&jobID, &job.ErrorMessage, &createdAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan scan job: %w", err)
	}
	if jobID.Valid {
		job.JobID = jobID.String
	}
	if createdAt.Valid {
		job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		job.CompletedAt = &t
	}
	return &job, nil
}

func scanScanJobRows(rows *sql.Rows) (*ScanJob, error) {
	var job ScanJob
	var completedAt sql.NullString
	var createdAt sql.NullString
	var jobID sql.NullString
	err := rows.Scan(
		&job.ID, &job.UserID, &job.ScannerDevice, &job.Filename, &job.StoredPath,
		&job.Status, &job.Resolution, &job.ColorMode, &job.PaperSize, &job.ScanArea,
		&jobID, &job.ErrorMessage, &createdAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan scan job: %w", err)
	}
	if jobID.Valid {
		job.JobID = jobID.String
	}
	if createdAt.Valid {
		job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		job.CompletedAt = &t
	}
	return &job, nil
}