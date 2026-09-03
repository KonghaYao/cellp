package gateway

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestIsUpgradeRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		h    http.Header
		want bool
	}{
		{"minimal", http.Header{
			"Connection": []string{"Upgrade"},
			"Upgrade":    []string{"websocket"},
		}, true},
		{"keep-alive", http.Header{
			"Connection": []string{"keep-alive, Upgrade"},
			"Upgrade":    []string{"WebSocket"},
		}, true},
		{"no upgrade token", http.Header{
			"Connection": []string{"keep-alive"},
			"Upgrade":    []string{"websocket"},
		}, false},
		{"http upgrade", http.Header{
			"Connection": []string{"Upgrade"},
			"Upgrade":    []string{"h2c"},
		}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, _ := http.NewRequest(http.MethodGet, "http://example/", nil)
			r.Header = tc.h
			if got := isUpgradeRequest(r); got != tc.want {
				t.Fatalf("isUpgradeRequest() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClassifyProxyError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{nil, "other"},
		{errors.New("hijack not supported"), "hijack"},
		{&net.OpError{Op: "dial", Err: errors.New("refused")}, "dial"},
		{errors.New("dial tcp 127.0.0.1:1: connection refused"), "dial"},
		{errors.New("something else"), "other"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := classifyProxyError(tc.err); got != tc.want {
				t.Fatalf("classifyProxyError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
