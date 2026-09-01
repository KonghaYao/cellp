package gateway

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestResolveIngressBindingByLocalListenPort(t *testing.T) {
	dir := t.TempDir()
	store, err := registry.OpenWithOptions(filepath.Join(dir, "resolve.sqlite"), registry.OpenOptions{
		IngressPortMin: 19080,
		IngressPortMax: 19095,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	gwID := "gw-1"
	gwOwner := gwID
	prodPort := 19083

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	bid := "demo-prod"
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", Role: registry.IngressRoleProd,
		SyntheticHost: "prod.syn", Active: true, OwnerGatewayID: &gwOwner, ListenPort: &prodPort,
	})

	cfg := GatewayConfig{
		GatewayPort: 8787, GatewayID: gwID, HostOnly: true, IngressTierB: "host",
	}
	gw := NewWithConfig(store, cfg)
	req := httptest.NewRequest("GET", "http://127.0.0.1:19083/", nil)
	req = req.WithContext(WithLocalListenPort(req.Context(), prodPort))
	b, err := gw.resolveIngressBinding(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || b.Role != registry.IngressRoleProd {
		t.Fatalf("binding=%+v", b)
	}
}
