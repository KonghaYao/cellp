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

// TempoBackend queries Grafana Tempo HTTP API (CELLP_OTEL_BACKEND=lgtm|lgtm-prod).
type TempoBackend struct {
	queryURL   string
	grafanaURL string
	client     *http.Client
	memory     *MemoryBackend
	loki       *LokiClient
}

func NewTempoBackend(queryURL, grafanaURL string, memory *MemoryBackend, loki *LokiClient) *TempoBackend {
	return &TempoBackend{
		queryURL:   strings.TrimRight(queryURL, "/"),
		grafanaURL: strings.TrimRight(grafanaURL, "/"),
		client:     &http.Client{Timeout: 15 * time.Second},
		memory:     memory,
		loki:       loki,
	}
}

func (b *TempoBackend) Context(_ context.Context, _, _ string) (ContextResponse, error) {
	resp := ContextResponse{Enabled: true, Backend: "lgtm", FlushHint: "1-5s"}
	if b.memory != nil {
		resp.Shed = b.memory.store.ShedCount()
	}
	return resp, nil
}

func (b *TempoBackend) DeepLink(project, version, view string) string {
	if b.grafanaURL == "" {
		return ""
	}
	if view == "" {
		view = "explore"
	}
	left := fmt.Sprintf(`{"datasource":"Tempo","queries":[{"queryType":"traceqlSearch","filters":[{"id":"resource.cellp.project","operator":"=","value":"%s"},{"id":"resource.cellp.version","operator":"=","value":"%s"}]}]}`, project, version)
	q := url.Values{}
	q.Set("orgId", "1")
	q.Set("left", left)
	return b.grafanaURL + "/" + view + "?" + q.Encode()
}

