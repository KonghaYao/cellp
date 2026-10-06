package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/telemetry"
)

func TestGatewayTraceparentPropagation(t *testing.T) {
	var seenTraceparent string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	host, portStr, _ := strings.Cut(strings.TrimPrefix(upstream.URL, "http://"), ":")

	store, err := registry.Open(t.TempDir() + "/tp.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	port, _ := strconv.Atoi(portStr)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: host, UpstreamPort: port,
	})
	hostHeader := "v1.demo.ingress.local"
	upsertPreviewBinding(t, store, "demo", "v1", hostHeader, "syn.v1.demo.ingress.local")

	gw := hostOnlyGW(store)
	t.Setenv("CELLP_OTEL_BACKEND", "memory")
	tel := telemetry.NewFromConfig(config.LoadOtelConfig())
	gw.SetTelemetry(tel)
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/hello", nil)
	req.Host = hostHeader
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	if seenTraceparent == "" {
		t.Fatal("upstream missing traceparent")
	}
	if !strings.HasPrefix(seenTraceparent, "00-") {
		t.Fatalf("traceparent = %q", seenTraceparent)
	}
}
