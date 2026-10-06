package telemetry

import "time"

// SpanRecord is a normalized span for query backends (AD-14 emit contract subset).
type SpanRecord struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	ServiceName  string
	Project      string
	Version      string
	Environment  string
	Method       string
	URL          string
	StatusCode   int
	Duration     time.Duration
	StartTime    time.Time
	OK           bool
	RequestID    string
}

// LogRecord is a normalized log line correlated to a trace.
type LogRecord struct {
	TraceID  string    `json:"trace_id,omitempty"`
	SpanID   string    `json:"span_id,omitempty"`
	Time     time.Time `json:"time"`
	Body     string    `json:"body"`
	Severity string    `json:"severity,omitempty"`
	Project  string    `json:"-"`
	Version  string    `json:"-"`
}

// ContextResponse is GET .../telemetry/context.
type ContextResponse struct {
	Enabled   bool   `json:"enabled"`
	Backend   string `json:"backend"`
	FlushHint string `json:"flush_hint,omitempty"`
	Shed      int64  `json:"shed"`
	DeepLink  string `json:"deep_link,omitempty"`
}

// TraceNode is one span in a trace tree response.
type TraceNode struct {
	TraceID      string        `json:"trace_id"`
	SpanID       string        `json:"span_id"`
	ParentSpanID string        `json:"parent_span_id,omitempty"`
	Name         string        `json:"name"`
	ServiceName  string        `json:"service_name,omitempty"`
	Method       string        `json:"http.method,omitempty"`
	URL          string        `json:"url,omitempty"`
	StatusCode   int           `json:"http.status_code,omitempty"`
	DurationMs   float64       `json:"duration_ms"`
	StartTime    string        `json:"start_time"`
	Children     []TraceNode   `json:"children,omitempty"`
}

// TraceResponse is GET .../telemetry/traces/{trace_id}.
type TraceResponse struct {
	TraceID string      `json:"trace_id"`
	Tree    TraceNode   `json:"tree"`
	Logs    []LogRecord `json:"logs"`
}

// SearchTemplate names the frozen template set (§5.1).
type SearchTemplate string

const (
	TemplateSlow      SearchTemplate = "slow"
	TemplateError     SearchTemplate = "error"
	TemplateStatus    SearchTemplate = "status"
	TemplateBody      SearchTemplate = "body"
	TemplateRequestID SearchTemplate = "request_id"
)

// SearchRequest is POST .../telemetry/search body.
type SearchRequest struct {
	Template  SearchTemplate `json:"template"`
	Start     time.Time      `json:"start"`
	End       time.Time      `json:"end"`
	Limit     int            `json:"limit"`
	Status    *int           `json:"status,omitempty"`
	Body      string         `json:"body,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	SlowMs    *int           `json:"slow_ms,omitempty"`
}

// SearchHit is one row from template search.
type SearchHit struct {
	TraceID    string  `json:"trace_id"`
	SpanID     string  `json:"span_id,omitempty"`
	Name       string  `json:"name"`
	URL        string  `json:"url,omitempty"`
	Method     string  `json:"http.method,omitempty"`
	StatusCode int     `json:"http.status_code,omitempty"`
	DurationMs float64 `json:"duration_ms"`
	StartTime  string  `json:"start_time"`
	Summary    string  `json:"summary,omitempty"`
}

const (
	DefaultSearchLimit = 1000
	MaxSearchLimit     = 10000
	DefaultSlowMs      = 1000
)
