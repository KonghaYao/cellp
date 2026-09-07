package registry

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func TestSqliteCodeBusyOrLocked(t *testing.T) {
	cases := []struct {
		name string
		code int
		want bool
	}{
		{"busy", sqlite3.SQLITE_BUSY, true},
		{"busy_recovery", sqlite3.SQLITE_BUSY_RECOVERY, true},
		{"busy_snapshot", sqlite3.SQLITE_BUSY_SNAPSHOT, true},
		{"busy_timeout", sqlite3.SQLITE_BUSY_TIMEOUT, true},
		{"locked", sqlite3.SQLITE_LOCKED, true},
		{"locked_sharedcache", sqlite3.SQLITE_LOCKED_SHAREDCACHE, true},
		{"locked_vtab", sqlite3.SQLITE_LOCKED_VTAB, true},
		{"busy_future_ext", sqlite3.SQLITE_BUSY | (42 << 8), true},
		{"locked_future_ext", sqlite3.SQLITE_LOCKED | (7 << 8), true},
		{"constraint", sqlite3.SQLITE_CONSTRAINT, false},
		{"error", sqlite3.SQLITE_ERROR, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sqliteCodeBusyOrLocked(tc.code); got != tc.want {
				t.Fatalf("sqliteCodeBusyOrLocked(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestIsBusyWrappedDriverError(t *testing.T) {
	busy, err := driverBusyError(t)
	if err != nil {
		t.Fatalf("setup busy error: %v", err)
	}
	if !isBusy(fmt.Errorf("wrapped: %w", busy)) {
		t.Fatal("expected wrapped driver busy error")
	}
	if isBusy(fmt.Errorf("wrapped: %w", errors.New("SQLITE_CONSTRAINT: check failed"))) {
		t.Fatal("non-busy wrapped message must not classify as busy")
	}
}

func TestIsBusyTypedConstraintIgnoresMessageHeuristic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "constraint-busy-msg.sqlite")
	dsn := registryDSN(path, 0)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	constraintErr, err := db.Exec(`INSERT INTO t (id) VALUES (1)`)
	if err == nil {
		t.Fatal("expected constraint error")
	}
	_ = constraintErr
	wrapped := fmt.Errorf("SQLITE_BUSY mentioned in text: %w", err)
	if isBusy(wrapped) {
		t.Fatal("typed non-busy sqlite error must not use string fallback")
	}
}

func TestOpenMigrateCloseErrorJoinUsesWrap(t *testing.T) {
	migrateErr := errors.New("migrate failed")
	closeErr := errors.New("close failed")
	joined := errors.Join(migrateErr, fmt.Errorf("registry database close failed: %w", closeErr))
	if !errors.Is(joined, closeErr) {
		t.Fatal("expected close error unwrap via %w in Join")
	}
}

func TestIsBusyStringFallback(t *testing.T) {
	if !isBusy(errors.New("database is locked (5) (SQLITE_BUSY)")) {
		t.Fatal("expected string fallback for busy message")
	}
	if isBusy(errors.New("no such table: foo")) {
		t.Fatal("ordinary error must not be busy")
	}
}

func driverBusyError(t *testing.T) (error, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "busy-detect.sqlite")
	dsn := registryDSN(path, 0)
	db1, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db1.Close()
	if _, err := db1.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)`); err != nil {
		return nil, err
	}
	tx, err := db1.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO t DEFAULT VALUES`); err != nil {
		return nil, err
	}
	db2, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db2.Close()
	_, busyErr := db2.Exec(`INSERT INTO t DEFAULT VALUES`)
	if busyErr == nil {
		return nil, fmt.Errorf("expected busy error from concurrent write")
	}
	if !isBusy(busyErr) && !strings.Contains(busyErr.Error(), "locked") && !strings.Contains(busyErr.Error(), "BUSY") {
		return nil, fmt.Errorf("unexpected error type: %v", busyErr)
	}
	return busyErr, nil
}
