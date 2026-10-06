package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OTLPExporter sends gateway spans to an OTLP/HTTP collector.
type OTLPExporter struct {
	endpoint string
	client   *http.Client
}

func NewOTLPExporter(collectorURL string) *OTLPExporter {
	base := strings.TrimRight(collectorURL, "/")
	return &OTLPExporter{
		endpoint: base + "/v1/traces",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func shouldExportGateway(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "jaeger", "lgtm", "lgtm-prod":
		return true
	default:
		return false
	}
}

func (e *OTLPExporter) ExportIngress(ctx context.Context, rec SpanRecord, parent TraceContext) {
	if e == nil {
		return
	}
	payload := otlpTracePayload(rec, parent)
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func otlpTracePayload(rec SpanRecord, parent TraceContext) map[string]interface{} {
	start := rec.StartTime
	end := start.Add(rec.Duration)
	statusCode := 1
	if !rec.OK || rec.StatusCode >= 500 {
		statusCode = 2
	}
	attrs := []map[string]interface{}{
		otlpString("http.method", rec.Method),
		otlpString("url.full", rec.URL),
		otlpInt("http.response.status_code", rec.StatusCode),
		otlpString("cellp.project", rec.Project),
		otlpString("cellp.version", rec.Version),
		otlpString("deployment.environment", rec.Environment),
	}
	if rec.RequestID != "" {
		attrs = append(attrs, otlpString("request_id", rec.RequestID))
	}
	span := map[string]interface{}{
		"traceId":           strings.ToLower(rec.TraceID),
		"spanId":            strings.ToLower(rec.SpanID),
		"name":              rec.Name,
		"kind":              2,
		"startTimeUnixNano": fmt.Sprintf("%d", start.UnixNano()),
		"endTimeUnixNano":   fmt.Sprintf("%d", end.UnixNano()),
		"attributes":        attrs,
		"status":            map[string]interface{}{"code": statusCode},
	}
	if parent.SpanID != [8]byte{} && HexSpanID(parent.SpanID) != rec.SpanID {
		span["parentSpanId"] = strings.ToLower(HexSpanID(parent.SpanID))
	}
	return map[string]interface{}{
		"resourceSpans": []map[string]interface{}{{
			"resource": map[string]interface{}{
				"attributes": []map[string]interface{}{
					otlpString("service.name", rec.ServiceName),
					otlpString("cellp.project", rec.Project),
					otlpString("cellp.version", rec.Version),
				},
			},
			"scopeSpans": []map[string]interface{}{{
				"scope":  map[string]interface{}{"name": rec.ServiceName},
				"spans":  []map[string]interface{}{span},
			}},
		}},
	}
}

func otlpString(key, value string) map[string]interface{} {
	return map[string]interface{}{
		"key":   key,
		"value": map[string]interface{}{"stringValue": value},
	}
}

func otlpInt(key string, value int) map[string]interface{} {
	return map[string]interface{}{
		"key":   key,
		"value": map[string]interface{}{"intValue": fmt.Sprintf("%d", value)},
	}
}
