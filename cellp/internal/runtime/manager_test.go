package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD1BranchPassesParentBucket(t *testing.T) {
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

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "name": "d1-seed",
  "d1_databases": [
    { "binding": "DB", "database_name": "guestbook", "database_id": "00000000-0000-0000-0000-000000000000" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Branch(context.Background(), "demo", "v-child", "v-parent", projectDir); err != nil {
		t.Fatalf("D1Branch: %v", err)
	}

	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"d1", "branch", "guestbook",
		"--parent-bucket", "s3://cellp-celld/demo/v-parent",
		projectDir,
		"--bucket", "s3://cellp-celld/demo/v-child",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
	if len(args) != len(want) {
		t.Fatalf("celld argv len = %d, want %d\nargs: %q", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("celld argv[%d] = %q, want %q\nfull: %q", i, args[i], want[i], args)
		}
	}
}

func TestD1BranchSkipsWhenNoD1Databases(t *testing.T) {
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

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(`{ "name": "counter" }`), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Branch(context.Background(), "demo", "v-child", "v-parent", projectDir); err != nil {
		t.Fatalf("D1Branch: %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("celld should not run when wrangler has no d1_databases")
	}
}

func TestD1BranchFailsWhenMultipleD1Databases(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	celld := filepath.Join(bin, "celld")
	if err := os.WriteFile(celld, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "d1_databases": [
    { "database_name": "a", "database_id": "1" },
    { "database_name": "b", "database_id": "2" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Branch(context.Background(), "demo", "v-child", "v-parent", projectDir); err == nil {
		t.Fatal("expected error when wrangler has more than one d1_databases entry")
	}
}

func TestSetD1DatabaseID(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "d1_databases": [
    { "database_name": "guestbook", "database_id": "old-id" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetD1DatabaseID(projectDir, "parent-id"); err != nil {
		t.Fatal(err)
	}
	got, err := D1DatabaseID(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "parent-id" {
		t.Fatalf("database_id = %q, want parent-id", got)
	}
}

func TestD1ExecutePassesDatabaseName(t *testing.T) {
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

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "name": "d1-seed",
  "d1_databases": [
    { "binding": "DB", "database_name": "guestbook", "database_id": "00000000-0000-0000-0000-000000000000" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	seedPath := filepath.Join(root, "seed.db")
	if err := os.WriteFile(seedPath, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Execute(context.Background(), "demo", "v1", projectDir, seedPath); err != nil {
		t.Fatalf("D1Execute: %v", err)
	}

	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		"d1", "import", "guestbook",
		"--file", seedPath,
		projectDir,
		"--bucket", "s3://cellp-celld/demo/v1",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
	if len(args) != len(want) {
		t.Fatalf("celld argv len = %d, want %d\nargs: %q", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("celld argv[%d] = %q, want %q\nfull: %q", i, args[i], want[i], args)
		}
	}
}

func TestD1ExecuteRejectsMultipleD1Databases(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	celld := filepath.Join(bin, "celld")
	if err := os.WriteFile(celld, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "d1_databases": [
    { "database_name": "a" },
    { "database_name": "b" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(root, "seed.db")
	if err := os.WriteFile(seedPath, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	err := m.D1Execute(context.Background(), "demo", "v1", projectDir, seedPath)
	if err == nil || !strings.Contains(err.Error(), "only one is supported") {
		t.Fatalf("expected multiple-databases error, got %v", err)
	}
}

func TestD1ExecuteSkipsWhenNoD1Databases(t *testing.T) {
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

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(`{ "name": "counter" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(root, "seed.db")
	if err := os.WriteFile(seedPath, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Execute(context.Background(), "demo", "v1", projectDir, seedPath); err != nil {
		t.Fatalf("D1Execute: %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("celld should not run when wrangler has no d1_databases")
	}
}

func TestD1ExecuteSQLFileUsesExecuteSubcommand(t *testing.T) {
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

	projectDir := filepath.Join(root, "project")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{ "d1_databases": [{ "database_name": "guestbook" }] }`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.jsonc"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	seedPath := filepath.Join(root, "seed.sql")
	if err := os.WriteFile(seedPath, []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Execute(context.Background(), "demo", "v1", projectDir, seedPath); err != nil {
		t.Fatalf("D1Execute: %v", err)
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 3 || lines[0] != "d1" || lines[1] != "execute" || lines[2] != "guestbook" {
		t.Fatalf("expected d1 execute guestbook, got %q", lines)
	}
}

func TestEphemeralWatchDirDefault(t *testing.T) {
	t.Setenv("CELLP_CELLD_WATCH_PERSIST", "")
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	dir, err := m.allocateWatchDir("demo-app", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("watch dir missing: %v", err)
	}
	if strings.Contains(dir, filepath.Join("dev", "data", "celld-watch")) {
		t.Fatalf("expected ephemeral temp watch, got %q", dir)
	}
	removeEphemeralWatch(dir)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("ephemeral watch should be removed, stat err=%v", err)
	}
}

func TestPersistentWatchDirOptIn(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("CELLP_CELLD_WATCH_PERSIST", "1")
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	dir, err := m.allocateWatchDir("demo", "v-test")
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := filepath.Join("dev", "data", "celld-watch", "demo", "v-test")
	if !strings.HasSuffix(dir, wantSuffix) {
		t.Fatalf("unexpected persist path %q want suffix %q", dir, wantSuffix)
	}
}
