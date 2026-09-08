package orch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
)

func TestCronShouldArm(t *testing.T) {
	vProd := "v-prod"
	vPreview := "v-preview"
	tests := []struct {
		name string
		proj *registry.Project
		vid  string
		want bool
	}{
		{"nil project", nil, vPreview, true},
		{"no prod yet", &registry.Project{ID: "p"}, vPreview, true},
		{"preview when prod set", &registry.Project{ID: "p", ProdVersionID: &vProd}, vPreview, false},
		{"prod version", &registry.Project{ID: "p", ProdVersionID: &vProd}, vProd, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CronShouldArm(tc.proj, tc.vid); got != tc.want {
				t.Fatalf("CronShouldArm() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnsureCronResidentDesire(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(filepath.Join(t.TempDir(), "cron-desire.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "wrangler.json"), []byte(`{"name":"cron","triggers":{"crons":["* * * * *"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{store: store, cfg: config.Config{Serving: config.LoadServingDefaults()}}

	if err := o.ensureCronResidentDesire(ctx, "demo", "v1", true, bundle); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetServingDesire(ctx, "demo", "v1")
	if err != nil || d == nil || d.DesiredReplicas != 1 || d.Reason != desireReasonCronResident {
		t.Fatalf("desire: %+v err=%v", d, err)
	}
}

func TestVersionBundleDirUsesArtifactDir(t *testing.T) {
	dir := t.TempDir()
	artDir := filepath.Join(dir, "artifacts", "demo", "v1")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "wrangler.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ArtifactsDir: filepath.Join(dir, "artifacts")}
	got, err := versionBundleDir(cfg, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(got, "wrangler.json")); err != nil {
		t.Fatalf("bundle dir %s: %v", got, err)
	}
}
