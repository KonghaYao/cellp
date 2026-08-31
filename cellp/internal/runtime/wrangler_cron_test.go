package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripCronsFromWranglerJSON(t *testing.T) {
	in := []byte(`{
  "name": "cron",
  "main": "index.js",
  "triggers": { "crons": ["* * * * *"] }
}`)
	out, err := stripCronsFromWranglerJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["triggers"]; ok {
		t.Fatalf("expected triggers removed, got %v", m["triggers"])
	}
}

func TestPrepareDeployBundlePreservesArtifact(t *testing.T) {
	dir := t.TempDir()
	orig := `{
  "name": "cron",
  "triggers": { "crons": ["0 * * * *"] }
}`
	path := filepath.Join(dir, "wrangler.jsonc")
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte("// x"), 0o644); err != nil {
		t.Fatal(err)
	}

	deployDir, cleanup, err := PrepareDeployBundle(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	stripped, err := os.ReadFile(filepath.Join(deployDir, "wrangler.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stripped), "crons") {
		t.Fatalf("deploy copy should not contain crons: %s", stripped)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != orig {
		t.Fatalf("artifact wrangler changed")
	}
}

func TestPrepareDeployBundleIncludeCrons(t *testing.T) {
	dir := t.TempDir()
	orig := `{"name":"cron","triggers":{"crons":["* * * * *"]}}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	deployDir, cleanup, err := PrepareDeployBundle(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	raw, err := os.ReadFile(filepath.Join(deployDir, "wrangler.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "crons") {
		t.Fatalf("expected crons kept: %s", raw)
	}
}
