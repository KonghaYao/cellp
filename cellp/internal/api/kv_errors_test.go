package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKVCelldUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "celld_unavailable")
}

func TestPutKVInvalidJSON(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", bytes.NewBufferString(`not-json`))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestPutKVValueRequired(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "value_required")
}
