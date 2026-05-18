package store

import (
	"context"
	"database/sql"
)

type CustomPaperSize struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	MarginTop    float64 `json:"marginTop"`
	MarginRight  float64 `json:"marginRight"`
	MarginBottom float64 `json:"marginBottom"`
	MarginLeft   float64 `json:"marginLeft"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type CreatePaperSizeInput struct {
	Name         string  `json:"name"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	MarginTop    float64 `json:"marginTop"`
	MarginRight  float64 `json:"marginRight"`
	MarginBottom float64 `json:"marginBottom"`
	MarginLeft   float64 `json:"marginLeft"`
}

type UpdatePaperSizeInput struct {
	Name         string  `json:"name"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	MarginTop    float64 `json:"marginTop"`
	MarginRight  float64 `json:"marginRight"`
	MarginBottom float64 `json:"marginBottom"`
	MarginLeft   float64 `json:"marginLeft"`
}

func ListCustomPaperSizes(ctx context.Context, tx *sql.Tx) ([]CustomPaperSize, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		id, name, width, height,
		margin_top, margin_right, margin_bottom, margin_left,
		created_at, updated_at
		FROM custom_paper_sizes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sizes []CustomPaperSize
	for rows.Next() {
		s, err := scanPaperSize(rows)
		if err != nil {
			return nil, err
		}
		sizes = append(sizes, s)
	}
	return sizes, rows.Err()
}

func GetCustomPaperSizeByID(ctx context.Context, tx *sql.Tx, id int64) (CustomPaperSize, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		id, name, width, height,
		margin_top, margin_right, margin_bottom, margin_left,
		created_at, updated_at
		FROM custom_paper_sizes WHERE id = ?`, id)
	return scanPaperSizeRow(row)
}

func CreateCustomPaperSize(ctx context.Context, tx *sql.Tx, input CreatePaperSizeInput) (CustomPaperSize, error) {
	now := nowUTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO custom_paper_sizes (
		name, width, height,
		margin_top, margin_right, margin_bottom, margin_left,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Name, input.Width, input.Height,
		input.MarginTop, input.MarginRight, input.MarginBottom, input.MarginLeft,
		now, now,
	)
	if err != nil {
		return CustomPaperSize{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CustomPaperSize{}, err
	}
	return GetCustomPaperSizeByID(ctx, tx, id)
}

func UpdateCustomPaperSize(ctx context.Context, tx *sql.Tx, id int64, input UpdatePaperSizeInput) (CustomPaperSize, error) {
	now := nowUTC()
	_, err := tx.ExecContext(ctx, `UPDATE custom_paper_sizes SET
		name = ?, width = ?, height = ?,
		margin_top = ?, margin_right = ?, margin_bottom = ?, margin_left = ?,
		updated_at = ?
		WHERE id = ?`,
		input.Name, input.Width, input.Height,
		input.MarginTop, input.MarginRight, input.MarginBottom, input.MarginLeft,
		now, id,
	)
	if err != nil {
		return CustomPaperSize{}, err
	}
	return GetCustomPaperSizeByID(ctx, tx, id)
}

func DeleteCustomPaperSize(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM custom_paper_sizes WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func scanPaperSize(s scanner) (CustomPaperSize, error) {
	var p CustomPaperSize
	err := s.Scan(
		&p.ID, &p.Name, &p.Width, &p.Height,
		&p.MarginTop, &p.MarginRight, &p.MarginBottom, &p.MarginLeft,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func scanPaperSizeRow(row *sql.Row) (CustomPaperSize, error) {
	var p CustomPaperSize
	err := row.Scan(
		&p.ID, &p.Name, &p.Width, &p.Height,
		&p.MarginTop, &p.MarginRight, &p.MarginBottom, &p.MarginLeft,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}
