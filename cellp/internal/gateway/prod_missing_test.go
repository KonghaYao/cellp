package gateway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestProdHostWithoutProdVersion(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/noprod.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp := doHostGet(t, srv.URL, "demo.ingress.local", "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
