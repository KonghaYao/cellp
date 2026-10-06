package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/telemetry"
)

func TestTelemetryContextAndSearch(t *testing.T) {
	t.Setenv("CELLP_OTEL_BACKEND", "memory")
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/telemetry/context", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("context status = %d %s", w.Code, w.Body.String())
	}

	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]interface{}{
		"template": "slow",
		"start":    now.Add(-time.Hour).Format(time.RFC3339Nano),
		"end":      now.Add(time.Hour).Format(time.RFC3339Nano),
		"limit":    10,
		"slow_ms":  100,
	})
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/telemetry/search", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search status = %d %s", w.Code, w.Body.String())
	}
}

func TestTelemetryDeployTokenForbidden(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy-only", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/telemetry/context", nil)
	req.Header.Set("Authorization", "Bearer deploy-only")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestTelemetrySearchInvalidWindow(t *testing.T) {
	srv, store, _ := testAPI(t, "d", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	body, _ := json.Marshal(telemetry.SearchRequest{Template: telemetry.TemplateError})
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/telemetry/search", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}
