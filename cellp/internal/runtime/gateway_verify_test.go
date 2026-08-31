package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyGatewayRouteOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo/v1/.well-known/celld/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := VerifyGatewayRoute(context.Background(), srv.URL, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGatewayRouteBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if err := VerifyGatewayRoute(context.Background(), srv.URL, "demo", "v1"); err == nil {
		t.Fatal("expected error")
	}
}
