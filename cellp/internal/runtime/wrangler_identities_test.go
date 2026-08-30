package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyBindingIdentitiesFromParentKV(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	parentJSON := `{
  "kv_namespaces": [{"binding": "KV", "id": "parent-ns-id"}]
}`
	childJSON := `{
  "kv_namespaces": [{"binding": "KV", "id": "child-placeholder"}]
}`
	if err := os.WriteFile(filepath.Join(parent, "wrangler.json"), []byte(parentJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "wrangler.json"), []byte(childJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyBindingIdentitiesFromParent(parent, child); err != nil {
		t.Fatalf("CopyBindingIdentitiesFromParent: %v", err)
	}
	got, err := ParseBindings(child)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.KV) != 1 || got.KV[0].ID != "parent-ns-id" {
		t.Fatalf("child kv id = %+v, want parent-ns-id", got.KV)
	}
}
