package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupCelldStub(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(root, "celld-args.log")
	celld := filepath.Join(bin, "celld")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + argsLog + "\n"
	if err := os.WriteFile(celld, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return root, argsLog
}

func TestKvBranchArgv(t *testing.T) {
	_, argsLog := setupCelldStub(t)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.KvBranch(context.Background(), "demo", "v-child", "v-parent", "ns-abc"); err != nil {
		t.Fatalf("KvBranch: %v", err)
	}
	raw, _ := os.ReadFile(argsLog)
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"kv", "branch", "ns-abc",
		"--parent-bucket", "s3://cellp-celld/demo/v-parent",
		"--bucket", "s3://cellp-celld/demo/v-child",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
	assertCelldArgv(t, args, want)
}

func TestQueueBranchArgv(t *testing.T) {
	_, argsLog := setupCelldStub(t)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.QueueBranch(context.Background(), "demo", "v-child", "v-parent", "orders"); err != nil {
		t.Fatalf("QueueBranch: %v", err)
	}
	raw, _ := os.ReadFile(argsLog)
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"queue", "branch", "orders",
		"--parent-bucket", "s3://cellp-celld/demo/v-parent",
		"--bucket", "s3://cellp-celld/demo/v-child",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
	assertCelldArgv(t, args, want)
}

func TestR2BranchArgv(t *testing.T) {
	_, argsLog := setupCelldStub(t)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.R2Branch(context.Background(), "demo", "v-child", "v-parent", "assets"); err != nil {
		t.Fatalf("R2Branch: %v", err)
	}
	raw, _ := os.ReadFile(argsLog)
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"r2", "branch",
		"--name", "assets",
		"--parent-bucket", "s3://cellp-celld/demo/v-parent",
		"--bucket", "s3://cellp-celld/demo/v-child",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
	assertCelldArgv(t, args, want)
}

func assertCelldArgv(t *testing.T, args, want []string) {
	t.Helper()
	if len(args) != len(want) {
		t.Fatalf("celld argv len = %d, want %d\nargs: %q", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("celld argv[%d] = %q, want %q\nfull: %q", i, args[i], want[i], args)
		}
	}
}
