package gc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestLoadConfigInvalidValues(t *testing.T) {
	t.Setenv("CELLP_GC_INTERVAL", "not-a-duration")
	t.Setenv("CELLP_GC_RETENTION_DAYS", "bad")
	cfg := LoadConfig()
	if cfg.Interval != defaultInterval {
		t.Fatalf("interval %v", cfg.Interval)
	}
	if cfg.Retention != defaultRetention {
		t.Fatalf("retention %v", cfg.Retention)
	}
}

func TestRunOncePurgesDestroyedVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := registry.Open(filepath.Join(dir, "gc2.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_, _ = s.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = s.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDestroyed, nil)

	res, err := RunOnce(ctx, s, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if res.VersionsDeleted < 1 {
		t.Fatalf("versions purged: %+v", res)
	}
}

func TestStartDisabledReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	s, err := registry.Open(filepath.Join(dir, "gc3.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Start(ctx, s, Config{Enabled: false})
}
