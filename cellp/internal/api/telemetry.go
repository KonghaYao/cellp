package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/cellp/cellp/internal/telemetry"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerTelemetryRoutes(r chi.Router) {
	r.Route("/telemetry", func(tr chi.Router) {
		tr.Get("/context", s.requireAdmin(s.handleTelemetryContext))
		tr.Get("/traces/{traceID}", s.requireAdmin(s.handleTelemetryGetTrace))
		tr.Post("/search", s.requireAdmin(s.handleTelemetrySearch))
		tr.Get("/logs/stream", s.requireAdmin(s.handleTelemetryLogsStream))
	})
}

func (s *Server) versionOr404(w http.ResponseWriter, r *http.Request) (*registry.Version, bool) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, err := s.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil || v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return nil, false
	}
	if v.Status == registry.StatusDestroyed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return nil, false
	}
	return v, true
}

func (s *Server) handleTelemetryContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.versionOr404(w, r); !ok {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	if s.telemetry == nil {
		writeJSON(w, http.StatusOK, telemetry.ContextResponse{Enabled: false, Backend: "none"})
		return
	}
	ctx, err := s.telemetry.Backend.Context(r.Context(), projectID, versionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if ctx.DeepLink == "" {
		ctx.DeepLink = s.telemetry.Backend.DeepLink(projectID, versionID, "explore")
	}
	writeJSON(w, http.StatusOK, ctx)
}

func (s *Server) handleTelemetryGetTrace(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.versionOr404(w, r); !ok {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	traceID := chi.URLParam(r, "traceID")
	if s.telemetry == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "trace_not_found"})
		return
	}
	tr, err := s.telemetry.Backend.GetTrace(r.Context(), projectID, versionID, traceID)
	if err != nil {
		if errors.Is(err, telemetry.ErrTraceNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "trace_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

func (s *Server) handleTelemetrySearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.versionOr404(w, r); !ok {
		return
	}
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	var raw struct {
		Template  telemetry.SearchTemplate `json:"template"`
		Start     string                   `json:"start"`
		End       string                   `json:"end"`
		Limit     int                      `json:"limit"`
		Status    *int                     `json:"status,omitempty"`
		Body      string                   `json:"body,omitempty"`
		RequestID string                   `json:"request_id,omitempty"`
		SlowMs    *int                     `json:"slow_ms,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	start, err := time.Parse(time.RFC3339Nano, raw.Start)
	if err != nil {
		start, err = time.Parse(time.RFC3339, raw.Start)
	}
	end, endErr := time.Parse(time.RFC3339Nano, raw.End)
	if endErr != nil {
		end, endErr = time.Parse(time.RFC3339, raw.End)
	}
	if err != nil || endErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_time_window"})
		return
	}
	req := telemetry.SearchRequest{
		Template:  raw.Template,
		Start:     start,
		End:       end,
		Limit:     raw.Limit,
		Status:    raw.Status,
		Body:      raw.Body,
		RequestID: raw.RequestID,
		SlowMs:    raw.SlowMs,
	}
	if err := telemetry.ValidateSearchRequest(req); err != nil {
		switch {
		case errors.Is(err, telemetry.ErrInvalidTemplate):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_template"})
		case errors.Is(err, telemetry.ErrInvalidWindow):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_time_window"})
		case errors.Is(err, telemetry.ErrScanTooLarge):
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "scan_too_large"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	if s.telemetry == nil {
		writeJSON(w, http.StatusOK, []telemetry.SearchHit{})
		return
	}
	hits, err := s.telemetry.Backend.Search(r.Context(), projectID, versionID, req)
	if err != nil {
		if errors.Is(err, telemetry.ErrScanTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "scan_too_large"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if hits == nil {
		hits = []telemetry.SearchHit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

func (s *Server) handleTelemetryLogsStream(w http.ResponseWriter, r *http.Request) {
	v, ok := s.versionOr404(w, r)
	if !ok {
		return
	}
	if v.Status == registry.StatusArchived {
		writeJSON(w, http.StatusGone, map[string]string{"error": "version_archived"})
		return
	}
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	logPath := runtime.CelldLogPath(projectID, versionID)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	tail := runtime.TailFileFromEnd(logPath, 64*1024)
	defer tail.Close()
	offset := tail.InitialOffset()
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			offset = n
		}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			chunk, newOff, err := tail.ReadSince(offset)
			if err != nil && !errors.Is(err, io.EOF) {
				_, _ = io.WriteString(w, "event: error\ndata: read_failed\n\n")
				flusher.Flush()
				return
			}
			if len(chunk) > 0 {
				for _, line := range strings.Split(string(chunk), "\n") {
					if line == "" {
						continue
					}
					_, _ = io.WriteString(w, "data: ")
					_, _ = io.WriteString(w, line)
					_, _ = io.WriteString(w, "\n\n")
				}
				offset = newOff
				flusher.Flush()
			}
		}
	}
}
