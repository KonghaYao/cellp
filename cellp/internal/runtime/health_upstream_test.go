package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestHealthUnhealthyWhenUpstreamFails(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	_ = os.Mkdir(bin, 0o755)
	_ = os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	if m.Health(context.Background(), "127.0.0.1", port) {
		t.Fatal("expected unhealthy")
	}
}
