package orch

import (
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
