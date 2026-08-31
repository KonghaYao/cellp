package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestProjectAndVersionCRUD(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()

	body := bytes.NewBufferString(`{"id":"demo"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("create project: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get project: %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list projects: %d", w.Code)
	}

	verBody := bytes.NewBufferString(`{"id":"v1","git_ref":"main","git_sha":"abc"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", verBody)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create version: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get version: %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list versions: %d", w.Code)
	}

	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/promote", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("promote: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/projects/demo/versions/v1", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("destroy: %d %s", w.Code, w.Body.String())
	}
}
