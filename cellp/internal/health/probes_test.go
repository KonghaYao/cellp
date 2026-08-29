package health_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/health"
)

func TestProbeRustFSOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := health.ProbeRustFS(context.Background(), srv.URL)
	if r.Status != "ok" {
		t.Fatalf("status = %q detail=%q", r.Status, r.Detail)
	}
	if r.Name != "rustfs" {
		t.Fatalf("name = %q", r.Name)
	}
}

func TestProbeRustFSDown(t *testing.T) {
	r := health.ProbeRustFS(context.Background(), "http://127.0.0.1:1")
	if r.Status != "down" {
		t.Fatalf("status = %q", r.Status)
	}
}
