package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func TestHandlerPrometheusFormat(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.Open(filepath.Join(dir, "metrics.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8803,
	})

	rm := runtime.New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := Collect(ctx, store, rm); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# TYPE cellp_pending_jobs gauge",
		"cellp_pending_jobs 1",
		"# TYPE cellp_routes_active gauge",
		"cellp_routes_active 1",
		"# TYPE cellp_celld_healthy gauge",
		"# TYPE cellp_celld_unhealthy gauge",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}
