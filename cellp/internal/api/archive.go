package api

import (
	"net/http"
	"strings"

	"github.com/cellp/cellp/internal/registry"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	proj, _ := s.store.GetProject(r.Context(), projectID)
	if proj != nil && proj.ProdVersionID != nil && *proj.ProdVersionID == versionID {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cannot_archive_prod"})
		return
	}
	if v.Pinned {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_pinned"})
		return
	}
	if v.Status != registry.StatusReady {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_not_ready"})
		return
	}
	if err := s.orch.Archive(r.Context(), projectID, versionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     registry.StatusArchived,
		"id":         versionID,
		"project_id": projectID,
	})
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status == registry.StatusDestroyed || v.Status == registry.StatusFailed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_not_wakeable"})
		return
	}
	if v.Status != registry.StatusArchived {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_not_archived"})
		return
	}
	if err := s.orch.Wake(r.Context(), projectID, versionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     registry.StatusReady,
		"id":         versionID,
		"project_id": projectID,
	})
}

func (s *Server) handlePin(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status != registry.StatusReady && v.Status != registry.StatusArchived {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_not_ready"})
		return
	}
	if err := s.store.SetVersionPinned(r.Context(), projectID, versionID, true); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pinned":     true,
		"id":         versionID,
		"project_id": projectID,
	})
}

func (s *Server) handleUnpin(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if err := s.store.SetVersionPinned(r.Context(), projectID, versionID, false); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pinned":     false,
		"id":         versionID,
		"project_id": projectID,
	})
}
