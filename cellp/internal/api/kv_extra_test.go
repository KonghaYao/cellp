package api_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPutKVBase64AndMetadata(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	raw := base64.StdEncoding.EncodeToString([]byte{0, 1, 2})
	body := bytes.NewBufferString(`{"value":"` + raw + `","base64":true,"metadata":{"role":"admin"}}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/bin", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPutKVInvalidBase64(t *testing.T) {
	installOperatorFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupKVQueueVersion(t, store, artifactsDir)

	body := bytes.NewBufferString(`{"value":"%%%","base64":true}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/demo/versions/v1/kv/ns-1/keys/k", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
