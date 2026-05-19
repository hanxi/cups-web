package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"cups-web/internal/store"
)

func startMaintenance(s *store.Store, uploads string) {
	go func() {
		for {
			printDeleted, err := cleanupOldPrints(context.Background(), s, uploads, time.Now())
			if err != nil {
				log.Println("cleanup prints failed:", err)
			}
			scanDeleted, err := cleanupOldScans(context.Background(), s, time.Now())
			if err != nil {
				log.Println("cleanup scans failed:", err)
			}
			if printDeleted+scanDeleted > 0 {
				if _, err := s.DB.ExecContext(context.Background(), "VACUUM"); err != nil {
					log.Println("vacuum failed:", err)
				}
				if _, err := s.DB.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
					log.Println("wal checkpoint failed:", err)
				}
			}
			time.Sleep(1 * time.Hour)
		}
	}()
}

func cleanupOldPrints(ctx context.Context, s *store.Store, uploads string, now time.Time) (int, error) {
	var retentionDays int64
	err := s.WithTx(ctx, true, func(tx *sql.Tx) error {
		val, err := store.GetSettingInt(ctx, tx, store.SettingRetentionDays, 0)
		if err != nil {
			return err
		}
		retentionDays = val
		return nil
	})
	if err != nil {
		return 0, err
	}
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := now.AddDate(0, 0, -int(retentionDays)).UTC().Format(time.RFC3339)
	var paths []string
	err = s.WithTx(ctx, false, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT stored_path FROM print_jobs WHERE created_at < ?", cutoff)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			paths = append(paths, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		_, err = tx.ExecContext(ctx, "DELETE FROM print_jobs WHERE created_at < ?", cutoff)
		return err
	})
	if err != nil {
		return 0, err
	}

	for _, rel := range paths {
		abs := filepath.Join(uploads, filepath.FromSlash(rel))
		_ = os.Remove(abs)
		convertedRel := convertedRelPath(rel)
		if convertedRel != "" {
			convertedAbs := filepath.Join(uploads, filepath.FromSlash(convertedRel))
			_ = os.Remove(convertedAbs)
		}
	}

	return len(paths), nil
}

func cleanupOldScans(ctx context.Context, s *store.Store, now time.Time) (int, error) {
	var retentionDays int64
	err := s.WithTx(ctx, true, func(tx *sql.Tx) error {
		val, err := store.GetSettingInt(ctx, tx, store.SettingRetentionDays, 0)
		if err != nil {
			return err
		}
		retentionDays = val
		return nil
	})
	if err != nil {
		return 0, err
	}
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := now.AddDate(0, 0, -int(retentionDays)).UTC().Format(time.RFC3339)
	var paths []string
	err = s.WithTx(ctx, false, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT stored_path FROM scan_jobs WHERE created_at < ?", cutoff)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			paths = append(paths, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		_, err = tx.ExecContext(ctx, "DELETE FROM scan_jobs WHERE created_at < ?", cutoff)
		return err
	})
	if err != nil {
		return 0, err
	}

	for _, p := range paths {
		if p != "" {
			_ = os.Remove(p)
		}
	}

	return len(paths), nil
}
