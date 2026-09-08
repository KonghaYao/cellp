package transport

import (
	"errors"
	"net"
	"syscall"
	"testing"
)

func TestNormalizeServeErrECONNRESETWhenNotStopping(t *testing.T) {
	err := &net.OpError{Op: "accept", Err: syscall.ECONNRESET}
	norm := normalizeServeErr(err, false)
	if norm == nil {
		t.Fatal("ECONNRESET on accept must propagate when not shutting down")
	}
	if !errors.Is(norm, syscall.ECONNRESET) {
		t.Fatalf("expected ECONNRESET, got %v", norm)
	}
}

func TestNormalizeServeErrECONNRESETWhenStopping(t *testing.T) {
	err := &net.OpError{Op: "accept", Err: syscall.ECONNRESET}
	if norm := normalizeServeErr(err, true); norm != nil {
		t.Fatalf("expected nil during shutdown accept teardown, got %v", norm)
	}
}

func TestNormalizeServeErrServerClosedWhenStopping(t *testing.T) {
	if norm := normalizeServeErr(net.ErrClosed, true); norm != nil {
		t.Fatalf("expected nil, got %v", norm)
	}
}

func TestNormalizeServeErrServerClosedWhenNotStopping(t *testing.T) {
	if norm := normalizeServeErr(net.ErrClosed, false); norm == nil {
		t.Fatal("net.ErrClosed must propagate when not shutting down")
	}
}
