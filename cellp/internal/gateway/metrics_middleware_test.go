package gateway

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hijackRecorder struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hij, ok := h.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, rw, err := hij.Hijack()
	if err == nil {
		h.hijacked = true
	}
	return conn, rw, err
}

func TestMetricsMiddlewareDelegatesHijack(t *testing.T) {
	t.Parallel()
	inner := &hijackRecorder{ResponseWriter: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: inner}
	var _ http.Hijacker = (*statusRecorder)(nil)
	_, _, err := rec.Hijack()
	if err != http.ErrNotSupported {
		t.Fatalf("Hijack err = %v, want ErrNotSupported from recorder backend", err)
	}
}

func TestMetricsMiddlewareHijackWithRealConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUpgradeRequest(r) {
				http.Error(w, "not upgrade", http.StatusBadRequest)
				return
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer conn.Close()
			_, _ = bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			_ = bufrw.Flush()
		}))
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
}
