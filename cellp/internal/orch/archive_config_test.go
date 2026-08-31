package orch

import (
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestLoadArchiveConfigDefaults(t *testing.T) {
	t.Setenv("CELLP_ARCHIVE_GRACE", "")
	t.Setenv("CELLP_ARCHIVE_IDLE", "")
	t.Setenv("CELLP_ROLLBACK_KEEP", "")
	t.Setenv("CELLP_ARCHIVE_REAPER", "")
	cfg := LoadArchiveConfig()
	if cfg.Grace != 15*time.Minute || cfg.Idle != 45*time.Minute {
		t.Fatalf("defaults: %+v", cfg)
	}
	if cfg.ReaperInterval != time.Minute {
		t.Fatalf("reaper interval %v", cfg.ReaperInterval)
	}
}

func TestLoadArchiveConfigCustom(t *testing.T) {
	t.Setenv("CELLP_ARCHIVE_GRACE", "2m")
	t.Setenv("CELLP_ARCHIVE_IDLE", "3m")
	t.Setenv("CELLP_ROLLBACK_KEEP", "4m")
	t.Setenv("CELLP_ARCHIVE_REAPER", "0")
	cfg := LoadArchiveConfig()
	if cfg.Grace != 2*time.Minute || cfg.Idle != 3*time.Minute || cfg.RollbackKeep != 4*time.Minute {
		t.Fatalf("custom: %+v", cfg)
	}
	if cfg.ReaperInterval != 0 {
		t.Fatalf("reaper disabled: %v", cfg.ReaperInterval)
	}
}

func TestParseRollbackKeepForTest(t *testing.T) {
	t.Setenv("CELLP_ROLLBACK_KEEP", "90m")
	if got := ParseRollbackKeepForTest(); got != 90*time.Minute {
		t.Fatalf("got %v", got)
	}
	t.Setenv("CELLP_ROLLBACK_KEEP", "")
	if got := ParseRollbackKeepForTest(); got != 60*time.Minute {
		t.Fatalf("default got %v", got)
	}
}

func TestArchiveRejectsProdAndBadStatus(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	if err := o.Archive(ctx, "demo", "v1"); err == nil {
		t.Fatal("expected cannot archive prod")
	}

	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v2", ProjectID: "demo"})
	if err := o.Archive(ctx, "demo", "v2"); err == nil {
		t.Fatal("expected not ready")
	}
}

func TestMayArchiveUsesUpdatedAtWhenNoReadyOrAccess(t *testing.T) {
	cfg := ArchiveConfig{Grace: 0, Idle: time.Hour, RollbackKeep: 0}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	updated := now.Add(-2 * time.Hour)
	v := &registry.Version{
		ID: "v1", Status: registry.StatusReady, UpdatedAt: updated,
	}
	if !MayArchive(&registry.Project{ID: "demo"}, v, cfg, now) {
		t.Fatal("expected idle archive via UpdatedAt")
	}
}
