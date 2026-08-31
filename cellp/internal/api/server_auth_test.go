package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestCORSOptions(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestDeployTokenCannotUseAdminRoutes(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy-only", "admin-only")
	defer store.Close()
	req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer deploy-only")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminTokenCannotDeploy(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy-only", "admin-only")
	defer store.Close()
	body := bytes.NewBufferString(`{"id":"v1"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer admin-only")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPromoteNotReady409(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/promote", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
