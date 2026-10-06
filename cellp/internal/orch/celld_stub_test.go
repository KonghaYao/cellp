package orch

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func freeRuntimeBasePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if port <= 11 {
		t.Fatalf("unexpected ephemeral port %d", port)
	}
	return port - 11
}

const fakeCelldMain = `package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "deploy", "diagnose", "d1", "kv", "r2", "queue", "cron", "workflow", "cell":
			os.Exit(0)
		}
	}
	listen := ""
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--listen" && i+1 < len(os.Args) {
			listen = os.Args[i+1]
			break
		}
	}
	if listen == "" {
		os.Exit(0)
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/celld/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	_ = http.Serve(ln, mux)
}
`

// installFakeCelld puts a celld shim on PATH: CLI subcommands exit 0; daemon mode
// (--listen) serves /.well-known/celld/health so runDeploy Start/Health can pass.
func installFakeCelld(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(fakeCelldMain), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module fakecelld\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(bin, "celld")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = srcDir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake celld: %v: %s", err, b)
	}
	t.Setenv("PATH", bin)
}
