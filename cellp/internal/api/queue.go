package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

const defaultQueueMax = 10000

// QueueMax returns deploy queue depth limit from CELLP_QUEUE_MAX (default 10000).
func QueueMax() int {
	if v := os.Getenv("CELLP_QUEUE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultQueueMax
}

type queueContext struct {
	operatorContext
	name string
}

func (s *Server) resolveQueueContext(r *http.Request) (*queueContext, error) {
	base, err := s.resolveReadyOperator(r)
	if err != nil {
		return nil, err
	}
	name := chi.URLParam(r, "name")
	if !runtime.HasQueue(base.projectDir, name) {
		return nil, &operatorError{status: http.StatusNotFound, code: "queue_not_found"}
	}
	return &queueContext{operatorContext: *base, name: name}, nil
}

func parseOperatorLimit(r *http.Request, defaultN int) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return defaultN, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100 {
		return 0, false
	}
	return n, true
}

func (s *Server) writeQueueJSON(w http.ResponseWriter, raw json.RawMessage, err error) {
	if err != nil {
		writeOperatorExecError(w, err)
		return
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		_, _ = w.Write([]byte("\n"))
	}
}

type queueListItem struct {
	Name string `json:"name"`
}

func (s *Server) handleListQueues(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveReadyOperator(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	names := runtime.DeclaredQueueNames(ctx.projectDir)
	items := make([]queueListItem, 0, len(names))
	for _, n := range names {
		items = append(items, queueListItem{Name: n})
	}
	writeJSON(w, http.StatusOK, map[string]any{"queues": items})
}

func (s *Server) handleGetQueueInfo(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	raw, err := s.runtime.QueueInfo(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name)
	s.writeQueueJSON(w, raw, err)
}

func (s *Server) handlePeekQueue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	limit, ok := parseOperatorLimit(r, 10)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
		return
	}
	raw, err := s.runtime.QueuePeek(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name, limit)
	s.writeQueueJSON(w, raw, err)
}

func (s *Server) handlePauseQueue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	raw, err := s.runtime.QueuePause(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name)
	s.writeQueueJSON(w, raw, err)
}

func (s *Server) handleResumeQueue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	raw, err := s.runtime.QueueResume(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name)
	s.writeQueueJSON(w, raw, err)
}

func (s *Server) handleRedriveQueue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	limit, ok := parseOperatorLimit(r, 100)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_limit"})
		return
	}
	raw, err := s.runtime.QueueRedrive(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name, limit)
	s.writeQueueJSON(w, raw, err)
}

func (s *Server) handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.resolveQueueContext(r)
	if err != nil {
		writeOperatorError(w, err)
		return
	}
	var body struct {
		Force json.RawMessage `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "force_required"})
		return
	}
	if string(bytes.TrimSpace(body.Force)) != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "force_required"})
		return
	}
	raw, err := s.runtime.QueuePurge(r.Context(), ctx.projectID, ctx.versionID, ctx.projectDir, ctx.name)
	s.writeQueueJSON(w, raw, err)
}
