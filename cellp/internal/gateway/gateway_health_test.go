package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/gateway"
	"github.com/cellp/cellp/internal/metrics"
	"github.com/cellp/cellp/internal/registry"
)

func TestGatewayHealthDeep(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	store, err := registry.Open(t.TempDir() + "/gw-deep.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	host, port := splitHostPort(t, upstream.URL)
	ctx := t.Context()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})

	gw := gateway.New(store)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health/deep")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("status = %v", body["status"])
	}
}

func TestGatewayMetricsMiddleware(t *testing.T) {
	gw, store, cleanup := setupGateway(t)
	defer cleanup()
	_ = store

	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	metricsResp := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsResp, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResp.Body.String(), "cellp_gateway_requests_total") {
		t.Fatalf("missing gateway metrics: %s", metricsResp.Body.String())
	}
}

func splitHostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	portStr := u.Port()
	if portStr == "" {
		if u.Scheme == "https" {
			portStr = "443"
		} else {
			portStr = "80"
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), port
}
