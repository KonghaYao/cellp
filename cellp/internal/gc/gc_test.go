package gc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("CELLP_GC_INTERVAL", "")
	t.Setenv("CELLP_GC_RETENTION_DAYS", "")
	cfg := LoadConfig()
	if !cfg.Enabled {
		t.Fatal("expected enabled by default")
	}
	if cfg.Interval != defaultInterval {
		t.Fatalf("interval: got %v want %v", cfg.Interval, defaultInterval)
	}
	if cfg.Retention != defaultRetention {
		t.Fatalf("retention: got %v want %v", cfg.Retention, defaultRetention)
	}
}

func TestLoadConfigDisabled(t *testing.T) {
	t.Setenv("CELLP_GC_INTERVAL", "0")
	cfg := LoadConfig()
	if cfg.Enabled {
		t.Fatal("expected disabled when interval is 0")
	}
}

func TestRunOnceIntegration(t *testing.T) {
	dir := t.TempDir()
	s, err := registry.Open(filepath.Join(dir, "gc.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_, _ = s.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	j, err := s.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}

	res, err := RunOnce(ctx, s, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if res.JobsDeleted != 1 {
		t.Fatalf("expected 1 job purged, got %+v", res)
	}
}
