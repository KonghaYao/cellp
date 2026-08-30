package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// shouldCountVersionAccess reports whether a version-scoped API response
// counts as use for AD-9 last_access (idle archive). Status polling and
// lifecycle verbs do not; operator KV/queue/D1 routes do. 503 is ignored
// (same as Gateway).
func shouldCountVersionAccess(method, pathSuffix string, status int) bool {
	if status < 200 || status >= 300 || status == http.StatusServiceUnavailable {
		return false
	}
	rest := strings.TrimSuffix(pathSuffix, "/")
	switch rest {
	case "", "/promote", "/archive", "/wake", "/pin", "/unpin":
		return false
	case "/env":
		// GET is Dashboard metadata (like GET version). PUT mutates runtime.
		return method != http.MethodGet && method != http.MethodHead
	default:
		return true
	}
}

func (s *Server) touchVersionAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		projectID := chi.URLParam(r, "projectID")
		versionID := chi.URLParam(r, "versionID")
		if projectID == "" || versionID == "" {
			return
		}
		prefix := fmt.Sprintf("/v1/projects/%s/versions/%s", projectID, versionID)
		suffix := strings.TrimPrefix(r.URL.Path, prefix)
		if !shouldCountVersionAccess(r.Method, suffix, rw.status) {
			return
		}
		s.touchLastAccessThrottled(projectID, versionID)
	})
}

func (s *Server) touchLastAccessThrottled(projectID, versionID string) {
	key := projectID + "/" + versionID
	now := time.Now().UTC()
	s.lastTouchMu.Lock()
	if last, ok := s.lastTouchAt[key]; ok && now.Sub(last) < time.Minute {
		s.lastTouchMu.Unlock()
		return
	}
	s.lastTouchAt[key] = now
	s.lastTouchMu.Unlock()
	go func() {
		_ = s.store.TouchLastAccess(context.Background(), projectID, versionID)
	}()
}
