package orch

import (
	"testing"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
)

func TestEnsurePreviewIngressDedicatedPort(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	t.Setenv("CELLP_INGRESS_TIER_B", config.IngressTierDedicatedPort)
	o.cfg.Ingress.TierB = config.IngressTierDedicatedPort
	o.cfg.InstanceID = "gw-test"

	_, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	_, previewURL, err := o.ensurePreviewIngress(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if previewURL == "" || previewURL[:len("http://127.0.0.1:")] != "http://127.0.0.1:" {
		t.Fatalf("previewURL %q", previewURL)
	}
	bid := previewBindingID("demo", "v1")
	b, _ := store.GetIngressBinding(ctx, bid)
	if b == nil || b.ListenPort == nil {
		t.Fatalf("binding %+v", b)
	}
	pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid)
	if pa == nil {
		t.Fatal("ledger missing")
	}
}

func TestEnsureProdIngressPreservesListenPort(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	t.Setenv("CELLP_INGRESS_TIER_B", config.IngressTierDedicatedPort)
	o.cfg.Ingress.TierB = config.IngressTierDedicatedPort

	port := 19083
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo", ProdListenPort: &port})
	_ = o.ensureProdIngress(ctx, "demo")
	b1, _ := store.GetIngressBinding(ctx, prodBindingID("demo"))
	if b1 == nil || b1.ListenPort == nil || *b1.ListenPort != port {
		t.Fatalf("first prod ingress: %+v", b1)
	}
	_ = o.ensureProdIngress(ctx, "demo")
	b2, _ := store.GetIngressBinding(ctx, prodBindingID("demo"))
	if b2 == nil || b2.ListenPort == nil || *b2.ListenPort != port {
		t.Fatalf("promote must preserve port: %+v", b2)
	}
}

func TestTeardownPreviewIngressDetach(t *testing.T) {
	o, store, ctx := newTestOrch(t)
	o.cfg.Ingress.TierB = config.IngressTierDedicatedPort
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _, _ = o.ensurePreviewIngress(ctx, "demo", "v1")
	o.teardownPreviewIngress(ctx, "demo", "v1", "test")
	bid := previewBindingID("demo", "v1")
	b, _ := store.GetIngressBinding(ctx, bid)
	if b != nil && b.ListenPort != nil {
		t.Fatalf("listen_port should be cleared: %+v", b)
	}
	if pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid); pa != nil {
		t.Fatalf("ephemeral ledger should be released (R-ARCHIVE-TEARDOWN): %+v", pa)
	}
}
