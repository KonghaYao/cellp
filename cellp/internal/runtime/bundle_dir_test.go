package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveVersionBundleDirUsesArtifactWranglerJSON(t *testing.T) {
	dir := t.TempDir()
	artDir := filepath.Join(dir, "artifacts", "demo", "v1")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "wrangler.json"), []byte(`{"name":"native"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveVersionBundleDir(filepath.Join(dir, "artifacts"), "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(artDir)
	if got != want {
		t.Fatalf("bundle dir = %q want %q", got, want)
	}
}

func TestValidateDeployBundleRequiresNativeComponentBytes(t *testing.T) {
	dir := t.TempDir()
	wrangler := `{
	  "name": "native-kv",
	  "cellp": {
	    "execution": "native-http-v1",
	    "component": "component.wasm"
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeployBundle(dir); err == nil {
		t.Fatal("expected missing component error")
	}
	if err := os.WriteFile(filepath.Join(dir, "component.wasm"), []byte("component-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeployBundle(dir); err != nil {
		t.Fatalf("ValidateDeployBundle: %v", err)
	}
}

func TestPrepareDeployBundleCopiesNativeComponent(t *testing.T) {
	dir := t.TempDir()
	wrangler := `{
	  "name": "native-kv",
	  "cellp": {
	    "execution": "native-http-v1",
	    "component": "component.wasm"
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	component := []byte("fixture component bytes")
	if err := os.WriteFile(filepath.Join(dir, "component.wasm"), component, 0o644); err != nil {
		t.Fatal(err)
	}
	deployDir, cleanup, err := PrepareDeployBundle(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(filepath.Join(deployDir, "component.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(component) {
		t.Fatalf("component bytes = %q", got)
	}
}
