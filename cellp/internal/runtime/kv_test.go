package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeOperatorCelld(t *testing.T) (argsLog, valueLog string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog = filepath.Join(root, "celld-args.log")
	valueLog = filepath.Join(root, "path-value.bin")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + argsLog + "\n" +
		"path_next=0\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$path_next\" = 1 ]; then cp \"$a\" " + valueLog + "; path_next=0; fi\n" +
		"  if [ \"$a\" = \"--path\" ]; then path_next=1; fi\n" +
		"done\n" +
		"case \"$1\" in\n" +
		"kv)\n" +
		"  case \"$2\" in\n" +
		"  list) printf '%s\\n' '{\"name\":\"k0\"}' '{\"name\":\"k1\"}' ;;\n" +
		"  info) printf '%s\\n' '{\"keys\":3,\"bytes\":12,\"stored\":3}' ;;\n" +
		"  get) printf 'hello-value' ;;\n" +
		"  esac\n" +
		"  ;;\n" +
		"queue) printf '%s\\n' '{\"ok\":true}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return argsLog, valueLog
}

func writeOperatorWrangler(t *testing.T, projectDir string) {
	t.Helper()
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }],
  "queues": {
    "producers": [{ "binding": "TASKS", "queue": "tasks" }],
    "consumers": [{ "queue": "events", "dead_letter_queue": "events-dlq" }]
  }
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fleetJSON() []string {
	return []string{
		"--bucket", "s3://cellp-celld/demo/v1",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
		"--json",
	}
}

func fleetNoJSON() []string {
	return []string{
		"--bucket", "s3://cellp-celld/demo/v1",
		"--endpoint", "http://127.0.0.1:9000",
		"--region", "us-east-1",
	}
}

func readArgv(t *testing.T, argsLog string) []string {
	t.Helper()
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func assertArgv(t *testing.T, argsLog string, want []string) {
	t.Helper()
	got := readArgv(t, argsLog)
	if len(got) != len(want) {
		t.Fatalf("argv len=%d want %d\ngot  %q\nwant %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d]=%q want %q\nfull got  %q\nfull want %q", i, got[i], want[i], got, want)
		}
	}
}

func resetArgv(t *testing.T, argsLog string) {
	t.Helper()
	_ = os.Remove(argsLog)
}

func TestKvListArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if _, err := m.KvList(context.Background(), "demo", "v1", projectDir, "ns-1", "app/", "k0", 50); err != nil {
		t.Fatalf("KvList: %v", err)
	}
	want := append([]string{"kv", "list", "ns-1", "--prefix", "app/", "--limit", "50", "--after", "k0"}, fleetJSON()...)
	assertArgv(t, argsLog, want)
}

func TestKvGetArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	got, err := m.KvGet(context.Background(), "demo", "v1", projectDir, "ns-1", "my-key")
	if err != nil {
		t.Fatalf("KvGet: %v", err)
	}
	if !bytes.Equal(got, []byte("hello-value")) {
		t.Fatalf("stdout = %q", got)
	}
	want := append([]string{"kv", "get", "ns-1", "my-key"}, fleetNoJSON()...)
	assertArgv(t, argsLog, want)
	for _, a := range readArgv(t, argsLog) {
		if a == "--json" {
			t.Fatal("kv get must omit --json")
		}
	}
}

func TestKvPutArgvInline(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	ttl := int64(60)
	err := m.KvPut(context.Background(), "demo", "v1", projectDir, "ns-1", "k", KvPutInput{
		Value:    []byte("hello"),
		TTL:      &ttl,
		Metadata: `{"seeded":true}`,
	})
	if err != nil {
		t.Fatalf("KvPut: %v", err)
	}
	want := append([]string{"kv", "put", "ns-1", "k", "hello", "--ttl", "60", "--metadata", `{"seeded":true}`}, fleetNoJSON()...)
	assertArgv(t, argsLog, want)
}

func TestKvPutArgvPath(t *testing.T) {
	argsLog, valueLog := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	raw, err := base64.StdEncoding.DecodeString("aGVsbG8=")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.KvPut(context.Background(), "demo", "v1", projectDir, "ns-1", "k", KvPutInput{
		Value:  raw,
		Binary: true,
	}); err != nil {
		t.Fatalf("KvPut: %v", err)
	}
	got := readArgv(t, argsLog)
	wantHead := []string{"kv", "put", "ns-1", "k", "--path"}
	if len(got) < 6 {
		t.Fatalf("argv too short: %q", got)
	}
	for i, w := range wantHead {
		if got[i] != w {
			t.Fatalf("argv[%d]=%q want %q full=%q", i, got[i], w, got)
		}
	}
	body, err := os.ReadFile(valueLog)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, raw) {
		t.Fatalf("path file = %q want %q", body, raw)
	}
	wantTail := fleetNoJSON()
	gotTail := got[6:]
	if len(gotTail) != len(wantTail) {
		t.Fatalf("fleet argv = %q want %q", gotTail, wantTail)
	}
	for i := range wantTail {
		if gotTail[i] != wantTail[i] {
			t.Fatalf("fleet argv[%d]=%q want %q", i, gotTail[i], wantTail[i])
		}
	}
}

func TestKvDeleteArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.KvDelete(context.Background(), "demo", "v1", projectDir, "ns-1", "my-key"); err != nil {
		t.Fatalf("KvDelete: %v", err)
	}
	want := append([]string{"kv", "delete", "ns-1", "my-key"}, fleetNoJSON()...)
	assertArgv(t, argsLog, want)
	for _, a := range readArgv(t, argsLog) {
		if a == "--json" {
			t.Fatal("kv delete must omit --json")
		}
	}
}

func TestKvInfoArgv(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	info, err := m.KvInfo(context.Background(), "demo", "v1", projectDir, "ns-1")
	if err != nil {
		t.Fatalf("KvInfo: %v", err)
	}
	if info.Keys != 3 || info.Bytes != 12 || info.Stored != 3 {
		t.Fatalf("info = %+v", info)
	}
	want := append([]string{"kv", "info", "ns-1"}, fleetJSON()...)
	assertArgv(t, argsLog, want)
}

func TestKvSkipsUnknownNamespace(t *testing.T) {
	argsLog, _ := installFakeOperatorCelld(t)
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if HasKVNamespace(projectDir, "ns-nope") {
		t.Fatal("HasKVNamespace(ns-nope) = true")
	}
	_, err := m.KvList(context.Background(), "demo", "v1", projectDir, "ns-nope", "", "", 0)
	if !errors.Is(err, ErrKVNamespaceNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("celld should not run for unknown namespace, stat=%v", err)
	}
}

func TestKvQueueUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	projectDir := filepath.Join(t.TempDir(), "project")
	writeOperatorWrangler(t, projectDir)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	_, err := m.KvInfo(context.Background(), "demo", "v1", projectDir, "ns-1")
	if !errors.Is(err, ErrCelldUnavailable) {
		t.Fatalf("KvInfo err = %v", err)
	}
	_, err = m.QueueInfo(context.Background(), "demo", "v1", projectDir, "tasks")
	if !errors.Is(err, ErrCelldUnavailable) {
		t.Fatalf("QueueInfo err = %v", err)
	}
}
