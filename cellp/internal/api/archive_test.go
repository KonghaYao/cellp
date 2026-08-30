package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestArchiveProd422(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetProdVersion(ctx, "demo", "v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/archive", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestPinThenArchive409(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/pin", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pin status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/archive", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("archive pinned: status = %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/unpin", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unpin status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/archive", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("archive after unpin: status = %d body=%s", w.Code, w.Body.String())
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusArchived {
		t.Fatalf("status = %v", v)
	}
}

func TestWakeRestoresReadyAPI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusArchived, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/wake", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("wake status = %d body=%s", w.Code, w.Body.String())
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("status = %v", v)
	}
}

func TestDestroyParentWithChild409(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	parent := "v-parent"
	child := "v-child"
	pid := parent
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: parent, ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: child, ProjectID: "demo", ParentVersionID: &pid,
	})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", child, registry.StatusArchived, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/demo/versions/"+parent, nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
