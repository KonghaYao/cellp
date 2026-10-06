package telemetry

import (
	"sync/atomic"
	"time"
)

// Service coordinates recording and query for cellpd (AD-14).
type Service struct {
	Backend Backend
	store   *MemoryStore
	enabled bool
	name    string
}

func NewService(backend Backend, store *MemoryStore, enabled bool, name string) *Service {
	return &Service{Backend: backend, store: store, enabled: enabled, name: name}
}

func (s *Service) Enabled() bool { return s.enabled }

func (s *Service) BackendName() string { return s.name }

func (s *Service) ShedCount() int64 {
	if s.store == nil {
		return 0
	}
	return s.store.ShedCount()
}

// RecordIngress stores a gateway ingress span when telemetry is enabled.
func (s *Service) RecordIngress(project, version, env, method, url string, status int, dur time.Duration, parent, span TraceContext) {
	if !s.enabled || s.store == nil {
		return
	}
	rec := SpanRecord{
		TraceID:     HexTraceID(span.TraceID),
		SpanID:      HexSpanID(span.SpanID),
		Name:        "ingress",
		ServiceName: "cellp-gateway",
		Project:     project,
		Version:     version,
		Environment: env,
		Method:      method,
		URL:         url,
		StatusCode:  status,
		Duration:    dur,
		StartTime:   time.Now().UTC().Add(-dur),
		OK:          status >= 200 && status < 500,
	}
	if parent.SpanID != span.SpanID {
		rec.ParentSpanID = HexSpanID(parent.SpanID)
	}
	s.store.IngestSpan(rec)
}

var shedCounter atomic.Int64

func IncShed() { shedCounter.Add(1) }
