package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestRegistryGuardFailClosedWithoutStoreOrHolder(t *testing.T) {
	ctx := context.Background()
	if err := (RegistryGuard{}).HoldsWriteLock(ctx); !errors.Is(err, ErrGuardLost) {
		t.Fatalf("empty guard: %v", err)
	}
	if err := (RegistryGuard{Store: nil, HolderID: "h"}).HoldsWriteLock(ctx); !errors.Is(err, ErrGuardLost) {
		t.Fatalf("nil store: %v", err)
	}
	store, err := registry.Open(t.TempDir() + "/guard.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := (RegistryGuard{Store: store, HolderID: ""}).HoldsWriteLock(ctx); !errors.Is(err, ErrGuardLost) {
		t.Fatalf("empty holder: %v", err)
	}
}
