package registry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigrationDeadlineUnderWriteLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open-deadline.sqlite")
	const migrateTimeout = 600 * time.Millisecond

	release := make(chan struct{})
	lockReady := make(chan struct{})
	lockErr := make(chan error, 1)
	go holdRegistryWriteLockForTest(path, release, lockReady, lockErr)
	<-lockReady

	start := time.Now()
	_, err := OpenWithOptions(path, OpenOptions{MigrateTimeout: migrateTimeout})
	elapsed := time.Since(start)
	close(release)

	select {
	case err := <-lockErr:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}

	if err == nil {
		t.Fatal("expected Open to fail while write lock held")
	}
	if elapsed > migrateTimeout+900*time.Millisecond {
		t.Fatalf("Open blocked too long: %s (budget %s)", elapsed, migrateTimeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !isBusy(err) {
		t.Fatalf("expected deadline or busy, got %v (%T)", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected migration to end with context deadline exceeded, got %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open after lock release: %v", err)
	}
	store.Close()
}
