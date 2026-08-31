package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func TestQueueMaxEnv(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "42")
	if got := QueueMax(); got != 42 {
		t.Fatalf("got %d", got)
	}
	t.Setenv("CELLP_QUEUE_MAX", "nope")
	if got := QueueMax(); got != defaultQueueMax {
		t.Fatalf("invalid env got %d", got)
	}
}

func TestParseOperatorLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=50", nil)
	n, ok := parseOperatorLimit(req, 10)
	if !ok || n != 50 {
		t.Fatalf("n=%d ok=%v", n, ok)
	}
	req = httptest.NewRequest(http.MethodGet, "/?limit=0", nil)
	if _, ok := parseOperatorLimit(req, 10); ok {
		t.Fatal("expected invalid")
	}
}

func TestParsePageLimitCap(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=9999", nil)
	if got := parsePageLimit(req); got != registry.MaxPageLimit {
		t.Fatalf("limit=%d", got)
	}
}

func TestParseSinceRFC3339(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?since=2026-01-02T15:04:05Z", nil)
	tm, err := parseSince(req)
	if err != nil || tm == nil {
		t.Fatalf("err=%v tm=%v", err, tm)
	}
}
