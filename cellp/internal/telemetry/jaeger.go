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

// JaegerBackend queries Jaeger all-in-one HTTP API (CELLP_OTEL_BACKEND=jaeger).
type JaegerBackend struct {
	queryURL   string
	grafanaURL string
	client     *http.Client
	memory     *MemoryBackend
}

func NewJaegerBackend(queryURL, grafanaURL string, memory *MemoryBackend) *JaegerBackend {
	return &JaegerBackend{
		queryURL:   strings.TrimRight(queryURL, "/"),
		grafanaURL: strings.TrimRight(grafanaURL, "/"),
		client:     &http.Client{Timeout: 15 * time.Second},
		memory:     memory,
	}
}

func (b *JaegerBackend) Context(_ context.Context, _, _ string) (ContextResponse, error) {
	resp := ContextResponse{Enabled: true, Backend: "jaeger", FlushHint: "1-5s"}
	if b.memory != nil {
		resp.Shed = b.memory.store.ShedCount()
	}
	return resp, nil
}

func (b *JaegerBackend) DeepLink(project, version, view string) string {
	if b.grafanaURL == "" {
		return ""
	}
	if view == "" {
		view = "explore"
	}
	q := url.Values{}
	q.Set("orgId", "1")
	left := fmt.Sprintf(`{"datasource":"Jaeger","queries":[{"query":"service=cellp-gateway cellp.project=%s cellp.version=%s"}]}`, project, version)
	q.Set("left", left)
	return b.grafanaURL + "/" + view + "?" + q.Encode()
}

func (b *JaegerBackend) GetTrace(ctx context.Context, project, version, traceID string) (TraceResponse, error) {
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
	resp, err := b.client.Do(req)
	if err != nil {
		return TraceResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return TraceResponse{}, ErrTraceNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return TraceResponse{}, fmt.Errorf("jaeger query: %s", strings.TrimSpace(string(body)))
	}
	var payload jaegerTracePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return TraceResponse{}, err
	}
	spans := payload.filter(project, version)
	if len(spans) == 0 {
		return TraceResponse{}, ErrTraceNotFound
	}
	tree := jaegerBuildTree(spans, traceID)
	return TraceResponse{TraceID: strings.ToLower(traceID), Tree: tree, Logs: nil}, nil
}

func (b *JaegerBackend) Search(ctx context.Context, project, version string, req SearchRequest) ([]SearchHit, error) {
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
	q.Set("service", "cellp-gateway")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("start", fmt.Sprintf("%d", req.Start.UnixMicro()))
	q.Set("end", fmt.Sprintf("%d", req.End.UnixMicro()))
	tags := map[string]string{
		"cellp.project": project,
		"cellp.version": version,
	}
	switch req.Template {
	case TemplateError:
		tags["error"] = "true"
	case TemplateStatus:
		if req.Status != nil {
			tags["http.status_code"] = strconv.Itoa(*req.Status)
		}
	case TemplateRequestID:
		if req.RequestID != "" {
			tags["request_id"] = req.RequestID
		}
	case TemplateSlow:
		slowMs := tempoSlowMs(req)
		q.Set("minDuration", fmt.Sprintf("%dus", slowMs*1000))
	case TemplateBody:
		if b.memory != nil {
			return b.memory.Search(ctx, project, version, req)
		}
	}
	tagJSON, _ := json.Marshal(tags)
	q.Set("tags", string(tagJSON))
	u := b.queryURL + "/api/traces?" + q.Encode()
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
		return nil, fmt.Errorf("jaeger search: %s", strings.TrimSpace(string(body)))
	}
	var payload jaegerSearchPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, data := range payload.Data {
		for _, sp := range data.Spans {
			if !sp.matches(project, version) {
				continue
			}
			hits = append(hits, sp.hit())
			if len(hits) >= limit {
				return hits, nil
			}
		}
	}
	return hits, nil
}

type jaegerSearchPayload struct {
	Data []struct {
		Spans []jaegerSpan `json:"spans"`
	} `json:"data"`
}

type jaegerTracePayload struct {
	Data []struct {
		Spans   []jaegerSpan `json:"spans"`
		Process map[string]struct {
			ServiceName string `json:"serviceName"`
		} `json:"processes"`
	} `json:"data"`
}

type jaegerSpan struct {
	TraceID       string            `json:"traceID"`
	SpanID        string            `json:"spanID"`
	OperationName string            `json:"operationName"`
	StartTime     int64             `json:"startTime"`
	Duration      int64             `json:"duration"`
	Tags          []jaegerTag       `json:"tags"`
	ProcessID     string            `json:"processID"`
	References    []jaegerReference `json:"references"`
}

type jaegerReference struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type jaegerTag struct {
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

func (s jaegerSpan) tag(key string) string {
	for _, t := range s.Tags {
		if t.Key == key {
			switch v := t.Value.(type) {
			case string:
				return v
			case float64:
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	return ""
}

func (s jaegerSpan) matches(project, version string) bool {
	return s.tag("cellp.project") == project && s.tag("cellp.version") == version
}

func (s jaegerSpan) hit() SearchHit {
	status, _ := strconv.Atoi(s.tag("http.status_code"))
	start := time.UnixMicro(s.StartTime)
	return SearchHit{
		TraceID:    s.TraceID,
		SpanID:     s.SpanID,
		Name:       s.OperationName,
		URL:        s.tag("url.full"),
		Method:     s.tag("http.method"),
		StatusCode: status,
		DurationMs: float64(s.Duration) / 1000.0,
		StartTime:  start.UTC().Format(time.RFC3339Nano),
	}
}

func (p jaegerTracePayload) filter(project, version string) []jaegerSpan {
	var out []jaegerSpan
	for _, d := range p.Data {
		for _, sp := range d.Spans {
			if sp.matches(project, version) {
				out = append(out, sp)
			}
		}
	}
	return out
}

func jaegerBuildTree(spans []jaegerSpan, traceID string) TraceNode {
	if len(spans) == 0 {
		return TraceNode{TraceID: traceID}
	}
	parent := make(map[string]string)
	byID := make(map[string]jaegerSpan)
	for _, s := range spans {
		byID[s.SpanID] = s
		for _, ref := range s.References {
			if ref.RefType == "CHILD_OF" {
				parent[s.SpanID] = ref.SpanID
			}
		}
	}
	var rootID string
	for id := range byID {
		if _, ok := parent[id]; !ok {
			rootID = id
			break
		}
	}
	if rootID == "" {
		rootID = spans[0].SpanID
	}
	children := make(map[string][]jaegerSpan)
	for id, p := range parent {
		children[p] = append(children[p], byID[id])
	}
	return jaegerNode(byID[rootID], children)
}

func jaegerNode(s jaegerSpan, children map[string][]jaegerSpan) TraceNode {
	status, _ := strconv.Atoi(s.tag("http.status_code"))
	n := TraceNode{
		TraceID:    s.TraceID,
		SpanID:     s.SpanID,
		Name:       s.OperationName,
		Method:     s.tag("http.method"),
		URL:        s.tag("url.full"),
		StatusCode: status,
		DurationMs: float64(s.Duration) / 1000.0,
		StartTime:  time.UnixMicro(s.StartTime).UTC().Format(time.RFC3339Nano),
	}
	for _, c := range children[s.SpanID] {
		n.Children = append(n.Children, jaegerNode(c, children))
	}
	return n
}
