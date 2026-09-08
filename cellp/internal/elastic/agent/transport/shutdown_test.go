package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWaitShutdownReturnsContextError(t *testing.T) {
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	s := &Server{shutdownDone: done}
	if err := s.waitShutdown(ctx, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
}

func TestShutdownTimesOutWithBlockingHandler(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/block", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: mux}
	s := &Server{
		httpServer:   hs,
		listener:     ln,
		ready:        make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	go func() { _ = hs.Serve(ln) }()
	go func() {
		c := &http.Client{Timeout: 0}
		_, _ = c.Get("http://" + ln.Addr().String() + "/block")
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	shCtx, shCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shCancel()
	if err := s.Shutdown(shCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline, got %v", err)
	}
	close(block)
}

func TestConcurrentDoubleShutdownDoesNotPanic(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: mux}
	s := &Server{
		httpServer:   hs,
		listener:     ln,
		ready:        make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	go func() { _ = hs.Serve(ln) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	var firstErr, secondErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		firstErr = s.Shutdown(ctx)
	}()
	go func() {
		defer wg.Done()
		secondErr = s.Shutdown(ctx)
	}()
	wg.Wait()
	if firstErr != nil && secondErr != nil && !errors.Is(firstErr, secondErr) && firstErr.Error() != secondErr.Error() {
		t.Fatalf("inconsistent shutdown errors: %v / %v", firstErr, secondErr)
	}
}

func TestWaitShutdownSharedResultAfterCompletion(t *testing.T) {
	done := make(chan struct{})
	s := &Server{shutdownDone: done}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	if err := s.waitShutdown(shortCtx, done); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	close(done)
	longCtx, longCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer longCancel()
	if err := s.waitShutdown(longCtx, done); err != nil {
		t.Fatalf("expected nil shared shutdown err after completion, got %v", err)
	}
}

func TestNormalizeServeErrClosedListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	hs := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() { errCh <- hs.Serve(ln) }()
	deadline := time.Now().Add(3 * time.Second)
	d := net.Dialer{Timeout: 50 * time.Millisecond}
	for time.Now().Before(deadline) {
		c, dialErr := d.Dial("tcp", addr)
		if dialErr == nil {
			_ = c.Close()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = ln.Close()
	err = <-errCh
	if norm := normalizeServeErr(err, true); norm != nil {
		t.Fatalf("expected nil normalized serve err during shutdown, got %v", norm)
	}
	if norm := normalizeServeErr(err, false); norm == nil {
		t.Fatal("unexpected serve err must propagate when not shutting down")
	}
}

func TestNormalizeServeErrAcceptECONNRESETWhenNotStopping(t *testing.T) {
	err := &net.OpError{Op: "accept", Err: syscall.ECONNRESET}
	if norm := normalizeServeErr(err, false); norm == nil {
		t.Fatal("expected ECONNRESET to propagate")
	}
}
