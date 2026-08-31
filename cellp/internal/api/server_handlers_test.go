package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestGetVersionNotFound(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/missing", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCreateProjectWithGitRemote(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	remote := "https://example.com/repo.git"
	body := bytes.NewBufferString(`{"id":"gitproj","git_remote":"` + remote + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDestroyVersionNotReady409(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/demo/versions/v1", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListVersionsEmptyProject(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "empty"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/empty/versions", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCreateVersionAutoCreatesProject(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	body := bytes.NewBufferString(`{"id":"v1","git_ref":"main"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/newproj/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	p, _ := store.GetProject(context.Background(), "newproj")
	if p == nil {
		t.Fatal("project should exist")
	}
}
