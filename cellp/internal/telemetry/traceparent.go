package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// ParentContext is a parsed W3C traceparent parent.
type ParentContext struct {
	TraceID  [16]byte
	SpanID   [8]byte
	Sampled  bool
}

// TraceContext is the active trace for propagation.
type TraceContext struct {
	TraceID  [16]byte
	SpanID   [8]byte
	Sampled  bool
}

// ParseTraceparent parses W3C traceparent. Malformed values return nil.
func ParseTraceparent(value string) *ParentContext {
	value = strings.TrimSpace(value)
	if len(value) < 55 || value[2] != '-' || value[35] != '-' || value[52] != '-' {
		return nil
	}
	version, err := hex.DecodeString(value[0:2])
	if err != nil || len(version) != 1 {
		return nil
	}
	if version[0] == 0xff || (version[0] == 0 && len(value) != 55) {
		return nil
	}
	traceID, err := hex.DecodeString(value[3:35])
	if err != nil || len(traceID) != 16 {
		return nil
	}
	spanID, err := hex.DecodeString(value[36:52])
	if err != nil || len(spanID) != 8 {
		return nil
	}
	flags, err := hex.DecodeString(value[53:55])
	if err != nil || len(flags) != 1 {
		return nil
	}
	allZeroTrace := true
	for _, b := range traceID {
		if b != 0 {
			allZeroTrace = false
			break
		}
	}
	allZeroSpan := true
	for _, b := range spanID {
		if b != 0 {
			allZeroSpan = false
			break
		}
	}
	if allZeroTrace || allZeroSpan {
		return nil
	}
	return &ParentContext{
		TraceID: copy16(traceID),
		SpanID:  copy8(spanID),
		Sampled: flags[0]&1 == 1,
	}
}

// FormatTraceparent builds a W3C traceparent header value.
func FormatTraceparent(ctx TraceContext) string {
	flags := "00"
	if ctx.Sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", hex.EncodeToString(ctx.TraceID[:]), hex.EncodeToString(ctx.SpanID[:]), flags)
}

// NewRootTrace creates a sampled root trace context.
func NewRootTrace() TraceContext {
	var traceID [16]byte
	var spanID [8]byte
	_, _ = rand.Read(traceID[:])
	_, _ = rand.Read(spanID[:])
	return TraceContext{TraceID: traceID, SpanID: spanID, Sampled: true}
}

// ChildSpan returns a child context with a new span id under the same trace.
func ChildSpan(parent TraceContext) TraceContext {
	var spanID [8]byte
	_, _ = rand.Read(spanID[:])
	return TraceContext{TraceID: parent.TraceID, SpanID: spanID, Sampled: parent.Sampled}
}

// StartIngressTrace extracts or creates a trace and returns (parent, gatewaySpan).
// gatewaySpan is what we put on the upstream traceparent header.
func StartIngressTrace(incoming string) (parent TraceContext, gatewaySpan TraceContext) {
	if p := ParseTraceparent(incoming); p != nil {
		parent = TraceContext{TraceID: p.TraceID, SpanID: p.SpanID, Sampled: p.Sampled}
		gatewaySpan = ChildSpan(parent)
		return parent, gatewaySpan
	}
	gatewaySpan = NewRootTrace()
	return gatewaySpan, gatewaySpan
}

func copy16(b []byte) [16]byte {
	var out [16]byte
	copy(out[:], b)
	return out
}

func copy8(b []byte) [8]byte {
	var out [8]byte
	copy(out[:], b)
	return out
}

func HexTraceID(id [16]byte) string { return hex.EncodeToString(id[:]) }
func HexSpanID(id [8]byte) string   { return hex.EncodeToString(id[:]) }
