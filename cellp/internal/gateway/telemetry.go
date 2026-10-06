package gateway

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/telemetry"
)

func deploymentEnv(binding *registry.IngressBinding) string {
	if binding != nil && binding.Role == registry.IngressRoleProd {
		return "prod"
	}
	return "preview"
}

type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("gateway statusCapturingWriter: underlying ResponseWriter is not a Hijacker")
	}
	return h.Hijack()
}

func (w *statusCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (g *Gateway) withIngressTrace(w http.ResponseWriter, r *http.Request, binding *registry.IngressBinding, projectID, versionID string, next func(http.ResponseWriter, *http.Request)) {
	parent, spanCtx := telemetry.StartIngressTrace(r.Header.Get("traceparent"))
	applyTraceparent(r, spanCtx)
	start := time.Now()
	if isUpgradeRequest(r) {
		next(w, r)
		if g.telemetry != nil && g.telemetry.Enabled() {
			g.telemetry.RecordIngress(
				projectID, versionID, deploymentEnv(binding),
				r.Method, r.URL.Path, http.StatusSwitchingProtocols, time.Since(start),
				parent, spanCtx,
			)
		}
		return
	}
	cw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
	next(cw, r)
	if g.telemetry != nil && g.telemetry.Enabled() {
		g.telemetry.RecordIngress(
			projectID, versionID, deploymentEnv(binding),
			r.Method, r.URL.Path, cw.status, time.Since(start),
			parent, spanCtx,
		)
	}
}

func applyTraceparent(req *http.Request, ctx telemetry.TraceContext) {
	req.Header.Set("traceparent", telemetry.FormatTraceparent(ctx))
}
