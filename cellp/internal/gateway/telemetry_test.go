package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusCapturingWriterDelegatesHijack(t *testing.T) {
	t.Parallel()
	inner := &hijackRecorder{ResponseWriter: httptest.NewRecorder()}
	cw := &statusCapturingWriter{ResponseWriter: inner, status: http.StatusOK}
	var _ http.Hijacker = (*statusCapturingWriter)(nil)
	_, _, err := cw.Hijack()
	if err != http.ErrNotSupported {
		t.Fatalf("Hijack err = %v, want ErrNotSupported from recorder backend", err)
	}
}

func TestStatusCapturingWriterHijackWithRealConn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
		if !isUpgradeRequest(r) {
			http.Error(cw, "not upgrade", http.StatusBadRequest)
		 return
		}
		conn, bufrw, err := cw.Hijack()
		if err != nil {
			http.Error(cw, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		_, _ = bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_ = bufrw.Flush()
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
