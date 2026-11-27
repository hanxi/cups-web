package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestWebAuthnCredentialCRUD(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	err = s.WithTx(ctx, false, func(tx *sql.Tx) error {
		u, err := CreateUser(ctx, tx, CreateUserInput{Username: "a", PasswordHash: "x", Role: RoleAdmin})
		if err != nil {
			return err
		}
		c, err := CreateWebAuthnCredential(ctx, tx, WebAuthnCredential{UserID: u.ID, CredentialID: "cid", Name: "n", CredentialJSON: "{}"})
		if err != nil {
			return err
		}
		if err := UpdateWebAuthnCredentialUsage(ctx, tx, c.ID, "{}"); err != nil {
			return err
		}
		if err := UpdateWebAuthnCredentialUsage(ctx, tx, c.ID+99, "{}"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("update missing credential must return ErrNoRows: %v", err)
		}
		if err := DeleteWebAuthnCredential(ctx, tx, c.ID, u.ID+1); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("delete by other user must fail: %v", err)
		}
		return DeleteWebAuthnCredential(ctx, tx, c.ID, u.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
}
