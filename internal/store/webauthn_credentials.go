package store

import (
	"context"
	"database/sql"
)

// WebAuthnCredential 是一枚 passkey 的持久化记录。CredentialJSON 保存
// go-webauthn 的 Credential 完整序列化（公钥、签名计数、标志位、transports 等），
// 这样升级库增加字段时不需要改表结构；CredentialID 单独成列用于唯一约束与查找。
type WebAuthnCredential struct {
	ID             int64
	UserID         int64
	CredentialID   string // base64url
	Name           string
	CredentialJSON string
	CreatedAt      string
	LastUsedAt     string
}

const webauthnCols = `id, user_id, credential_id, name, credential_json, created_at, last_used_at`

func scanWebAuthn(s scanner) (WebAuthnCredential, error) {
	var c WebAuthnCredential
	err := s.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.Name, &c.CredentialJSON, &c.CreatedAt, &c.LastUsedAt)
	return c, err
}

func CreateWebAuthnCredential(ctx context.Context, tx *sql.Tx, c WebAuthnCredential) (WebAuthnCredential, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO webauthn_credentials
		(user_id, credential_id, name, credential_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		c.UserID, c.CredentialID, c.Name, c.CredentialJSON, nowUTC())
	if err != nil {
		return WebAuthnCredential{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return WebAuthnCredential{}, err
	}
	return scanWebAuthn(tx.QueryRowContext(ctx, `SELECT `+webauthnCols+` FROM webauthn_credentials WHERE id = ?`, id))
}

func ListWebAuthnCredentials(ctx context.Context, tx *sql.Tx, userID int64) ([]WebAuthnCredential, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+webauthnCols+` FROM webauthn_credentials WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebAuthnCredential{}
	for rows.Next() {
		c, err := scanWebAuthn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func GetWebAuthnCredentialByCredentialID(ctx context.Context, tx *sql.Tx, credentialID string) (WebAuthnCredential, error) {
	return scanWebAuthn(tx.QueryRowContext(ctx, `SELECT `+webauthnCols+` FROM webauthn_credentials WHERE credential_id = ?`, credentialID))
}

// CountAdminWebAuthnCredentials 统计管理员账号名下的 passkey 数量。DISABLE_PASSWORD_LOGIN
// 只有在至少一个管理员绑定了 passkey 时才生效，避免管理员被锁在系统外。
func CountAdminWebAuthnCredentials(ctx context.Context, tx *sql.Tx) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM webauthn_credentials c
		JOIN users u ON u.id = c.user_id WHERE u.role = ?`, RoleAdmin).Scan(&n)
	return n, err
}

func CountWebAuthnCredentialsByUser(ctx context.Context, tx *sql.Tx, userID int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// UpdateWebAuthnCredentialUsage 登录成功后回写最新的签名计数等状态与最近使用时间。
func UpdateWebAuthnCredentialUsage(ctx context.Context, tx *sql.Tx, id int64, credentialJSON string) error {
	_, err := tx.ExecContext(ctx, `UPDATE webauthn_credentials SET credential_json = ?, last_used_at = ? WHERE id = ?`,
		credentialJSON, nowUTC(), id)
	return err
}

// DeleteWebAuthnCredential 只删除属于 userID 的记录，无匹配返回 sql.ErrNoRows。
func DeleteWebAuthnCredential(ctx context.Context, tx *sql.Tx, id, userID int64) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
