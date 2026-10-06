package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestMemoryBackendSearchSlow(t *testing.T) {
	store := NewMemoryStore(100, 100, time.Hour)
	b := NewMemoryBackend(store)
	now := time.Now().UTC()
	store.IngestSpan(SpanRecord{
		TraceID: "abc", SpanID: "1", Name: "ingress", Project: "demo", Version: "v1",
		Duration: 2 * time.Second, StartTime: now, OK: true, StatusCode: 200,
	})
	store.IngestSpan(SpanRecord{
		TraceID: "def", SpanID: "2", Name: "ingress", Project: "demo", Version: "v1",
		Duration: 10 * time.Millisecond, StartTime: now, OK: true, StatusCode: 200,
	})
	hits, err := b.Search(context.Background(), "demo", "v1", SearchRequest{
		Template: TemplateSlow,
		Start:    now.Add(-time.Minute),
		End:      now.Add(time.Minute),
		Limit:    10,
		SlowMs:   intPtr(500),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].TraceID != "abc" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestMemoryBackendGetTrace(t *testing.T) {
	store := NewMemoryStore(100, 100, time.Hour)
	b := NewMemoryBackend(store)
	now := time.Now().UTC()
	store.IngestSpan(SpanRecord{TraceID: "t1", SpanID: "s1", Name: "ingress", Project: "p", Version: "v", StartTime: now})
	store.IngestLog(LogRecord{TraceID: "t1", Body: "hello", Project: "p", Version: "v", Time: now})
	tr, err := b.GetTrace(context.Background(), "p", "v", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Tree.Name != "ingress" || len(tr.Logs) != 1 {
		t.Fatalf("trace = %+v", tr)
	}
}

func intPtr(v int) *int { return &v }
