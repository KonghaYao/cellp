package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

// TestIngressSSEFlushesFirstChunk 保证 ReverseProxy FlushInterval=-1 +
// metrics statusRecorder.Flush 会立刻把 text/event-stream 首块刷到客户端，
// 而不会等上游结束（A03 GET /event 验收：3s 内须有字节）。
func TestIngressSSEFlushesFirstChunk(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flush", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "event: message\ndata: {\"type\":\"server.connected\",\"properties\":{}}\n\n")
		fl.Flush()
		time.Sleep(2 * time.Second)
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw-sse.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetProdVersion(ctx, "demo", "v1")

	host, port := upstreamHostPort(t, upstream.URL)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})
	prodHost := "demo.ingress.local"
	upsertProdBinding(t, store, "demo", prodHost, "syn.demo.ingress.local")

	gw := hostOnlyGW(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/event", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = prodHost
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}

	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, err := resp.Body.Read(buf)
		if n > 0 {
			got <- string(buf[:n])
			return
		}
		if err != nil {
			got <- "err:" + err.Error()
		}
	}()

	select {
	case chunk := <-got:
		if !strings.Contains(chunk, "server.connected") {
			t.Fatalf("first chunk = %q, want server.connected", chunk)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SSE first chunk not flushed within 500ms (proxy buffering)")
	}
}
