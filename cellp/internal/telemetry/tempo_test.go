package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTempoBackendSearchSlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("minDuration"); got != "500ms" {
			t.Errorf("minDuration = %q, want 500ms", got)
		}
		_ = json.NewEncoder(w).Encode(tempoSearchPayload{
			Traces: []struct {
				TraceID         string `json:"traceID"`
				RootServiceName string `json:"rootServiceName"`
				RootTraceName   string `json:"rootTraceName"`
				StartTimeUnix   int64  `json:"startTimeUnix"`
			}{{TraceID: "abc123", RootServiceName: "cellp-gateway", RootTraceName: "ingress", StartTimeUnix: time.Now().Unix()}},
		})
	}))
	defer srv.Close()

	b := NewTempoBackend(srv.URL, "", nil, nil)
	now := time.Now().UTC()
	hits, err := b.Search(context.Background(), "demo", "v1", SearchRequest{
		Template: TemplateSlow,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
		Limit:    10,
		SlowMs:   intPtr(500),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].TraceID != "abc123" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestTempoBackendSearchStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tags := r.URL.Query().Get("tags")
		if tags == "" || !strings.Contains(tags, "http.status_code=404") || !strings.Contains(tags, "resource.cellp.project=demo") {
			t.Errorf("tags = %q", tags)
		}
		_ = json.NewEncoder(w).Encode(tempoSearchPayload{})
	}))
	defer srv.Close()

	status := 404
	b := NewTempoBackend(srv.URL, "", nil, nil)
	now := time.Now().UTC()
	_, err := b.Search(context.Background(), "demo", "v1", SearchRequest{
		Template: TemplateStatus,
		Start:    now.Add(-time.Hour),
		End:      now.Add(time.Hour),
		Status:   &status,
	})
	if err != nil {
		t.Fatal(err)
	}
}
