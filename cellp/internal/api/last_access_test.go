package api

import (
	"net/http"
	"testing"
)

func TestShouldCountVersionAccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		suffix string
		status int
		want   bool
	}{
		{suffix: "", status: 200, want: false},
		{suffix: "/", status: 200, want: false},
		{suffix: "/promote", status: 200, want: false},
		{suffix: "/archive", status: 200, want: false},
		{suffix: "/wake", status: 200, want: false},
		{suffix: "/pin", status: 200, want: false},
		{suffix: "/unpin", status: 200, want: false},
		{suffix: "/kv/ns/keys/k", status: 200, want: true},
		{suffix: "/queues/tasks/peek", status: 200, want: true},
		{suffix: "/database/query", status: 200, want: true},
		{suffix: "/kv/ns/keys/k", status: 503, want: false},
		{suffix: "/kv/ns/keys/k", status: 404, want: false},
	}
	for _, tc := range cases {
		got := shouldCountVersionAccess(tc.suffix, tc.status)
		if got != tc.want {
			t.Errorf("shouldCountVersionAccess(%q, %d) = %v, want %v", tc.suffix, tc.status, got, tc.want)
		}
	}
}

func TestShouldCountVersionAccessOKConst(t *testing.T) {
	if http.StatusServiceUnavailable != 503 {
		t.Fatal("expected 503 mapping")
	}
}
