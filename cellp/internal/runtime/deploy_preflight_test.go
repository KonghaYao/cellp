package runtime

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeCelldDiagnoseFailDeployOK(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = diagnose ]; then
  echo "fail peer node_dead: simulated fleet failure" >&2
  exit 1
fi
if [ "$1" = deploy ]; then
  exit 0
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func celldHealthServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/celld/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

func TestDeployFullFleetDiagnoseFailClosed(t *testing.T) {
	installFakeCelldDiagnoseFailDeployOK(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	err := m.Deploy(context.Background(), "demo", "v1", dir, true)
	if err == nil || !strings.Contains(err.Error(), "celld diagnose") {
		t.Fatalf("expected diagnose failure, got %v", err)
	}
}

func TestDeployCronReconcileSkipsFleetDiagnose(t *testing.T) {
	installFakeCelldDiagnoseFailDeployOK(t)
	srv := celldHealthServer(t)
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	if err := m.DeployForCronReconcile(context.Background(), "demo", "v1", dir, false, host, port); err != nil {
		t.Fatal(err)
	}
}

func TestDeployCronReconcileFailsWhenConfiguredUpstreamUnhealthy(t *testing.T) {
	installFakeCelldDiagnoseFailDeployOK(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(`{"name":"counter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	err := m.DeployForCronReconcile(context.Background(), "demo", "v1", dir, false, "127.0.0.1", 1)
	if err == nil || !strings.Contains(err.Error(), "unreachable or unhealthy") {
		t.Fatalf("expected upstream failure, got %v", err)
	}
}
