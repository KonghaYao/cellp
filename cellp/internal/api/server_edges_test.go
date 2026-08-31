package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestServerEdges(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/missing", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get missing project: %d", w.Code)
	}

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions?since=not-a-time", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid since: %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions?status=ready&limit=5", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list versions filter: %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects?q=demo&limit=500", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list projects: %d", w.Code)
	}

	prod := "v-prod"
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: prod, ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", prod)
	body := bytes.NewBufferString(`{"id":"v-pr","parent_version_id":"v-prod","git_ref":"refs/pull/1/merge"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("fork prod pr: %d %s", w.Code, w.Body.String())
	}

	t.Setenv("CELLP_QUEUE_MAX", "1")
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-pending", ProjectID: "demo"})
	_, _ = store.EnqueueJob(ctx, "demo", "v-pending", registry.StatusFetching)
	body = bytes.NewBufferString(`{"id":"v-new"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("queue full: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics: %d", w.Code)
	}

	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-route", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-route", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-route", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 1,
	})
	req = httptest.NewRequest(http.MethodGet, "/v1/runtime/routes", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("runtime routes: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/health/deep", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Fatalf("deep health: %d", w.Code)
	}
}

func TestCreateVersionInvalidJSON(t *testing.T) {
	srv, store, _ := testAPI(t, "d", "a")
	defer store.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", bytes.NewBufferString("{"))
	req.Header.Set("Authorization", "Bearer d")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}
