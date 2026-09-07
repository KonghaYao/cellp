package activator_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
)

func TestBufferBoundedBodyDetectsLowReportedLength(t *testing.T) {
	max := int64(8)
	body := []byte("0123456789")
	req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(bytes.NewReader(body)))
	req.ContentLength = 4
	_, cancel, err := activator.BufferBoundedBody(req, max)
	cancel()
	if err != activator.ErrBodyTooLarge {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

func TestBufferBoundedBodyChunkedExactLimit(t *testing.T) {
	max := int64(4)
	req := httptest.NewRequest(http.MethodPut, "/", io.NopCloser(strings.NewReader("abcd")))
	req.ContentLength = -1
	reserved, cancel, err := activator.BufferBoundedBody(req, max)
	cancel()
	if err != nil || reserved != max {
		t.Fatalf("reserved=%d err=%v", reserved, err)
	}
	reread, _ := io.ReadAll(req.Body)
	if !bytes.Equal(reread, []byte("abcd")) {
		t.Fatalf("body not replayable: %q", reread)
	}
}

func TestFlightLimiterPerVersionAndGlobal(t *testing.T) {
	lim := activator.NewFlightLimiter(1, 1)
	if !lim.TryAcquire("p", "v1") {
		t.Fatal("first flight")
	}
	if lim.TryAcquire("p", "v2") {
		t.Fatal("global flight cap")
	}
	lim.Release("p", "v1")
	if !lim.TryAcquire("p", "v2") {
		t.Fatal("after release")
	}
}

func TestFastFailFlightAdmissionExhausted(t *testing.T) {
	cfg := activator.DefaultConfig()
	cfg.GlobalActivationFlights = 1
	cfg.PerVersionActivationFlights = 1
	cfg.WakeTimeout = time.Second
	block := &blockingEnsure{started: make(chan struct{}), release: make(chan struct{})}
	a := activator.New(true, block, cfg)
	reqBlock := httptest.NewRequest(http.MethodPost, "/", nil)
	reqBlock.ContentLength = cfg.MaxBufferedBodyBytes + 1
	go func() {
		_ = a.Admit(context.Background(), reqBlock, "p", "v1", contract.StatusDeployReady, 1,
			func() (string, bool) { return "", false })
	}()
	<-block.started

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = cfg.MaxBufferedBodyBytes + 1
	res := a.Admit(context.Background(), req, "p", "v2", contract.StatusDeployReady, 1, func() (string, bool) { return "", false })
	if res.Reason != activator.ReasonWakeQueueFull {
		t.Fatalf("reason %q", res.Reason)
	}
	close(block.release)
}

func TestParallelDesiredGenerationFlightsBounded(t *testing.T) {
	fe := &blockingEnsure{started: make(chan struct{}), release: make(chan struct{})}
	cfg := activator.DefaultConfig()
	cfg.GlobalActivationFlights = 1
	cfg.PerVersionActivationFlights = 1
	cfg.WakeTimeout = time.Second
	a := activator.New(true, fe, cfg)
	go func() {
		_ = a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 1,
			func() (string, bool) { return "", false })
	}()
	<-fe.started
	res := a.Admit(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), "p", "v", contract.StatusDeployReady, 2,
		func() (string, bool) { return "", false })
	if res.Reason != activator.ReasonWakeQueueFull {
		t.Fatalf("parallel generation flight should be bounded: %q", res.Reason)
	}
	close(fe.release)
}
