package orch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func installFakeCelldDeployOnly(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = deploy ]; then
  exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestShouldSkipInactiveDisarmCronReconcile(t *testing.T) {
	if !shouldSkipInactiveDisarmCronReconcile(false, false, false) {
		t.Fatal("expected skip when disarming drained dead upstream")
	}
	cases := []struct {
		arm, active, healthy bool
	}{
		{true, false, false},
		{false, true, false},
		{false, false, true},
	}
	for _, tc := range cases {
		if shouldSkipInactiveDisarmCronReconcile(tc.arm, tc.active, tc.healthy) {
			t.Fatalf("unexpected skip for arm=%v active=%v healthy=%v", tc.arm, tc.active, tc.healthy)
		}
	}
}

func TestReconcileCronNewProdUnhealthyStillFails(t *testing.T) {
	installFakeCelldDeployOnly(t)
	o, store, ctx := newTestOrch(t)
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v-new", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v-new", registry.StatusReady, nil)
	_ = store.SetProdVersionCAS(ctx, "demo", "", "v-new")
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v-new", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 1,
	})
	bundle := filepath.Join(o.cfg.ArtifactsDir, "demo", "v-new")
	_ = os.MkdirAll(bundle, 0o755)
	_ = os.WriteFile(filepath.Join(bundle, "wrangler.json"), []byte(`{"name":"x"}`), 0o644)

	err := o.ReconcileCronAfterProdChange(ctx, "demo", "", "v-new")
	if err == nil || !strings.Contains(err.Error(), "unreachable or unhealthy") {
		t.Fatalf("expected new prod upstream failure, got %v", err)
	}
}
