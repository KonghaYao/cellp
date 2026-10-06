package telemetry

import (
	"context"
	"errors"
)

var (
	ErrTraceNotFound   = errors.New("trace_not_found")
	ErrVersionNotFound = errors.New("version_not_found")
	ErrScanTooLarge    = errors.New("scan_too_large")
	ErrInvalidTemplate = errors.New("invalid_template")
	ErrInvalidWindow   = errors.New("invalid_time_window")
)

// Backend is the query driver (AD-14 §5.2).
type Backend interface {
	Context(ctx context.Context, project, version string) (ContextResponse, error)
	GetTrace(ctx context.Context, project, version, traceID string) (TraceResponse, error)
	Search(ctx context.Context, project, version string, req SearchRequest) ([]SearchHit, error)
	DeepLink(project, version, view string) string
}

// Recorder ingests spans/logs into backends that support live capture.
type Recorder interface {
	IngestSpan(span SpanRecord)
	IngestLog(log LogRecord)
	ShedCount() int64
}
