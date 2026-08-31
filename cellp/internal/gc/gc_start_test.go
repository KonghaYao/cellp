package gc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestStartBackgroundOnce(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "gc-bg.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := Config{Enabled: true, Interval: 5 * time.Millisecond, Retention: time.Hour}
	Start(ctx, store, cfg)
	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
}

func TestLoadConfigRetentionDays(t *testing.T) {
	t.Setenv("CELLP_GC_RETENTION_DAYS", "14")
	cfg := LoadConfig()
	if cfg.Retention != 14*24*time.Hour {
		t.Fatalf("retention %v", cfg.Retention)
	}
}
