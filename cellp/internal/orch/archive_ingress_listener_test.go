package orch

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

// TestArchiveReadyVersionClosesDedicatedListener covers archive → teardown → reconcile → TCP unreachable (P5c R-ARCHIVE-TEARDOWN).
func TestArchiveReadyVersionClosesDedicatedListener(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := registry.OpenWithOptions(filepath.Join(dir, "archive-listeners.sqlite"), registry.OpenOptions{
		IngressPortMin: 19080,
		IngressPortMax: 19095,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const gwID = "gw-self"
	cfg := config.Config{ArtifactsDir: dir, ArtifactsBucket: "cellp-artifacts", InstanceID: gwID}
	cfg.Ingress.TierB = config.IngressTierDedicatedPort
	o := New(store, job.NewSQLiteQueue(store), branch.New(dir+"/off", store),
		runtime.New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s"),
		&artifact.Store{Bucket: "cellp-artifacts", LocalDir: dir}, cfg)

	gwCfg := gateway.GatewayConfig{GatewayPort: 8787, GatewayID: gwID, HostOnly: true}
	gw := gateway.NewWithConfig(store, gwCfg)
	lm := gateway.NewListenerManager(gw, store, gwCfg)
	t.Cleanup(func() { lm.CloseAll(ctx) })
	o.SetIngressListenerReconciler(lm)

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	_, _, err = o.ensurePreviewIngress(ctx, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	bid := previewBindingID("demo", "v1")
	b, _ := store.GetIngressBinding(ctx, bid)
	if b == nil || b.ListenPort == nil {
		t.Fatalf("binding %+v", b)
	}
	port := *b.ListenPort

	client := &http.Client{Timeout: 2 * time.Second}
	if _, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port)); err != nil {
		t.Fatalf("listener should be open before archive: %v", err)
	}

	if err := o.Archive(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusArchived {
		t.Fatalf("status=%v", v)
	}
	if pa, _ := store.GetActivePortAllocationByOwner(ctx, registry.PortOwnerIngressBinding, bid); pa != nil {
		t.Fatalf("ephemeral ledger should be released: %+v", pa)
	}
	b, _ = store.GetIngressBinding(ctx, bid)
	if b == nil || !b.Active || b.ListenPort != nil {
		t.Fatalf("archived preview binding should stay active without a listener: %+v", b)
	}

	short := &http.Client{Timeout: 400 * time.Millisecond}
	if _, err := short.Get(fmt.Sprintf("http://127.0.0.1:%d/", port)); err == nil {
		t.Fatal("expected connection failure after archive teardown")
	}
}
