package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyGatewayRouteHostOK(t *testing.T) {
	host := "v1.demo.ingress.local"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || r.URL.Path != "/.well-known/celld/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := VerifyGatewayRouteHost(context.Background(), srv.URL, host); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGatewayRouteHostBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if err := VerifyGatewayRouteHost(context.Background(), srv.URL, "v1.demo.ingress.local"); err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyGatewayRouteDeprecated(t *testing.T) {
	if err := VerifyGatewayRoute(context.Background(), "http://127.0.0.1:1", "demo", "v1"); err == nil {
		t.Fatal("expected deprecated error")
	}
}