func (b *TempoBackend) GetTrace(ctx context.Context, project, version, traceID string) (TraceResponse, error) {
	if b.memory != nil {
		if tr, err := b.memory.GetTrace(ctx, project, version, traceID); err == nil {
			return tr, nil
		}
	}
	u := fmt.Sprintf("%s/api/traces/%s", b.queryURL, url.PathEscape(traceID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return TraceResponse{}, err
	}
	httpResp, err := b.client.Do(req)
	if err != nil {
		return TraceResponse{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode == http.StatusNotFound {
		return TraceResponse{}, ErrTraceNotFound
	}
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return TraceResponse{}, fmt.Errorf("tempo query: %s", strings.TrimSpace(string(body)))
	}
	var payload tempoTracePayload
	if err := json.NewDecoder(httpResp.Body).Decode(&payload); err != nil {
		return TraceResponse{}, err
	}
	spans := payload.filter(project, version)
	if len(spans) == 0 {
		return TraceResponse{}, ErrTraceNotFound
	}
	tree := tempoBuildTree(spans, traceID)
	out := TraceResponse{TraceID: strings.ToLower(traceID), Tree: tree}
	if b.loki != nil {
		start := time.Now().Add(-24 * time.Hour)
		end := time.Now().Add(time.Minute)
		if logs, err := b.loki.QueryLogsForTrace(ctx, project, version, traceID, start, end); err == nil && len(logs) > 0 {
			out.Logs = logs
		}
	}
	if len(out.Logs) == 0 && b.memory != nil {
		if tr, err := b.memory.GetTrace(ctx, project, version, traceID); err == nil {
			out.Logs = tr.Logs
		}
	}
	return out, nil
}

func (b *TempoBackend) Search(ctx context.Context, project, version string, req SearchRequest) ([]SearchHit, error) {
	if b.memory != nil {
		if hits, err := b.memory.Search(ctx, project, version, req); err == nil && len(hits) > 0 {
			return hits, nil
		}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("start", strconv.FormatInt(req.Start.Unix(), 10))
	q.Set("end", strconv.FormatInt(req.End.Unix(), 10))
	baseTags := fmt.Sprintf("resource.cellp.project=%s resource.cellp.version=%s", project, version)
	switch req.Template {
	case TemplateError:
		q.Set("tags", baseTags+" status=error")
	case TemplateStatus:
		if req.Status != nil {
			q.Set("tags", fmt.Sprintf("%s http.status_code=%d", baseTags, *req.Status))
		} else {
			q.Set("tags", baseTags)
		}
	case TemplateRequestID:
		if req.RequestID != "" {
			q.Set("tags", fmt.Sprintf("%s request_id=%s", baseTags, req.RequestID))
		} else {
			q.Set("tags", baseTags)
		}
	case TemplateSlow:
		q.Set("tags", baseTags)
		slowMs := DefaultSlowMs
		if req.SlowMs != nil && *req.SlowMs > 0 {
			slowMs = *req.SlowMs
		}
		q.Set("minDuration", fmt.Sprintf("%dms", slowMs))
	case TemplateBody:
		if b.loki != nil && req.Body != "" {
			ids, err := b.loki.SearchTraceIDsByBody(ctx, project, version, req.Body, req.Start, req.End, limit)
			if err != nil {
				return nil, err
			}
			return tempoHitsFromTraceIDs(ids, limit), nil
		}
		if b.memory != nil {
			return b.memory.Search(ctx, project, version, req)
		}
		q.Set("tags", baseTags)
	default:
		q.Set("tags", baseTags)
	}
	u := b.queryURL + "/api/search?" + q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tempo search: %s", strings.TrimSpace(string(body)))
	}
	var payload tempoSearchPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, tr := range payload.Traces {
		hits = append(hits, SearchHit{
			TraceID:   tr.TraceID,
			Name:      tr.RootServiceName,
			StartTime: time.Unix(tr.StartTimeUnix, 0).UTC().Format(time.RFC3339Nano),
			Summary:   tr.RootTraceName,
		})
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}

func tempoHitsFromTraceIDs(ids []string, limit int) []SearchHit {
	var hits []SearchHit
	for _, id := range ids {
		hits = append(hits, SearchHit{TraceID: id})
		if limit > 0 && len(hits) >= limit {
			break
		}
	}
	return hits
}

func tempoSlowMs(req SearchRequest) int {
	slowMs := DefaultSlowMs
	if req.SlowMs != nil && *req.SlowMs > 0 {
		slowMs = *req.SlowMs
	}
	return slowMs
}

type tempoSearchPayload struct {
	Traces []struct {
		TraceID         string `json:"traceID"`
		RootServiceName string `json:"rootServiceName"`
		RootTraceName   string `json:"rootTraceName"`
		StartTimeUnix   int64  `json:"startTimeUnix"`
	} `json:"traces"`
}

type tempoTracePayload struct {
	Batches []struct {
		Resource struct {
			Attributes []tempoAttr `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []tempoSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"batches"`
}

type tempoSpan struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	ParentSpanID      string      `json:"parentSpanId"`
	Name              string      `json:"name"`
	StartTimeUnixNano string      `json:"startTimeUnixNano"`
	EndTimeUnixNano   string      `json:"endTimeUnixNano"`
	Attributes        []tempoAttr `json:"attributes"`
}

type tempoAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
		IntValue    string `json:"intValue"`
	} `json:"value"`
}

func (a tempoAttr) stringVal() string {
	if a.Value.StringValue != "" {
		return a.Value.StringValue
	}
	return a.Value.IntValue
}

func (p tempoTracePayload) filter(project, version string) []tempoSpan {
	var out []tempoSpan
	for _, batch := range p.Batches {
		proj, ver := "", ""
		for _, attr := range batch.Resource.Attributes {
			switch attr.Key {
			case "cellp.project":
				proj = attr.stringVal()
			case "cellp.version":
				ver = attr.stringVal()
			}
		}
		if proj != project || ver != version {
			continue
		}
		for _, ss := range batch.ScopeSpans {
			out = append(out, ss.Spans...)
		}
	}
	return out
}

func tempoBuildTree(spans []tempoSpan, traceID string) TraceNode {
	if len(spans) == 0 {
		return TraceNode{TraceID: traceID}
	}
	byID := make(map[string]tempoSpan)
	children := make(map[string][]tempoSpan)
	var root tempoSpan
	for _, s := range spans {
		byID[s.SpanID] = s
	}
	for _, s := range spans {
		if s.ParentSpanID == "" || s.ParentSpanID == "0000000000000000" {
			root = s
			continue
		}
		children[s.ParentSpanID] = append(children[s.ParentSpanID], s)
	}
	if root.SpanID == "" {
		root = spans[0]
	}
	return tempoNode(root, children)
}

func tempoNode(s tempoSpan, children map[string][]tempoSpan) TraceNode {
	start, _ := strconv.ParseInt(s.StartTimeUnixNano, 10, 64)
	end, _ := strconv.ParseInt(s.EndTimeUnixNano, 10, 64)
	method, urlPath, status := "", "", 0
	for _, a := range s.Attributes {
		switch a.Key {
		case "http.method":
			method = a.stringVal()
		case "url.full":
			urlPath = a.stringVal()
		case "http.response.status_code", "http.status_code":
			status, _ = strconv.Atoi(a.stringVal())
		}
	}
	n := TraceNode{
		TraceID:    s.TraceID,
		SpanID:     s.SpanID,
		ParentSpanID: s.ParentSpanID,
		Name:       s.Name,
		Method:     method,
		URL:        urlPath,
		StatusCode: status,
		DurationMs: float64(end-start) / 1e6,
		StartTime:  time.Unix(0, start).UTC().Format(time.RFC3339Nano),
	}
	for _, c := range children[s.SpanID] {
		n.Children = append(n.Children, tempoNode(c, children))
	}
	return n
}
