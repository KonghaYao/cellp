package telemetry

import (
	"strings"

	"github.com/cellp/cellp/internal/config"
)

// NewFromConfig builds the telemetry service for cellpd.
func NewFromConfig(cfg config.OtelConfig) *Service {
	store := NewMemoryStore(cfg.MemoryMaxSpans, cfg.MemoryMaxLogs, cfg.MemoryMaxAge)
	mem := NewMemoryBackend(store)
	var backend Backend = NoneBackend{}
	name := cfg.Backend
	enabled := false
	switch strings.ToLower(cfg.Backend) {
	case "memory":
		backend = mem
		enabled = true
	case "jaeger":
		backend = NewJaegerBackend(cfg.JaegerQueryURL, cfg.GrafanaURL, mem)
		enabled = true
	case "lgtm", "lgtm-prod":
		backend = NewTempoBackend(cfg.TempoQueryURL, cfg.GrafanaURL, mem, NewLokiClient(cfg.LokiQueryURL))
		enabled = true
		name = "lgtm"
	case "otlp-file":
		backend = mem
		enabled = true
		name = "otlp-file"
	default:
		name = "none"
	}
	return NewService(backend, store, enabled, name, cfg)
}

// ValidateSearchRequest enforces frozen search contract.
func ValidateSearchRequest(req SearchRequest) error {
	if req.Start.IsZero() || req.End.IsZero() || !req.End.After(req.Start) {
		return ErrInvalidWindow
	}
	switch req.Template {
	case TemplateSlow, TemplateError, TemplateStatus, TemplateBody, TemplateRequestID:
	default:
		return ErrInvalidTemplate
	}
	if req.Template == TemplateBody && len(req.Body) > 256 {
		return ErrScanTooLarge
	}
	if req.Limit > MaxSearchLimit {
		return ErrScanTooLarge
	}
	return nil
}
