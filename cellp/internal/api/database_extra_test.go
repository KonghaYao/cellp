package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestDatabaseBranchMethodOffshootExport(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	dir := filepath.Join(artifactsDir, "demo", "v1")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(`{
	  "d1_databases":[{"binding":"DB","database_name":"main","database_id":"db-1"}]
	}`), 0o644)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseHiddenTableRejected(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables/_cf_KV/rows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseCelldUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseQueryInvalidJSON(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/database/query",
		bytes.NewBufferString(`{`))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestDatabaseRowsWithOffset(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables/users/rows?limit=1&offset=1", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
