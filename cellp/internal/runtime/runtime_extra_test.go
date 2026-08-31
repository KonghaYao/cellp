package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseWranglerVars(t *testing.T) {
	dir := t.TempDir()
	w := `{"vars":{"A":"1","B":"two"}}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(w), 0o644); err != nil {
		t.Fatal(err)
	}
	vars, err := ParseWranglerVars(dir)
	if err != nil || vars["A"] != "1" || vars["B"] != "two" {
		t.Fatalf("vars=%v err=%v", vars, err)
	}
}

func TestCelldExecFailurePrefix(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	_ = os.Mkdir(bin, 0o755)
	script := "#!/bin/sh\necho 'no key' >&2\nexit 1\n"
	_ = os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755)
	t.Setenv("PATH", bin)

	projectDir := t.TempDir()
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	_, err := m.KvGet(context.Background(), "demo", "v1", projectDir, "ns-1", "missing")
	if err != ErrKVKeyNotFound {
		t.Fatalf("err=%v", err)
	}
}

func TestKvPutUsesPathForDashValue(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	ttl := int64(120)
	if err := m.KvPut(context.Background(), "demo", "v1", projectDir, "ns-1", "k", KvPutInput{
		Value: []byte("-flag"), TTL: &ttl, Metadata: `{"x":1}`,
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(argsLog)
	if !contains(string(raw), "--path") || !contains(string(raw), "--ttl") {
		t.Fatalf("argv=%s", raw)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
