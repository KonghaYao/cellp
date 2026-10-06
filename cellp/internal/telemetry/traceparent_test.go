package telemetry

import (
	"strings"
	"testing"
)

func TestParseTraceparentValid(t *testing.T) {
	tp := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	p := ParseTraceparent(tp)
	if p == nil {
		t.Fatal("expected parse ok")
	}
	if !p.Sampled {
		t.Fatal("expected sampled")
	}
	out := FormatTraceparent(TraceContext{TraceID: p.TraceID, SpanID: p.SpanID, Sampled: true})
	if !strings.HasPrefix(out, "00-") {
		t.Fatalf("format = %q", out)
	}
}

func TestParseTraceparentInvalid(t *testing.T) {
	if ParseTraceparent("bad") != nil {
		t.Fatal("expected nil")
	}
}

func TestStartIngressTraceRoot(t *testing.T) {
	parent, child := StartIngressTrace("")
	if parent.TraceID != child.TraceID || parent.SpanID != child.SpanID {
		t.Fatal("root should use same span")
	}
}

func TestStartIngressTraceChild(t *testing.T) {
	root := NewRootTrace()
	in := FormatTraceparent(root)
	parent, child := StartIngressTrace(in)
	if parent.SpanID == child.SpanID {
		t.Fatal("expected child span")
	}
	if HexTraceID(parent.TraceID) != HexTraceID(child.TraceID) {
		t.Fatal("trace id mismatch")
	}
}
