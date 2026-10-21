package store

import (
	"context"
	"database/sql"
)

// PrintPreset 是用户保存的一组打印参数（issue #128）。SettingsJSON 原样保存前端
// 快照，字段校验在提交打印时由 /api/print 负责，预置本身不承担可信语义。
type PrintPreset struct {
	ID           int64
	UserID       int64
	Name         string
	SettingsJSON string
	CreatedAt    string
	UpdatedAt    string
}

// ErrPrintPresetNotFound 复用 sql.ErrNoRows 语义：id 不存在或不属于该用户。
var ErrPrintPresetNotFound = sql.ErrNoRows

func ListPrintPresetsByUser(ctx context.Context, tx *sql.Tx, userID int64) ([]PrintPreset, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		id, user_id, name, settings_json, created_at, updated_at
		FROM print_presets WHERE user_id = ? ORDER BY name COLLATE NOCASE, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	presets := []PrintPreset{}
	for rows.Next() {
		p, err := scanPrintPreset(rows)
		if err != nil {
			return nil, err
		}
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func CountPrintPresetsByUser(ctx context.Context, tx *sql.Tx, userID int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM print_presets WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

func GetPrintPresetByName(ctx context.Context, tx *sql.Tx, userID int64, name string) (PrintPreset, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		id, user_id, name, settings_json, created_at, updated_at
		FROM print_presets WHERE user_id = ? AND name = ?`, userID, name)
	return scanPrintPreset(row)
}

func CreatePrintPreset(ctx context.Context, tx *sql.Tx, userID int64, name, settingsJSON string) (PrintPreset, error) {
	now := nowUTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO print_presets (
		user_id, name, settings_json, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?)`, userID, name, settingsJSON, now, now)
	if err != nil {
		return PrintPreset{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return PrintPreset{}, err
	}
	return GetPrintPreset(ctx, tx, id, userID)
}

// UpdatePrintPreset 覆盖名称与参数；用 user_id 二次约束防止越权修改他人预置。
func UpdatePrintPreset(ctx context.Context, tx *sql.Tx, id, userID int64, name, settingsJSON string) (PrintPreset, error) {
	res, err := tx.ExecContext(ctx, `UPDATE print_presets SET name = ?, settings_json = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`, name, settingsJSON, nowUTC(), id, userID)
	if err != nil {
		return PrintPreset{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return PrintPreset{}, err
	}
	if n == 0 {
		return PrintPreset{}, ErrPrintPresetNotFound
	}
	return GetPrintPreset(ctx, tx, id, userID)
}

func DeletePrintPreset(ctx context.Context, tx *sql.Tx, id, userID int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM print_presets WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrPrintPresetNotFound
	}
	return nil
}

func GetPrintPreset(ctx context.Context, tx *sql.Tx, id, userID int64) (PrintPreset, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		id, user_id, name, settings_json, created_at, updated_at
		FROM print_presets WHERE id = ? AND user_id = ?`, id, userID)
	return scanPrintPreset(row)
}

func scanPrintPreset(s scanner) (PrintPreset, error) {
	var p PrintPreset
	err := s.Scan(&p.ID, &p.UserID, &p.Name, &p.SettingsJSON, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}
