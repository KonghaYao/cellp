package serve_test

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/serve"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestRunStopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CELLP_REGISTRY_DB", filepath.Join(dir, "registry.sqlite"))
	t.Setenv("PLATFORM_PORT", strconv.Itoa(freePort(t)))
	t.Setenv("GATEWAY_PORT", strconv.Itoa(freePort(t)))
	t.Setenv("OFFSHOOT_STORE", filepath.Join(dir, "offshoot"))
	t.Setenv("ARTIFACTS_DIR", filepath.Join(dir, "artifacts"))
	t.Setenv("CELLP_GC_INTERVAL", "0")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve.Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			return
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for Run to exit")
	}
}
