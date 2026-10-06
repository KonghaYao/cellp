package telemetry

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/cellp/cellp/internal/config"
)

// Service coordinates recording and query for cellpd (AD-14).
type Service struct {
	Backend Backend
	store   *MemoryStore
	enabled bool
	name    string
	exporter *OTLPExporter
}

func NewService(backend Backend, store *MemoryStore, enabled bool, name string, cfg config.OtelConfig) *Service {
	var exporter *OTLPExporter
	if cfg.ExportGateway && shouldExportGateway(cfg.Backend) {
		exporter = NewOTLPExporter(cfg.CollectorURL)
	}
	return &Service{Backend: backend, store: store, enabled: enabled, name: name, exporter: exporter}
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
	if s.exporter != nil {
		go s.exporter.ExportIngress(context.Background(), rec, parent)
	}
}

var shedCounter atomic.Int64

func IncShed() { shedCounter.Add(1) }
