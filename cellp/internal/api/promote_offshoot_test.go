package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestPromoteOffshootFailed502(t *testing.T) {
	t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "1")
	t.Cleanup(func() { t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL", "") })

	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-old", ProjectID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-old", registry.StatusReady, nil)
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", registry.StatusReady, nil)
	_ = store.SetProdVersionCAS(ctx, "demo", "", "v-old")
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-old", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v-new/promote", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
