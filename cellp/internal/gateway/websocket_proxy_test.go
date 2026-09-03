package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func setupProxyGW(t *testing.T, upstreamURL string) (srvURL, previewHost string, cleanup func()) {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/gw-ws.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	host, port := upstreamHostPort(t, upstreamURL)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})
	previewHost = "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", previewHost, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	cleanup = func() {
		srv.Close()
		store.Close()
	}
	return srv.URL, previewHost, cleanup
}

func wsUpgradeRequest(t *testing.T, url, host string, extra func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Protocol", "chat")
	if extra != nil {
		extra(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestProxyIngressWebSocketUpstream101(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-WebSocket-Key") == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	resp := wsUpgradeRequest(t, srvURL+"/", previewHost, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%q, want 101 (must not hit ErrorHandler 502)", resp.StatusCode, body)
	}
}

func TestProxyIngressWebSocketUpstream426(t *testing.T) {
	const wantBody = "websocket upgrade required"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, wantBody, http.StatusUpgradeRequired)
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	resp := wsUpgradeRequest(t, srvURL+"/", previewHost, nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("status=%d body=%q, want 426 passthrough", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), wantBody) {
		t.Fatalf("body=%q, want substring %q", body, wantBody)
	}
}

func TestProxyIngressWebSocketDialFailure502(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/gw-ws-dial.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 1,
	})
	previewHost := "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", previewHost, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp := wsUpgradeRequest(t, srv.URL+"/", previewHost, nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502", resp.StatusCode)
	}
	if !strings.Contains(string(body), "bad gateway") {
		t.Fatalf("body=%q, want bad gateway", body)
	}
}

func TestProxyIngressPreservesSecWebSocketHeaders(t *testing.T) {
	var gotKey, gotVersion, gotProto string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Sec-WebSocket-Key")
		gotVersion = r.Header.Get("Sec-WebSocket-Version")
		gotProto = r.Header.Get("Sec-WebSocket-Protocol")
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	resp := wsUpgradeRequest(t, srvURL+"/", previewHost, nil)
	resp.Body.Close()
	if gotKey != "dGhlIHNhbXBsZSBub25jZQ==" {
		t.Fatalf("Sec-WebSocket-Key=%q", gotKey)
	}
	if gotVersion != "13" {
		t.Fatalf("Sec-WebSocket-Version=%q", gotVersion)
	}
	if gotProto != "chat" {
		t.Fatalf("Sec-WebSocket-Protocol=%q", gotProto)
	}
	_ = resp
}

func TestProxyIngressNonUpgradeGETUnchanged(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain-http"))
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	resp := doHostGet(t, srvURL, previewHost, "/")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "plain-http" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestProxyIngressUpgradeKeepsConnectionHeader(t *testing.T) {
	var connHdr string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connHdr = r.Header.Get("Connection")
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	resp := wsUpgradeRequest(t, srvURL+"/", previewHost, nil)
	resp.Body.Close()
	if !strings.Contains(strings.ToLower(connHdr), "upgrade") {
		t.Fatalf("upstream Connection=%q, want upgrade token", connHdr)
	}
}

// TestReverseProxyHijackStub ensures a hijackable client conn works through the full handler stack.
func TestReverseProxyHijackStub(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = bufrw.Flush()
	}))
	defer upstream.Close()

	srvURL, previewHost, cleanup := setupProxyGW(t, upstream.URL)
	defer cleanup()

	req, err := http.NewRequest(http.MethodGet, srvURL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = previewHost
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status=%d, want 101 via Hijack path", resp.StatusCode)
	}
}
