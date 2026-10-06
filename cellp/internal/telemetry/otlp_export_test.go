package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOTLPExporterExportIngress(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			http.NotFound(w, r)
			return
		}
		posts.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("invalid json: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL)
	rec := SpanRecord{
		TraceID: "abc", SpanID: "def", Name: "ingress", ServiceName: "cellp-gateway",
		Project: "demo", Version: "v1", Environment: "preview",
		Method: "GET", URL: "/hello", StatusCode: 200, OK: true,
		StartTime: time.Now().UTC(), Duration: time.Millisecond,
	}
	exp.ExportIngress(context.Background(), rec, TraceContext{})
	if posts.Load() != 1 {
		t.Fatalf("posts = %d, want 1", posts.Load())
	}
}

func TestShouldExportGateway(t *testing.T) {
	if !shouldExportGateway("lgtm") || !shouldExportGateway("jaeger") {
		t.Fatal("expected export for lgtm/jaeger")
	}
	if shouldExportGateway("memory") || shouldExportGateway("none") {
		t.Fatal("expected no export for memory/none")
	}
}
