package orch

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestStartArchiveReaperRunsOnce(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	bg, cancel := context.WithCancel(ctx)
	defer cancel()
	cfg := LoadArchiveConfig()
	cfg.ReaperInterval = 5 * time.Millisecond
	cfg.Grace = 0
	cfg.Idle = time.Hour
	o.StartArchiveReaper(bg, cfg)
	time.Sleep(25 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
}
