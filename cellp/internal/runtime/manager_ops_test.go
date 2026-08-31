package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func installFakeCelld(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
  diagnose|deploy) exit 0 ;;
  d1) exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return bin
}

func TestDeployWithFakeCelld(t *testing.T) {
	installFakeCelld(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.Deploy(context.Background(), "demo", "v1", dir, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Deploy(context.Background(), "demo", "v1", dir, false); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnoseWithFakeCelld(t *testing.T) {
	installFakeCelld(t)
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.Diagnose(context.Background(), "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestHealthAndRuntimeHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	if !m.Health(context.Background(), "127.0.0.1", port) {
		t.Fatal("expected healthy upstream")
	}
	rh := m.RuntimeHealth(context.Background(), []registry.Route{{
		ProjectID: "demo", VersionID: "v1", UpstreamHost: "127.0.0.1", UpstreamPort: port,
	}})
	if len(rh) != 1 || !rh[0].Healthy {
		t.Fatalf("rh=%+v", rh)
	}
}

func TestAllocateAndSeedPort(t *testing.T) {
	m := New(8800, "", "us-east-1", "s3://cellp-celld", "k", "s")
	p1 := m.AllocatePort("demo", "v1")
	p2 := m.AllocatePort("demo", "v2")
	if p1 == p2 {
		t.Fatalf("ports %d %d", p1, p2)
	}
	if err := m.SeedPort("demo", "v1", p1); err != nil {
		t.Fatal(err)
	}
	if m.AllocatePort("demo", "v1") != p1 {
		t.Fatal("seed should reuse port")
	}
}

func TestD1ExecuteWithFakeCelld(t *testing.T) {
	installFakeCelld(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{"d1_databases":[{"database_name":"guestbook"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(dir, "seed.sql")
	if err := os.WriteFile(seed, []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.D1Execute(context.Background(), "demo", "v1", dir, seed); err != nil {
		t.Fatal(err)
	}
}

func TestStopNoProcess(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	if err := m.Stop(context.Background(), "demo", "v-none"); err != nil {
		t.Fatal(err)
	}
}
