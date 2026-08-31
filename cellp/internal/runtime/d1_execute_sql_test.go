package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestD1ExecuteSQLWithFakeCelld(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = "d1" ] && [ "$2" = "execute" ]; then
  echo '{"n":1}'
  echo "Executed 1 statement(s) in 3ms" >&2
  exit 0
fi
exit 1
`
	celld := filepath.Join(bin, "celld")
	if err := os.WriteFile(celld, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	projectDir := filepath.Join(root, "proj")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{"d1_databases":[{"database_name":"main"}]}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	res, err := m.D1ExecuteSQL(context.Background(), "demo", "v1", projectDir, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if res.DurationMS != 3 || len(res.Rows) != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestD1ExecuteSQLErrorFromCelld(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	_ = os.Mkdir(bin, 0o755)
	script := `#!/bin/sh
echo "syntax error" >&2
exit 1
`
	_ = os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755)
	t.Setenv("PATH", bin)
	projectDir := filepath.Join(root, "proj")
	_ = os.Mkdir(projectDir, 0o755)
	_ = os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(`{"d1_databases":[{"database_name":"db"}]}`), 0o644)

	m := New(8792, "", "us-east-1", "s3://b/p", "k", "s")
	_, err := m.D1ExecuteSQL(context.Background(), "demo", "v1", projectDir, "BAD")
	if err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("err=%v", err)
	}
}

func TestD1ExecuteSQLNoDatabase(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	_ = os.Mkdir(bin, 0o755)
	_ = os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin)
	m := New(8792, "", "us-east-1", "s3://b/p", "k", "s")
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(`{"name":"x"}`), 0o644)
	_, err := m.D1ExecuteSQL(context.Background(), "demo", "v1", dir, "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "no d1") {
		t.Fatalf("err=%v", err)
	}
}
