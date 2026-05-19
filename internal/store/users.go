package store

import (
	"context"
	"database/sql"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Protected    bool
	ContactName  string
	Phone        string
	Email        string
	CreatedAt    string
	UpdatedAt    string
}

type CreateUserInput struct {
	Username     string
	PasswordHash string
	Role         string
	Protected    bool
	ContactName  string
	Phone        string
	Email        string
}

type UpdateUserInput struct {
	ID           int64
	Username     string
	PasswordHash *string
	Role         string
	ContactName  string
	Phone        string
	Email        string
}

func CountUsers(ctx context.Context, tx *sql.Tx) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM users").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func GetUserByUsername(ctx context.Context, tx *sql.Tx, username string) (User, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		id, username, password_hash, role, protected, contact_name, phone, email,
		created_at, updated_at
		FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func GetUserByID(ctx context.Context, tx *sql.Tx, id int64) (User, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		id, username, password_hash, role, protected, contact_name, phone, email,
		created_at, updated_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func ListUsers(ctx context.Context, tx *sql.Tx) ([]User, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		id, username, password_hash, role, protected, contact_name, phone, email,
		created_at, updated_at
		FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func CreateUser(ctx context.Context, tx *sql.Tx, input CreateUserInput) (User, error) {
	now := nowUTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (
		username, password_hash, role, protected, contact_name, phone, email,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.Username, input.PasswordHash, input.Role, input.Protected, input.ContactName, input.Phone, input.Email,
		now, now,
	)
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return GetUserByID(ctx, tx, id)
}

func UpdateUser(ctx context.Context, tx *sql.Tx, input UpdateUserInput) (User, error) {
	now := nowUTC()
	if input.PasswordHash != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET
			username = ?, password_hash = ?, role = ?, contact_name = ?, phone = ?, email = ?,
			updated_at = ?
			WHERE id = ?`,
			input.Username, *input.PasswordHash, input.Role, input.ContactName, input.Phone, input.Email,
			now, input.ID,
		); err != nil {
			return User{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET
			username = ?, role = ?, contact_name = ?, phone = ?, email = ?,
			updated_at = ?
			WHERE id = ?`,
			input.Username, input.Role, input.ContactName, input.Phone, input.Email,
			now, input.ID,
		); err != nil {
			return User{}, err
		}
	}
	return GetUserByID(ctx, tx, input.ID)
}

func DeleteUser(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanUser(s scanner) (User, error) {
	var user User
	var contactName, phone, email sql.NullString
	err := s.Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.Protected,
		&contactName, &phone, &email,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return user, err
	}
	if contactName.Valid {
		user.ContactName = contactName.String
	}
	if phone.Valid {
		user.Phone = phone.String
	}
	if email.Valid {
		user.Email = email.String
	}
	return user, nil
}
