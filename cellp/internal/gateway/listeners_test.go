package gateway_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/registry"
	"github.com/google/uuid"
)

func openListenerTestStore(t *testing.T) *registry.SQLiteStore {
	t.Helper()
	s, err := registry.OpenWithOptions(filepath.Join(t.TempDir(), "listeners.sqlite"), registry.OpenOptions{
		IngressPortMin: 19080,
		IngressPortMax: 19095,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestListenerManagerReconcileOpensDedicatedPort(t *testing.T) {
	store := openListenerTestStore(t)
	ctx := context.Background()
	gwID := "gw-self"
	bid := "demo-v1-preview"
	port := 19081
	gwOwner := gwID

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: registry.IngressRolePreview, SyntheticHost: "syn.local", Active: true,
		OwnerGatewayID: &gwOwner, ListenPort: &port,
	})
	_, err := store.AllocateIngressListenPort(ctx, registry.AllocateIngressListenPortInput{
		Stability: registry.PortStabilityEphemeral,
		OwnerKind: registry.PortOwnerIngressBinding,
		OwnerID:   bid,
		GatewayID: &gwOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Align ledger port with binding (allocate picks first free).
	pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid)
	if pa == nil {
		t.Fatal("missing allocation")
	}
	p := pa.Port
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: registry.IngressRolePreview, SyntheticHost: "syn.local", Active: true,
		OwnerGatewayID: &gwOwner, ListenPort: &p,
	})

	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	parsed, _ := url.Parse(upstream.URL)
	upPort := 0
	_, _ = fmt.Sscanf(parsed.Port(), "%d", &upPort)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: parsed.Hostname(), UpstreamPort: upPort,
	})

	cfg := gateway.GatewayConfig{GatewayPort: 8787, GatewayID: gwID, HostOnly: true}
	gw := gateway.NewWithConfig(store, cfg)
	lm := gateway.NewListenerManager(gw, store, cfg)
	t.Cleanup(func() { lm.CloseAll(ctx) })

	if err := lm.ReconcileIngressListeners(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", p))
	if err != nil {
		t.Fatalf("GET dedicated port: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("health status=%d body=%q", resp.StatusCode, body)
	}

	resp2, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/widgets", p))
	if err != nil {
		t.Fatalf("GET route: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("route status=%d", resp2.StatusCode)
	}
	if gotPath != "/widgets" {
		t.Fatalf("upstream path=%q", gotPath)
	}
}

func TestListenerManagerReconcileClosesAfterDetach(t *testing.T) {
	store := openListenerTestStore(t)
	ctx := context.Background()
	gwID := "gw-self"
	bid := "demo-v1-preview"
	gwOwner := gwID

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: registry.IngressRolePreview, SyntheticHost: "syn.local", Active: true,
		OwnerGatewayID: &gwOwner,
	})
	if err := store.AttachIngressListenPort(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: registry.IngressRolePreview, SyntheticHost: "syn.local", Active: true,
		OwnerGatewayID: &gwOwner,
	}, registry.AllocateIngressListenPortInput{
		OwnerKind: registry.PortOwnerIngressBinding,
		OwnerID:   bid,
		GatewayID: &gwOwner,
	}); err != nil {
		t.Fatal(err)
	}
	pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid)
	if pa == nil {
		t.Fatal("no port")
	}
	p := pa.Port

	cfg := gateway.GatewayConfig{GatewayPort: 8787, GatewayID: gwID, HostOnly: true}
	gw := gateway.NewWithConfig(store, cfg)
	lm := gateway.NewListenerManager(gw, store, cfg)
	t.Cleanup(func() { lm.CloseAll(ctx) })

	if err := lm.ReconcileIngressListeners(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.DetachIngressListenPort(ctx, bid, "test"); err != nil {
		t.Fatal(err)
	}
	_ = store.SetIngressBindingActive(ctx, bid, false)
	if err := lm.ReconcileIngressListeners(ctx); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", p))
	if err == nil {
		t.Fatal("expected connection failure after close")
	}
}

func TestListenerManagerOrphanRelease(t *testing.T) {
	store := openListenerTestStore(t)
	ctx := context.Background()
	gwID := "gw-self"
	gwOwner := gwID
	bid := "orphan-binding"

	_, err := store.AllocateIngressListenPort(ctx, registry.AllocateIngressListenPortInput{
		OwnerKind: registry.PortOwnerIngressBinding,
		OwnerID:   bid,
		GatewayID: &gwOwner,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := gateway.GatewayConfig{GatewayPort: 8787, GatewayID: gwID}
	gw := gateway.NewWithConfig(store, cfg)
	lm := gateway.NewListenerManager(gw, store, cfg)
	t.Cleanup(func() { lm.CloseAll(ctx) })

	if err := lm.ReconcileIngressListeners(ctx); err != nil {
		t.Fatal(err)
	}
	active, _ := store.ListActivePortAllocations(ctx, registry.PortPurposeIngressListen)
	if len(active) != 0 {
		t.Fatalf("orphan should release ledger, still active=%d", len(active))
	}
}

func TestListenerManagerSkipsOtherGatewayID(t *testing.T) {
	store := openListenerTestStore(t)
	ctx := context.Background()
	bid := uuid.NewString()
	other := "other-gw"
	self := "self-gw"

	_, _ = store.AllocateIngressListenPort(ctx, registry.AllocateIngressListenPortInput{
		OwnerKind: registry.PortOwnerIngressBinding,
		OwnerID:   bid,
		GatewayID: &other,
	})
	pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid)
	if pa == nil {
		t.Fatal("allocation missing")
	}
	port := pa.Port
	_ = store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: bid, ProjectID: "demo", Role: registry.IngressRolePreview,
		SyntheticHost: "s", Active: true, OwnerGatewayID: &other, ListenPort: &port,
	})

	cfg := gateway.GatewayConfig{GatewayPort: 8787, GatewayID: self}
	gw := gateway.NewWithConfig(store, cfg)
	lm := gateway.NewListenerManager(gw, store, cfg)
	t.Cleanup(func() { lm.CloseAll(ctx) })

	if err := lm.ReconcileIngressListeners(ctx); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 300 * time.Millisecond}
	_, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err == nil {
		t.Fatal("other gateway_id must not listen on this cellpd")
	}
}
