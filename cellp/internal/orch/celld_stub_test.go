package orch

import (
	"os"
	"path/filepath"
	"testing"
)

// installFakeCelld puts a celld shim on PATH: CLI subcommands exit 0; daemon mode
// (--listen) serves /.well-known/celld/health so runDeploy Start/Health can pass.
func installFakeCelld(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
deploy|diagnose|d1|kv|r2|queue|cron|workflow|cell) exit 0 ;;
esac
listen=""
while [ $# -gt 0 ]; do
  case "$1" in
  --listen) listen="$2"; shift 2 ;;
  *) shift ;;
  esac
done
[ -z "$listen" ] && exit 0
host="${listen%%:*}"
port="${listen#*:}"
exec /usr/bin/python3 -u -c '
import http.server, socketserver, sys
host, port = sys.argv[1], int(sys.argv[2])
class H(http.server.BaseHTTPRequestHandler):
	def do_GET(self):
		self.send_response(200)
		if self.path == "/.well-known/celld/health":
			self.send_header("Content-Type", "application/json")
			self.end_headers()
			self.wfile.write(b"{\"ok\":true}")
		else:
			self.end_headers()
	def log_message(self, *args):
		pass
with socketserver.TCPServer((host, port), H) as httpd:
	httpd.serve_forever()
' "$host" "$port"
`
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}
