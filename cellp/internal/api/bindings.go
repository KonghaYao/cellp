package api

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetBindings(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")

	v, err := s.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status != registry.StatusReady {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_ready"})
		return
	}

	projectDir := filepath.Join(s.cfg.ArtifactsDir, projectID, versionID)
	bindings, err := runtime.ParseBindings(projectDir)
	if err != nil {
		if errors.Is(err, runtime.ErrNoWrangler) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrangler_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, bindings)
}
