package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareForwardsSemanticOptions(t *testing.T) {
	called := false
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Allow", "GET, OPTIONS")
		w.Header().Set("Special", "worker-response")
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/methods", nil)

	handler.ServeHTTP(recorder, request)

	if !called {
		t.Fatal("semantic OPTIONS request did not reach upstream handler")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, OPTIONS" {
		t.Fatalf("Allow = %q, want upstream value", got)
	}
	if got := recorder.Header().Get("Special"); got != "worker-response" {
		t.Fatalf("Special = %q, want upstream value", got)
	}
}

func TestCORSMiddlewareHandlesPreflight(t *testing.T) {
	called := false
	handler := corsMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/methods", nil)
	request.Header.Set("Origin", "https://client.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	handler.ServeHTTP(recorder, request)

	if called {
		t.Fatal("CORS preflight unexpectedly reached upstream handler")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}
