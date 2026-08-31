package metrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestStartCollectorExitsOnCancel(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "mc.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rm := runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx, cancel := context.WithCancel(context.Background())
	StartCollector(ctx, store, rm, 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestStartCollectorZeroInterval(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "mc2.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	StartCollector(context.Background(), store, runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"), 0)
}
