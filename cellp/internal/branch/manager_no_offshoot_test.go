package branch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func isolatedPATH(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestManagerWithoutOffshoot(t *testing.T) {
	isolatedPATH(t)
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "branch.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := New(filepath.Join(dir, "store"), store)
	ctx := context.Background()
	if err := m.EnsureProject(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := m.Fork(ctx, "demo", "main", "child"); err != nil {
		t.Fatal(err)
	}
	if err := m.Drain(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Promote(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestManagerPromoteInjectFail(t *testing.T) {
	isolatedPATH(t)
	t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "1")
	t.Cleanup(func() { t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "") })

	m := New(filepath.Join(t.TempDir(), "store"), nil)
	err := m.Promote(context.Background(), "demo", "v1")
	if err == nil {
		t.Fatal("expected injected promote failure")
	}
}
