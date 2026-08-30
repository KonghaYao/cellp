package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

type envVar struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Source   string `json:"source"` // platform | override | wrangler
	Readonly bool   `json:"readonly"`
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status == registry.StatusDestroyed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}

	overrides, err := s.store.GetVersionEnv(r.Context(), projectID, versionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	wrangler := map[string]string{}
	projectDir := filepath.Join(s.cfg.ArtifactsDir, projectID, versionID)
	if parsed, err := runtime.ParseWranglerVars(projectDir); err == nil {
		wrangler = parsed
	}

	merged := map[string]envVar{}
	for k, val := range wrangler {
		merged[k] = envVar{Key: k, Value: val, Source: "wrangler", Readonly: false}
	}
	for k, val := range overrides {
		merged[k] = envVar{Key: k, Value: val, Source: "override", Readonly: false}
	}
	merged["PROJECT_ID"] = envVar{Key: "PROJECT_ID", Value: projectID, Source: "platform", Readonly: true}
	merged["VERSION_ID"] = envVar{Key: "VERSION_ID", Value: versionID, Source: "platform", Readonly: true}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vars := make([]envVar, 0, len(keys))
	for _, k := range keys {
		vars = append(vars, merged[k])
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"vars": vars})
}

type putEnvReq struct {
	Vars map[string]string `json:"vars"`
}

func (s *Server) handlePutEnv(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil || v.Status == registry.StatusDestroyed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status == registry.StatusFailed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version_not_ready"})
		return
	}

	var req putEnvReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	normalized, err := config.NormalizeWorkerEnv(req.Vars)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.orch.ApplyWorkerEnv(r.Context(), projectID, versionID, normalized); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
			return
		}
		if strings.Contains(err.Error(), "destroyed") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "version_destroyed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     v.Status,
		"project_id": projectID,
		"id":         versionID,
	})
}
