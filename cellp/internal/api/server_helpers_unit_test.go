package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestParsePageLimitCaps(t *testing.T) {
	r := httptest.NewRequest("GET", "/?limit=99999", nil)
	if got := parsePageLimit(r); got != registry.MaxPageLimit {
		t.Fatalf("got %d", got)
	}
	r = httptest.NewRequest("GET", "/", nil)
	if got := parsePageLimit(r); got != registry.DefaultPageLimit {
		t.Fatalf("default %d", got)
	}
}

func TestParseSinceOnServer(t *testing.T) {
	r := httptest.NewRequest("GET", "/?since=2026-01-02T03:04:05Z", nil)
	ts, err := parseSince(r)
	if err != nil || ts == nil {
		t.Fatalf("err=%v ts=%v", err, ts)
	}
	r = httptest.NewRequest("GET", "/?since=bad", nil)
	if _, err := parseSince(r); err == nil {
		t.Fatal("expected invalid_since")
	}
}

func TestProdURL(t *testing.T) {
	got := prodURL("http://gw.example/", "demo")
	want := "http://gw.example/demo/"
	if got != want {
		t.Fatalf("%q", got)
	}
}

func TestParseSinceNano(t *testing.T) {
	r := httptest.NewRequest("GET", "/?since=2026-01-02T03:04:05.123456789Z", nil)
	ts, err := parseSince(r)
	if err != nil || ts == nil || ts.UTC().Year() != 2026 {
		t.Fatalf("err=%v ts=%v", err, ts)
	}
	_ = time.Now()
}
