package gateway_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
	"github.com/google/uuid"
)

func hostOnlyGW(store registry.Store) *gateway.Gateway {
	cfg := gateway.ConfigFromEnv()
	cfg.HostOnly = true
	return gateway.NewWithConfig(store, cfg)
}

func upsertPreviewBinding(t *testing.T, store *registry.SQLiteStore, project, version, host, synthetic string) {
	t.Helper()
	ctx := context.Background()
	vid := version
	if err := store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: uuid.NewString(), ProjectID: project, VersionID: &vid,
		Role: registry.IngressRolePreview, Host: &host, SyntheticHost: synthetic, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func upsertProdBinding(t *testing.T, store *registry.SQLiteStore, project, host, synthetic string) {
	t.Helper()
	ctx := context.Background()
	if err := store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: uuid.NewString(), ProjectID: project, VersionID: nil,
		Role: registry.IngressRoleProd, Host: &host, SyntheticHost: synthetic, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func doHostGet(t *testing.T, srvURL, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srvURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func upstreamHostPort(t *testing.T, upstreamURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatal(err)
	}
	port := 0
	if parsed.Port() != "" {
		fmt.Sscanf(parsed.Port(), "%d", &port)
	}
	return parsed.Hostname(), port
}
