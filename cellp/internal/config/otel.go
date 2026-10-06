package config

import (
	"strings"
	"time"
)

// OtelConfig holds AD-14 backend selection (CELLP_OTEL_*).
type OtelConfig struct {
	Backend         string
	CollectorURL    string
	JaegerQueryURL  string
	TempoQueryURL   string
	LokiQueryURL    string
	GrafanaURL      string
	IngestPort      int
	MemoryMaxSpans  int
	MemoryMaxLogs   int
	MemoryMaxAge    time.Duration
	ExportGateway   bool
}

func LoadOtelConfig() OtelConfig {
	cfg := OtelConfig{
		Backend:        envOr("CELLP_OTEL_BACKEND", "none"),
		CollectorURL:   envOr("CELLP_OTEL_COLLECTOR", envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")),
		JaegerQueryURL: envOr("CELLP_OTEL_JAEGER_QUERY", "http://127.0.0.1:16686"),
		TempoQueryURL:  envOr("CELLP_OTEL_TEMPO_QUERY", "http://127.0.0.1:3200"),
		LokiQueryURL:   envOr("CELLP_OTEL_LOKI_QUERY", "http://127.0.0.1:3100"),
		GrafanaURL:     envOr("CELLP_OTEL_GRAFANA", "http://127.0.0.1:3000"),
		IngestPort:     envInt("CELLP_OTEL_INGEST_PORT", 4318),
		MemoryMaxSpans: envInt("CELLP_OTEL_MEMORY_MAX_SPANS", 50000),
		MemoryMaxLogs:  envInt("CELLP_OTEL_MEMORY_MAX_LOGS", 100000),
		MemoryMaxAge:   time.Duration(envInt("CELLP_OTEL_MEMORY_MAX_AGE_H", 24)) * time.Hour,
		ExportGateway:  envOr("CELLP_OTEL_GATEWAY_EXPORT", "1") != "0",
	}
	return cfg
}

// CelldOtelEnv returns env vars to inject into per-version celld when OTEL is enabled.
func (c OtelConfig) CelldOtelEnv(project, version, environment string) []string {
	if strings.EqualFold(c.Backend, "none") {
		return nil
	}
	out := []string{"CELLD_OTEL=" + strings.TrimRight(c.CollectorURL, "/")}
	out = append(out, "OTEL_SERVICE_NAME=celld")
	out = append(out, "CELLD_OTEL_FLUSH_MS=5000")
	if environment != "" {
		out = append(out, "OTEL_RESOURCE_ATTRIBUTES=deployment.environment="+environment)
	}
	_ = project
	_ = version
	return out
}

// OtelEnabled reports whether any telemetry backend is active.
func (c OtelConfig) OtelEnabled() bool {
	b := strings.ToLower(strings.TrimSpace(c.Backend))
	return b != "" && b != "none"
}
