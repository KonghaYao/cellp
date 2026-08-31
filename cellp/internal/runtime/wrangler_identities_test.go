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

func TestCopyBindingIdentitiesFromParentFull(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	child := filepath.Join(root, "child")
	for _, d := range []string{parent, child} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	parentJSON := `{
	  "d1_databases":[{"binding":"DB","database_name":"main","database_id":"db-parent"}],
	  "kv_namespaces":[{"binding":"KV","id":"parent-ns"}],
	  "r2_buckets":[{"binding":"R2","bucket_name":"parent-bucket"}],
	  "queues":{"producers":[{"binding":"Q","queue":"tasks"}]}
	}`
	childJSON := `{
	  "d1_databases":[{"binding":"DB","database_name":"main","database_id":"child-placeholder"}],
	  "kv_namespaces":[{"binding":"KV","id":"child-ns"}],
	  "r2_buckets":[{"binding":"R2","bucket_name":"child-bucket"}],
	  "queues":{"producers":[{"binding":"Q","queue":"tasks"}]}
	}`
	if err := os.WriteFile(filepath.Join(parent, "wrangler.json"), []byte(parentJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "wrangler.json"), []byte(childJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyBindingIdentitiesFromParent(parent, child); err != nil {
		t.Fatal(err)
	}
	id, err := D1DatabaseID(child)
	if err != nil || id != "db-parent" {
		t.Fatalf("d1 id=%q err=%v", id, err)
	}
	got, err := ParseBindings(child)
	if err != nil {
		t.Fatal(err)
	}
	if got.R2[0].BucketName != "parent-bucket" || got.KV[0].ID != "parent-ns" {
		t.Fatalf("bindings %+v", got)
	}
}
