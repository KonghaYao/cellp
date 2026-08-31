package orch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestStartArchiveReaperDisabled(t *testing.T) {
	o, _, _ := newTestOrch(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.StartArchiveReaper(ctx, ArchiveConfig{ReaperInterval: 0})
}

func TestReconcileCronAfterProdChangeNoCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-old", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-old", registry.StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", registry.StatusReady, nil)
	bundle := filepath.Join(o.cfg.ArtifactsDir, "demo", "v-new")
	_ = os.MkdirAll(bundle, 0o755)
	_ = os.WriteFile(filepath.Join(bundle, "wrangler.json"), []byte(`{"name":"x","triggers":{"crons":["* * * * *"]}}`), 0o644)

	if err := o.ReconcileCronAfterProdChange(ctx, "demo", "v-old", "v-new"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadArchiveConfigReaperDuration(t *testing.T) {
	t.Setenv("CELLP_ARCHIVE_REAPER", "2s")
	cfg := LoadArchiveConfig()
	if cfg.ReaperInterval != 2*time.Second {
		t.Fatalf("got %v", cfg.ReaperInterval)
	}
}
