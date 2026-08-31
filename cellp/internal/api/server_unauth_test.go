package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnauthorizedAdminRoutes(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCreateProjectMissingID(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/projects", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
