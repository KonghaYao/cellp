package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LokiClient queries Grafana Loki HTTP API (CELLP_OTEL_BACKEND=lgtm|lgtm-prod).
type LokiClient struct {
	queryURL string
	client   *http.Client
}

func NewLokiClient(queryURL string) *LokiClient {
	if strings.TrimSpace(queryURL) == "" {
		return nil
	}
	return &LokiClient{
		queryURL: strings.TrimRight(queryURL, "/"),
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *LokiClient) QueryLogsForTrace(ctx context.Context, project, version, traceID string, start, end time.Time) ([]LogRecord, error) {
	if c == nil {
		return nil, nil
	}
	traceID = strings.ToLower(strings.TrimSpace(traceID))
	query := fmt.Sprintf(
		`{service_name=~"celld|cellp-gateway"} | json | cellp_project="%s" | cellp_version="%s" | trace_id="%s"`,
		project, version, traceID,
	)
	// Fallback when structured fields are absent: substring match on trace id.
	fallback := fmt.Sprintf(`{service_name=~"celld|cellp-gateway"} |= "%s"`, traceID)
	logs, err := c.queryRange(ctx, query, start, end)
	if err != nil || len(logs) == 0 {
		if fb, fbErr := c.queryRange(ctx, fallback, start, end); fbErr == nil {
			return fb, nil
		}
		return logs, err
	}
	return logs, nil
}

func (c *LokiClient) SearchTraceIDsByBody(ctx context.Context, project, version, body string, start, end time.Time, limit int) ([]string, error) {
	if c == nil || body == "" {
		return nil, nil
	}
	query := fmt.Sprintf(
		`{service_name=~"celld|cellp-gateway"} | json | cellp_project="%s" | cellp_version="%s" |= "%s"`,
		project, version, escapeLogQLString(body),
	)
	fallback := fmt.Sprintf(`{service_name=~"celld|cellp-gateway"} |= "%s"`, escapeLogQLString(body))
	logs, err := c.queryRange(ctx, query, start, end)
	if err != nil || len(logs) == 0 {
		if fb, fbErr := c.queryRange(ctx, fallback, start, end); fbErr == nil {
			logs = fb
		}
	}
	seen := make(map[string]struct{})
	var ids []string
	for _, l := range logs {
		id := strings.ToLower(strings.TrimSpace(l.TraceID))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	return ids, err
}

func escapeLogQLString(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func (c *LokiClient) queryRange(ctx context.Context, query string, start, end time.Time) ([]LogRecord, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", "1000")
	u := c.queryURL + "/loki/api/v1/query_range?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("loki query: %s", strings.TrimSpace(string(body)))
	}
	var payload lokiQueryPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var out []LogRecord
	for _, stream := range payload.Data.Result {
		for _, pair := range stream.Values {
			if len(pair) < 2 {
				continue
			}
			tsNano, err := strconv.ParseInt(pair[0], 10, 64)
			if err != nil {
				continue
			}
			rec := LogRecord{
				Time: time.Unix(0, tsNano).UTC(),
				Body: pair[1],
			}
			if tid := stream.Stream["trace_id"]; tid != "" {
				rec.TraceID = tid
			}
			out = append(out, rec)
		}
	}
	return out, nil
}

type lokiQueryPayload struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
