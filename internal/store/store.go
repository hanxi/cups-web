package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	SettingRetentionDays  = "retention_days"
	SettingSaveHistory    = "save_history"
	SettingMaxPagesPerJob = "max_pages_per_job"
	SettingMaxUploadBytes = "max_upload_bytes"
	SettingCustomCSS      = "custom_css"
	SettingGuestMode      = "guest_mode"
)

type Store struct {
	DB *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) WithTx(ctx context.Context, readOnly bool, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			protected INTEGER NOT NULL DEFAULT 0,
			contact_name TEXT,
			phone TEXT,
			email TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scheduled_prints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			printer_uri TEXT NOT NULL,
			filename TEXT NOT NULL,
			stored_path TEXT NOT NULL,
			is_duplex INTEGER NOT NULL DEFAULT 0,
			is_color INTEGER NOT NULL DEFAULT 1,
			copies INTEGER NOT NULL DEFAULT 1,
			orientation TEXT NOT NULL DEFAULT 'portrait',
			paper_size TEXT NOT NULL DEFAULT 'A4',
			paper_type TEXT NOT NULL DEFAULT 'plain',
			media_source TEXT NOT NULL DEFAULT 'auto',
			print_scaling TEXT NOT NULL DEFAULT 'fit',
			page_range TEXT NOT NULL DEFAULT '',
			page_set TEXT NOT NULL DEFAULT 'all',
			mirror INTEGER NOT NULL DEFAULT 0,
			watermark_text TEXT NOT NULL DEFAULT '',
			number_up INTEGER NOT NULL DEFAULT 1,
			number_up_layout TEXT NOT NULL DEFAULT 'lrtb',
			page_border TEXT NOT NULL DEFAULT 'none',
			schedule_type TEXT NOT NULL DEFAULT 'once',
			schedule_time TEXT NOT NULL DEFAULT '09:00',
			schedule_weekday INTEGER NOT NULL DEFAULT 0,
			schedule_day INTEGER NOT NULL DEFAULT 1,
			run_at TEXT NOT NULL DEFAULT '',
			next_run_at TEXT NOT NULL,
			last_run_at TEXT NOT NULL DEFAULT '',
			last_status TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_prints_next
			ON scheduled_prints(enabled, next_run_at)`,
		`CREATE TABLE IF NOT EXISTS scan_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			device TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'color',
			resolution INTEGER NOT NULL DEFAULT 300,
			source TEXT NOT NULL DEFAULT '',
			format TEXT NOT NULL DEFAULT 'png',
			filename TEXT NOT NULL,
			stored_path TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			err_msg TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			finished_at TEXT,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_scan_records_user_time
			ON scan_records(user_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			prefix TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			last_used_at TEXT NOT NULL DEFAULT '',
			last_used_ip TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`,
		// Issue #128：用户命名的打印预置，同一用户内名称唯一。
		`CREATE TABLE IF NOT EXISTS print_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			settings_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, name),
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS print_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			printer_uri TEXT NOT NULL,
			filename TEXT NOT NULL,
			stored_path TEXT NOT NULL,
			pages INTEGER NOT NULL,
			job_id TEXT,
			status TEXT NOT NULL,
			is_duplex INTEGER NOT NULL DEFAULT 0,
			is_color INTEGER NOT NULL DEFAULT 1,
			copies INTEGER NOT NULL DEFAULT 1,
			orientation TEXT NOT NULL DEFAULT 'portrait',
			paper_size TEXT NOT NULL DEFAULT 'A4',
			paper_type TEXT NOT NULL DEFAULT 'plain',
			media_source TEXT NOT NULL DEFAULT 'auto',
			print_scaling TEXT NOT NULL DEFAULT 'fit',
			page_range TEXT NOT NULL DEFAULT '',
			page_set TEXT NOT NULL DEFAULT 'all',
			mirror INTEGER NOT NULL DEFAULT 0,
			watermark_text TEXT NOT NULL DEFAULT '',
			number_up INTEGER NOT NULL DEFAULT 1,
			number_up_layout TEXT NOT NULL DEFAULT 'lrtb',
			page_border TEXT NOT NULL DEFAULT 'none',
			created_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	for _, col := range []string{"photo_layout_json TEXT NOT NULL DEFAULT '{}'", "rendered_path TEXT NOT NULL DEFAULT ''"} {
		if err := addColumnIfMissing(ctx, s.DB, "print_jobs", col); err != nil {
			return err
		}
	}
	// Issue #121：队列为空时保留旧任务反复打印单文件的语义。
	for _, col := range []string{
		"files_json TEXT NOT NULL DEFAULT '[]'",
		"next_file_index INTEGER NOT NULL DEFAULT 0",
		"submitted_count INTEGER NOT NULL DEFAULT 0",
		"loop INTEGER NOT NULL DEFAULT 0",
		"start_date TEXT NOT NULL DEFAULT ''",
		"history_json TEXT NOT NULL DEFAULT '[]'",
	} {
		if err := addColumnIfMissing(ctx, s.DB, "scheduled_prints", col); err != nil {
			return fmt.Errorf("migrate scheduled prints: %w", err)
		}
	}
	// Issue #122: snapshot driver choices and preserve one-photo behavior for old queues.
	for _, table := range []string{"print_jobs", "scheduled_prints"} {
		if err := addColumnIfMissing(ctx, s.DB, table, "driver_options_json TEXT NOT NULL DEFAULT '{}'"); err != nil {
			return err
		}
	}
	if err := addColumnIfMissing(ctx, s.DB, "scheduled_prints", "photos_per_run INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, s.DB, "users", "protected INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := addColumnIfMissing(ctx, s.DB, "print_jobs", "is_duplex INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := addColumnIfMissing(ctx, s.DB, "print_jobs", "is_color INTEGER NOT NULL DEFAULT 1"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, table := range []string{"print_jobs", "scheduled_prints"} {
		if err := addColumnIfMissing(ctx, s.DB, table, "photo_date_watermark INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	// 完整打印参数落库，供「重新打印」精确预填第一次的每一项设置（Issue #68）。
	// 旧库热升级：老记录这些列取默认值，重打时即退化为合理默认。
	printJobOptionCols := []string{
		"copies INTEGER NOT NULL DEFAULT 1",
		"orientation TEXT NOT NULL DEFAULT 'portrait'",
		"paper_size TEXT NOT NULL DEFAULT 'A4'",
		"paper_type TEXT NOT NULL DEFAULT 'plain'",
		"media_source TEXT NOT NULL DEFAULT 'auto'",
		"print_scaling TEXT NOT NULL DEFAULT 'fit'",
		"page_range TEXT NOT NULL DEFAULT ''",
		"page_set TEXT NOT NULL DEFAULT 'all'",
		"mirror INTEGER NOT NULL DEFAULT 0",
		"watermark_text TEXT NOT NULL DEFAULT ''",
		"number_up INTEGER NOT NULL DEFAULT 1",
		"number_up_layout TEXT NOT NULL DEFAULT 'lrtb'",
		"page_border TEXT NOT NULL DEFAULT 'none'",
	}
	for _, col := range printJobOptionCols {
		if err := addColumnIfMissing(ctx, s.DB, "print_jobs", col); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`,
		SettingRetentionDays, "0",
	); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`,
		SettingSaveHistory, "1",
	); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`,
		SettingMaxPagesPerJob, "0",
	); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`,
		SettingMaxUploadBytes, "0",
	); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`,
		SettingGuestMode, "0",
	); err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}

	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, table string, columnDef string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, columnDef))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}
