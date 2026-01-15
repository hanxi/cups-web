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
			if err := applyAutoTopups(context.Background(), s, time.Now()); err != nil {
				log.Println("auto topup failed:", err)
			}
			if err := cleanupOldPrints(context.Background(), s, uploads, time.Now()); err != nil {
				log.Println("cleanup failed:", err)
			}
			time.Sleep(1 * time.Hour)
		}
	}()
}

func applyAutoTopups(ctx context.Context, s *store.Store, now time.Time) error {
	today := now.Format("2006-01-02")
	month := now.Format("2006-01")
	year := now.Format("2006")

	type userRow struct {
		id               int64
		balance          int64
		dailyTopup       int64
		monthlyTopup     int64
		yearlyTopup      int64
		lastDailyTopup   string
		lastMonthlyTopup string
		lastYearlyTopup  string
	}

	return s.WithTx(ctx, false, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT
			id, balance_cents, daily_topup_cents, monthly_topup_cents, yearly_topup_cents,
			last_daily_topup, last_monthly_topup, last_yearly_topup
			FROM users`)
		if err != nil {
			return err
		}

		var users []userRow
		for rows.Next() {
			var u userRow
			if err := rows.Scan(
				&u.id, &u.balance, &u.dailyTopup, &u.monthlyTopup, &u.yearlyTopup,
				&u.lastDailyTopup, &u.lastMonthlyTopup, &u.lastYearlyTopup,
			); err != nil {
				return err
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		for _, u := range users {
			changed := false
			balance := u.balance
			lastDaily := u.lastDailyTopup
			lastMonthly := u.lastMonthlyTopup
			lastYearly := u.lastYearlyTopup

			if u.dailyTopup > 0 && lastDaily != today {
				before := balance
				balance += u.dailyTopup
				if _, err := store.InsertTopup(ctx, tx, u.id, u.dailyTopup, before, balance, "auto_daily", nil, "system"); err != nil {
					return err
				}
				lastDaily = today
				changed = true
			}
			if u.monthlyTopup > 0 && lastMonthly != month {
				before := balance
				balance += u.monthlyTopup
				if _, err := store.InsertTopup(ctx, tx, u.id, u.monthlyTopup, before, balance, "auto_monthly", nil, "system"); err != nil {
					return err
				}
				lastMonthly = month
				changed = true
			}
			if u.yearlyTopup > 0 && lastYearly != year {
				before := balance
				balance += u.yearlyTopup
				if _, err := store.InsertTopup(ctx, tx, u.id, u.yearlyTopup, before, balance, "auto_yearly", nil, "system"); err != nil {
					return err
				}
				lastYearly = year
				changed = true
			}

			if changed {
				if _, err := tx.ExecContext(ctx, `UPDATE users SET
					balance_cents = ?, last_daily_topup = ?, last_monthly_topup = ?, last_yearly_topup = ?, updated_at = ?
					WHERE id = ?`,
					balance, lastDaily, lastMonthly, lastYearly, time.Now().UTC().Format(time.RFC3339), u.id,
				); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func cleanupOldPrints(ctx context.Context, s *store.Store, uploads string, now time.Time) error {
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
		return err
	}
	if retentionDays <= 0 {
		return nil
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
		return err
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

	if len(paths) > 0 {
		if _, err := s.DB.ExecContext(ctx, "VACUUM"); err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return err
		}
	}
	return nil
}
