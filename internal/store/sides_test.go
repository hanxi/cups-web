package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"cups-web/internal/ipp"
)

func TestSidesMigrationAndPersistence(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "sides.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var user User
	var printIDs, taskIDs []int64
	if err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		var err error
		user, err = CreateUser(ctx, tx, CreateUserInput{Username: "test", PasswordHash: "unused", Role: RoleUser})
		if err != nil {
			return err
		}
		for _, duplex := range []bool{false, true} {
			id, err := InsertPrintRecord(ctx, tx, &PrintRecord{UserID: user.ID, Filename: "test.pdf", CreatedAt: "now", IsDuplex: duplex})
			if err != nil {
				return err
			}
			printIDs = append(printIDs, id)
			id, err = InsertScheduledPrint(ctx, tx, &ScheduledPrint{UserID: user.ID, Filename: "test.pdf", CreatedAt: "now", UpdatedAt: "now", IsDuplex: duplex})
			if err != nil {
				return err
			}
			taskIDs = append(taskIDs, id)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-#129 schema with both legacy boolean values populated.
	for _, table := range []string{"print_jobs", "scheduled_prints"} {
		if _, err := s.DB.ExecContext(ctx, "ALTER TABLE "+table+" DROP COLUMN sides"); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := s.migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		for i, want := range []string{ipp.SidesOneSided, ipp.SidesLongEdge} {
			print, err := GetPrintRecordByID(ctx, tx, printIDs[i])
			if err != nil {
				return err
			}
			task, err := GetScheduledPrintByID(ctx, tx, taskIDs[i])
			if err != nil {
				return err
			}
			if print.Sides != want || task.Sides != want || print.IsDuplex != (i == 1) || task.IsDuplex != (i == 1) {
				t.Fatalf("legacy row: print=%+v task=%+v", print, task)
			}
		}
		for _, sides := range []string{ipp.SidesShortEdge, ipp.SidesLongEdge, ipp.SidesOneSided} {
			// Deliberately conflict with the keyword; stored bool must follow sides.
			print := PrintRecord{UserID: user.ID, CreatedAt: "now", Sides: sides, IsDuplex: sides == ipp.SidesOneSided}
			id, err := InsertPrintRecord(ctx, tx, &print)
			if err != nil {
				return err
			}
			loaded, err := GetPrintRecordByID(ctx, tx, id)
			if err != nil {
				return err
			}
			if loaded.Sides != sides || loaded.IsDuplex != (sides != ipp.SidesOneSided) {
				t.Fatalf("print snapshot=%+v", loaded)
			}
			task, err := GetScheduledPrintByID(ctx, tx, taskIDs[0])
			if err != nil {
				return err
			}
			task.Sides, task.IsDuplex = sides, sides == ipp.SidesOneSided
			if err := UpdateScheduledPrint(ctx, tx, &task); err != nil {
				return err
			}
			for _, enabled := range []bool{false, true} {
				if err := SetScheduledPrintEnabled(ctx, tx, task.ID, enabled); err != nil {
					return err
				}
				loaded, err := GetScheduledPrintByID(ctx, tx, task.ID)
				if err != nil {
					return err
				}
				if loaded.Sides != sides || loaded.IsDuplex != (sides != ipp.SidesOneSided) || loaded.Enabled != enabled {
					t.Fatalf("task snapshot=%+v", loaded)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
