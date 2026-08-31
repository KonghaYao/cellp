package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKvR2QueueBranchFakeCelld(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	ctx := context.Background()
	if err := m.KvBranch(ctx, "demo", "child", "parent", "ns1"); err != nil {
		t.Fatal(err)
	}
	if err := m.R2Branch(ctx, "demo", "child", "parent", "bucket"); err != nil {
		t.Fatal(err)
	}
	if err := m.QueueBranch(ctx, "demo", "child", "parent", "q1"); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8800, "", "us-east-1", "s3://cellp-celld", "k", "s")
	host, port, err := m.Start(context.Background(), "demo", "v1")
	if err != nil || host == "" || port == 0 {
		t.Fatalf("start %s:%d err=%v", host, port, err)
	}
}
