package api

import (
	"net/http"
	"testing"
)

func TestShouldCountVersionAccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method string
		suffix string
		status int
		want   bool
	}{
		{method: http.MethodGet, suffix: "", status: 200, want: false},
		{method: http.MethodGet, suffix: "/", status: 200, want: false},
		{method: http.MethodGet, suffix: "/promote", status: 200, want: false},
		{method: http.MethodGet, suffix: "/archive", status: 200, want: false},
		{method: http.MethodGet, suffix: "/wake", status: 200, want: false},
		{method: http.MethodGet, suffix: "/pin", status: 200, want: false},
		{method: http.MethodGet, suffix: "/unpin", status: 200, want: false},
		{method: http.MethodGet, suffix: "/env", status: 200, want: false},
		{method: http.MethodPut, suffix: "/env", status: 200, want: true},
		{method: http.MethodGet, suffix: "/kv/ns/keys/k", status: 200, want: true},
		{method: http.MethodGet, suffix: "/queues/tasks/peek", status: 200, want: true},
		{method: http.MethodGet, suffix: "/database/query", status: 200, want: true},
		{method: http.MethodGet, suffix: "/kv/ns/keys/k", status: 503, want: false},
		{method: http.MethodGet, suffix: "/kv/ns/keys/k", status: 404, want: false},
	}
	for _, tc := range cases {
		got := shouldCountVersionAccess(tc.method, tc.suffix, tc.status)
		if got != tc.want {
			t.Errorf("shouldCountVersionAccess(%q, %q, %d) = %v, want %v", tc.method, tc.suffix, tc.status, got, tc.want)
		}
	}
}

func TestShouldCountVersionAccessOKConst(t *testing.T) {
	if http.StatusServiceUnavailable != 503 {
		t.Fatal("expected 503 mapping")
	}
}
