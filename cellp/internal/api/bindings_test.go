package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestGetBindingsReady(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	d1, ok := resp["d1"].([]interface{})
	if !ok || len(d1) != 1 {
		t.Fatalf("d1 = %v", resp["d1"])
	}
	d10 := d1[0].(map[string]interface{})
	if d10["binding"] != "DB" {
		t.Fatalf("d1[0].binding = %v", d10["binding"])
	}
	for _, key := range []string{"kv", "queues", "workflows", "r2", "crons"} {
		arr, ok := resp[key].([]interface{})
		if !ok || len(arr) != 0 {
			t.Fatalf("%s = %v (want empty array)", key, resp[key])
		}
	}
}

func TestGetBindingsVersionNotFound(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/missing/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "version_not_found")
}

func TestGetBindingsVersionNotReady(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "version_not_ready")
}

func TestGetBindingsNoWrangler(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	projectDir := filepath.Join(artifactsDir, "demo", "v1")
	_ = os.Remove(filepath.Join(projectDir, "wrangler.json"))

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "wrangler_not_found")
}

func TestGetBindingsEmptyDeclared(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{"name":"x"}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"d1", "kv", "queues", "workflows", "r2", "crons"} {
		arr, ok := resp[key].([]interface{})
		if !ok || len(arr) != 0 {
			t.Fatalf("%s = %v (want empty array)", key, resp[key])
		}
	}
}

func TestGetBindingsInvalidJSONC(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)
	projectDir := filepath.Join(artifactsDir, "demo", "v1")
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(`{ binding:`), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/bindings", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestCORSAllowsPUT(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodOptions, "/v1/projects/demo/versions/v1/bindings", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	methods := w.Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(methods, "PUT") {
		t.Fatalf("Allow-Methods = %q", methods)
	}
}

func setupReadyVersion(t *testing.T, store *registry.SQLiteStore, artifactsDir, wrangler string) {
	t.Helper()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	projectDir := filepath.Join(artifactsDir, "demo", "v1")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertAPIError(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body = %s", w.Body.String())
	}
	if resp["error"] != code {
		t.Fatalf("error = %q want %q body=%s", resp["error"], code, w.Body.String())
	}
}
