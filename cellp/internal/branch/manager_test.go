package branch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestManagerForkExportRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("offshoot"); err != nil {
		t.Skip("offshoot CLI not installed")
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}

	dir := t.TempDir()
	store := filepath.Join(dir, "store")
	seed := filepath.Join(dir, "seed.db")
	out := filepath.Join(dir, "export.db")

	cmd := exec.Command("sqlite3", seed, "CREATE TABLE t(x INTEGER); INSERT INTO t VALUES (42);")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	m := New(store, nil)
	ctx := context.Background()
	if err := m.EnsureProject(ctx, "demo"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// Recreate from seed so main has rows (EnsureProject create is empty).
	_ = exec.Command("offshoot", "-store", store, "destroy", "demo", "--force").Run()
	create := exec.Command("offshoot", "-store", store, "create", "demo", "--from", seed)
	if b, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create --from: %v: %s", err, b)
	}
	if err := m.Fork(ctx, "demo", "main", "child"); err != nil {
		t.Fatalf("fork: %v", err)
	}
	if err := m.Export(ctx, "demo", "child", out); err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err := exec.Command("sqlite3", out, "SELECT x FROM t").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "42\n" {
		t.Fatalf("exported row = %q", got)
	}
	_ = os.Remove(out)
}
