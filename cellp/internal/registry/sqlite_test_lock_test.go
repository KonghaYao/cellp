package registry

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// holdRegistryWriteLockForTest starts a transaction that holds a write lock on path until release is closed.
// If ready is non-nil, it is closed after the write lock is acquired (test helper only).
// Errors are sent to errc if non-nil; otherwise the helper panics (must not call t.Fatal from a goroutine).
func holdRegistryWriteLockForTest(path string, release <-chan struct{}, ready chan struct{}, errc chan<- error) {
	fail := func(err error) {
		if errc != nil {
			errc <- err
			return
		}
		panic(err)
	}
	dsn := registryDSN(path, 0)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		fail(err)
		return
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS registry_write_lock_probe (id INTEGER PRIMARY KEY)`); err != nil {
		fail(err)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(err)
		return
	}
	if _, err := tx.Exec(`INSERT INTO registry_write_lock_probe DEFAULT VALUES`); err != nil {
		_ = tx.Rollback()
		fail(err)
		return
	}
	if ready != nil {
		close(ready)
	}
	<-release
	if err := tx.Rollback(); err != nil {
		fail(fmt.Errorf("rollback write-lock probe tx: %w", err))
	}
}
