package orch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunBindingBranchesAllKinds(t *testing.T) {
	installFakeCelld(t)
	o, _, ctx := newTestOrch(t)
	dir := t.TempDir()
	wrangler := `{
	  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }],
	  "r2_buckets": [{ "binding": "R2", "bucket_name": "assets" }],
	  "queues": {
	    "producers": [{ "binding": "Q", "queue": "tasks" }],
	    "consumers": [{ "queue": "events", "dead_letter_queue": "events-dlq" }]
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.runBindingBranches(ctx, "demo", "child", "parent", dir); err != nil {
		t.Fatal(err)
	}
}
