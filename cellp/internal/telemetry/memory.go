package telemetry

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxSpans = 50_000
	defaultMaxLogs  = 100_000
	defaultMaxAge   = 24 * time.Hour
)

// MemoryStore holds spans and logs in-process with ring caps (CELLP_OTEL_BACKEND=memory).
type MemoryStore struct {
	mu        sync.RWMutex
	spans     []SpanRecord
	logs      []LogRecord
	maxSpans  int
	maxLogs   int
	maxAge    time.Duration
	shedCount int64
}

func NewMemoryStore(maxSpans, maxLogs int, maxAge time.Duration) *MemoryStore {
	if maxSpans <= 0 {
		maxSpans = defaultMaxSpans
	}
	if maxLogs <= 0 {
		maxLogs = defaultMaxLogs
	}
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	return &MemoryStore{maxSpans: maxSpans, maxLogs: maxLogs, maxAge: maxAge}
}

func (m *MemoryStore) ShedCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shedCount
}

func (m *MemoryStore) IngestSpan(span SpanRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	if len(m.spans) >= m.maxSpans {
		m.shedCount++
		copy(m.spans, m.spans[1:])
		m.spans = m.spans[:len(m.spans)-1]
	}
	m.spans = append(m.spans, span)
}

func (m *MemoryStore) IngestLog(log LogRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	if len(m.logs) >= m.maxLogs {
		m.shedCount++
		copy(m.logs, m.logs[1:])
		m.logs = m.logs[:len(m.logs)-1]
	}
	m.logs = append(m.logs, log)
}

func (m *MemoryStore) pruneLocked(now time.Time) {
	cutoff := now.Add(-m.maxAge)
	i := 0
	for _, s := range m.spans {
		if !s.StartTime.Before(cutoff) {
			m.spans[i] = s
			i++
		}
	}
	m.spans = m.spans[:i]
	j := 0
	for _, l := range m.logs {
		if !l.Time.Before(cutoff) {
			m.logs[j] = l
			j++
		}
	}
	m.logs = m.logs[:j]
}

type MemoryBackend struct {
	store *MemoryStore
}

func NewMemoryBackend(store *MemoryStore) *MemoryBackend {
	return &MemoryBackend{store: store}
}

func (b *MemoryBackend) Context(_ context.Context, _, _ string) (ContextResponse, error) {
	return ContextResponse{
		Enabled:   true,
		Backend:   "memory",
		FlushHint: "immediate",
		Shed:      b.store.ShedCount(),
	}, nil
}

func (b *MemoryBackend) GetTrace(_ context.Context, project, version, traceID string) (TraceResponse, error) {
	b.store.mu.RLock()
	defer b.store.mu.RUnlock()
	var matched []SpanRecord
	for _, s := range b.store.spans {
		if s.Project == project && s.Version == version && strings.EqualFold(s.TraceID, traceID) {
			matched = append(matched, s)
		}
	}
	if len(matched) == 0 {
		return TraceResponse{}, ErrTraceNotFound
	}
	var logs []LogRecord
	for _, l := range b.store.logs {
		if l.Project == project && l.Version == version && strings.EqualFold(l.TraceID, traceID) {
			logs = append(logs, l)
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].Time.Before(logs[j].Time) })
	root := buildTree(matched, traceID)
	return TraceResponse{TraceID: strings.ToLower(traceID), Tree: root, Logs: logs}, nil
}

func (b *MemoryBackend) Search(_ context.Context, project, version string, req SearchRequest) ([]SearchHit, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	slowMs := DefaultSlowMs
	if req.SlowMs != nil && *req.SlowMs > 0 {
		slowMs = *req.SlowMs
	}
	b.store.mu.RLock()
	defer b.store.mu.RUnlock()
	var hits []SearchHit
scan:
	for i := len(b.store.spans) - 1; i >= 0; i-- {
		s := b.store.spans[i]
		if s.Project != project || s.Version != version {
			continue
		}
		if req.Start.After(time.Time{}) && s.StartTime.Before(req.Start) {
			continue
		}
		if req.End.After(time.Time{}) && s.StartTime.After(req.End) {
			continue
		}
		switch req.Template {
		case TemplateSlow:
			if s.Duration < time.Duration(slowMs)*time.Millisecond {
				continue
			}
		case TemplateError:
			if s.OK && s.StatusCode < 500 {
				continue
			}
		case TemplateStatus:
			if req.Status == nil || s.StatusCode != *req.Status {
				continue
			}
		case TemplateRequestID:
			if req.RequestID == "" || !strings.EqualFold(s.RequestID, req.RequestID) {
				continue
			}
		case TemplateBody:
			if req.Body == "" {
				continue
			}
			found := false
			for _, l := range b.store.logs {
				if l.Project == project && l.Version == version && strings.EqualFold(l.TraceID, s.TraceID) &&
					strings.Contains(strings.ToLower(l.Body), strings.ToLower(req.Body)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		default:
			continue scan
		}
		hits = append(hits, spanHit(s))
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}

func (b *MemoryBackend) DeepLink(_, _ string, _ string) string { return "" }

func spanHit(s SpanRecord) SearchHit {
	return SearchHit{
		TraceID:    s.TraceID,
		SpanID:     s.SpanID,
		Name:       s.Name,
		URL:        s.URL,
		Method:     s.Method,
		StatusCode: s.StatusCode,
		DurationMs: float64(s.Duration) / float64(time.Millisecond),
		StartTime:  s.StartTime.UTC().Format(time.RFC3339Nano),
	}
}

func buildTree(spans []SpanRecord, traceID string) TraceNode {
	byID := make(map[string]SpanRecord, len(spans))
	for _, s := range spans {
		byID[s.SpanID] = s
	}
	children := make(map[string][]SpanRecord)
	var roots []SpanRecord
	for _, s := range spans {
		if s.ParentSpanID == "" {
			roots = append(roots, s)
			continue
		}
		children[s.ParentSpanID] = append(children[s.ParentSpanID], s)
	}
	if len(roots) == 0 && len(spans) > 0 {
		roots = []SpanRecord{spans[0]}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].StartTime.Before(roots[j].StartTime) })
	if len(roots) == 0 {
		return TraceNode{TraceID: traceID}
	}
	return nodeFromSpan(roots[0], children)
}

func nodeFromSpan(s SpanRecord, children map[string][]SpanRecord) TraceNode {
	n := TraceNode{
		TraceID:      s.TraceID,
		SpanID:       s.SpanID,
		ParentSpanID: s.ParentSpanID,
		Name:         s.Name,
		ServiceName:  s.ServiceName,
		Method:       s.Method,
		URL:          s.URL,
		StatusCode:   s.StatusCode,
		DurationMs:   float64(s.Duration) / float64(time.Millisecond),
		StartTime:    s.StartTime.UTC().Format(time.RFC3339Nano),
	}
	kids := children[s.SpanID]
	sort.Slice(kids, func(i, j int) bool { return kids[i].StartTime.Before(kids[j].StartTime) })
	for _, c := range kids {
		n.Children = append(n.Children, nodeFromSpan(c, children))
	}
	return n
}
