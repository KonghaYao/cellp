package serve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestControllerGuardRequired(t *testing.T) {
	if !controllerGuardRequired() {
		t.Fatal("cellpd always requires controller guard")
	}
	if err := requireElasticControllerGuard(true, false); err == nil {
		t.Fatal("elastic skip guard must fail closed")
	}
	if err := requireElasticControllerGuard(true, true); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseControllerGuardOnlyWhenQuiesced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard.sqlite")
	store, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	guardID := "test-guard"
	otherPID := os.Getpid() + 1
	if err := store.TryAcquireControllerGuard(ctx, guardID, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if err := releaseControllerGuardIfQuiesced(ctx, store, guardID, true, false); err != nil {
		t.Fatal(err)
	}
	if err := store.TryAcquireControllerGuard(ctx, "other", otherPID); err != registry.ErrControllerGuardHeld {
		t.Fatalf("guard must remain held when shutdown not quiesced: %v", err)
	}
	if err := releaseControllerGuardIfQuiesced(ctx, store, guardID, true, true); err != nil {
		t.Fatal(err)
	}
	if err := store.TryAcquireControllerGuard(ctx, "other", otherPID); err != nil {
		t.Fatalf("guard should be released: %v", err)
	}
}
