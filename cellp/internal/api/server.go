package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
	"github.com/go-chi/chi/v5"
)

const maxReadyVersionsDefault = 5

// Server is the cellp REST API (DESIGN §9).
type Server struct {
	store   registry.Store
	queue   job.Queue
	orch    *orch.Orchestrator
	runtime *runtime.Manager
	cfg     config.Config
	router  chi.Router
}

// NewServer creates an API server.
func NewServer(store registry.Store, queue job.Queue, o *orch.Orchestrator, rm *runtime.Manager, cfg config.Config) *Server {
	s := &Server{store: store, queue: queue, orch: o, runtime: rm, cfg: cfg}
	s.router = chi.NewRouter()
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.router.Use(corsMiddleware)

	s.router.Get("/v1/health", s.handleHealth)
	s.router.Get("/v1/health/deep", s.handleHealthDeep)
	s.router.Get("/v1/runtime/routes", s.requireAdmin(s.handleRuntimeRoutes))

	s.router.Route("/v1/projects", func(r chi.Router) {
		r.Get("/", s.requireAdmin(s.handleListProjects))
		r.Post("/", s.requireAdmin(s.handleCreateProject))

		r.Route("/{projectID}", func(r chi.Router) {
			r.Get("/", s.requireAdmin(s.handleGetProject))

			r.Route("/versions", func(r chi.Router) {
				r.Get("/", s.requireAdmin(s.handleListVersions))
				r.Post("/", s.requireDeploy(s.handleCreateVersion))

				r.Route("/{versionID}", func(r chi.Router) {
					r.Get("/", s.requireAdmin(s.handleGetVersion))
					r.Post("/promote", s.requireAdmin(s.handlePromote))
					r.Delete("/", s.requireAdmin(s.handleDestroy))

					r.Get("/bindings", s.requireAdmin(s.handleGetBindings))
					r.Get("/workflows", s.requireAdmin(s.handleListWorkflows))
					r.Get("/workflows/{name}/instances", s.requireAdmin(s.handleListWorkflowInstances))

					r.Route("/database", func(r chi.Router) {
						r.Get("/", s.requireAdmin(s.handleGetDatabase))
						r.Get("/tables", s.requireAdmin(s.handleListDatabaseTables))
						r.Get("/tables/{tableName}/rows", s.requireAdmin(s.handleGetDatabaseTableRows))
						r.Post("/query", s.requireAdmin(s.handleDatabaseQuery))
					})

					r.Route("/kv", func(r chi.Router) {
						r.Get("/", s.requireAdmin(s.handleListKVNamespaces))
						r.Route("/{ns}", func(r chi.Router) {
							r.Get("/", s.requireAdmin(s.handleGetKVInfo))
							r.Get("/keys", s.requireAdmin(s.handleListKVKeys))
							r.Get("/keys/{key:*}", s.requireAdmin(s.handleGetKVValue))
							r.Put("/keys/{key:*}", s.requireAdmin(s.handlePutKVValue))
							r.Delete("/keys/{key:*}", s.requireAdmin(s.handleDeleteKVValue))
						})
					})
					r.Route("/queues", func(r chi.Router) {
						r.Get("/", s.requireAdmin(s.handleListQueues))
						r.Route("/{name}", func(r chi.Router) {
							r.Get("/", s.requireAdmin(s.handleGetQueueInfo))
							r.Get("/peek", s.requireAdmin(s.handlePeekQueue))
							r.Post("/pause", s.requireAdmin(s.handlePauseQueue))
							r.Post("/resume", s.requireAdmin(s.handleResumeQueue))
							r.Post("/redrive", s.requireAdmin(s.handleRedriveQueue))
							r.Post("/purge", s.requireAdmin(s.handlePurgeQueue))
						})
					})
				})
			})
		})
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"registry": s.cfg.RegistryDB,
		"gateway":  s.cfg.GatewayURL,
	})
}

type createProjectReq struct {
	ID        string  `json:"id"`
	GitRemote *string `json:"git_remote"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	p, err := s.store.CreateProject(r.Context(), registry.CreateProjectInput{
		ID: req.ID, GitRemote: req.GitRemote,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func parsePageLimit(r *http.Request) int {
	limit := registry.DefaultPageLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > registry.MaxPageLimit {
		limit = registry.MaxPageLimit
	}
	return limit
}

func parseSince(r *http.Request) (*time.Time, error) {
	v := r.URL.Query().Get("since")
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, v)
	}
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	page, err := s.store.ListProjects(r.Context(), registry.ListProjectsOpts{
		Limit:  parsePageLimit(r),
		Cursor: r.URL.Query().Get("cursor"),
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
	})
	if err != nil {
		if strings.Contains(err.Error(), "cursor") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type item struct {
		ID            string    `json:"id"`
		ProdVersionID *string   `json:"prod_version_id"`
		VersionCount  int       `json:"version_count"`
		CreatedAt     time.Time `json:"created_at"`
	}
	var out []item
	for _, p := range page.Projects {
		out = append(out, item{
			ID: p.ID, ProdVersionID: p.ProdVersionID,
			VersionCount: p.VersionCount, CreatedAt: p.CreatedAt,
		})
	}
	resp := map[string]interface{}{"projects": out}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	p, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project_not_found"})
		return
	}
	versionCount, err := s.store.CountVersions(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]interface{}{
		"id": p.ID, "git_remote": p.GitRemote, "prod_version_id": p.ProdVersionID,
		"prod_url":   prodURL(s.cfg.GatewayURL, projectID),
		"created_at": p.CreatedAt, "version_count": versionCount,
		"versions_url": "/v1/projects/" + projectID + "/versions",
	}
	if r.URL.Query().Get("include") == "versions" {
		page, err := s.store.ListVersions(r.Context(), projectID, registry.ListVersionsOpts{
			Limit: parsePageLimit(r),
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		resp["versions"] = page.Versions
		if page.NextCursor != "" {
			resp["versions_next_cursor"] = page.NextCursor
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	p, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if p == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project_not_found"})
		return
	}
	since, err := parseSince(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_since"})
		return
	}
	page, err := s.store.ListVersions(r.Context(), projectID, registry.ListVersionsOpts{
		Limit:  parsePageLimit(r),
		Cursor: r.URL.Query().Get("cursor"),
		Status: r.URL.Query().Get("status"),
		Since:  since,
	})
	if err != nil {
		if strings.Contains(err.Error(), "cursor") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]interface{}{"versions": page.Versions}
	if page.NextCursor != "" {
		resp["next_cursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, resp)
}

type createVersionReq struct {
	ID              string            `json:"id"`
	ParentVersionID *string           `json:"parent_version_id"`
	GitRef          string            `json:"git_ref"`
	GitSHA          string            `json:"git_sha"`
	ArtifactURI     string            `json:"artifact_uri"` // ignored (TP-SEC-1)
	ArtifactDigest  string            `json:"artifact_digest"`
	Env             map[string]string `json:"env"`
}

func (s *Server) handleCreateVersion(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	var req createVersionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.ID == "" {
		req.ID = "v-" + time.Now().UTC().Format("20060102150405")
	}

	// Ensure project exists
	p, _ := s.store.GetProject(r.Context(), projectID)
	if p == nil {
		_, _ = s.store.CreateProject(r.Context(), registry.CreateProjectInput{ID: projectID})
		p, _ = s.store.GetProject(r.Context(), projectID)
	}

	// TP-SEC-3: reject fork prod for PR
	if err := orch.ValidateForkProd(req.ParentVersionID, p.ProdVersionID, req.GitRef); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	// TP-SEC-4: strip platform env keys
	req.Env = config.StripPlatformEnv(req.Env)

	ready, err := s.store.CountReadyVersions(r.Context(), projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	maxReady := maxReadyVersionsDefault
	if v := os.Getenv("CELLP_MAX_READY_VERSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxReady = n
		}
	}
	if ready >= maxReady {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "ready_version_limit_exceeded"})
		return
	}

	pending, err := s.store.CountPendingJobs(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	queueMax := QueueMax()
	if pending >= queueMax {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error":        "queue_full",
			"pending_jobs": pending,
			"queue_max":    queueMax,
		})
		return
	}

	// TP-SEC-1: server-side artifact URI
	artifactURI := artifact.ServerArtifactURI(s.cfg.ArtifactsBucket, projectID, req.ID)
	previewURL := strings.TrimRight(s.cfg.GatewayURL, "/") + "/" + projectID + "/" + req.ID + "/"

	v, err := s.store.CreateVersion(r.Context(), registry.CreateVersionInput{
		ID: req.ID, ProjectID: projectID, ParentVersionID: req.ParentVersionID,
		GitRef: req.GitRef, GitSHA: req.GitSHA,
		ArtifactURI: artifactURI, ArtifactDigest: req.ArtifactDigest,
		PreviewURL: previewURL, Env: req.Env,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if err := s.queue.Enqueue(r.Context(), &job.DeployJob{
		ProjectID: projectID, VersionID: req.ID, Step: registry.StatusFetching,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"id": v.ID, "project_id": v.ProjectID, "status": v.Status,
		"preview_url": previewURL,
		"poll_url":    "/v1/projects/" + projectID + "/versions/" + req.ID,
	})
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status != registry.StatusReady {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version not ready"})
		return
	}
	if err := s.orch.Promote(r.Context(), projectID, versionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "promoted",
		"prod_version_id": versionID,
		"prod_url":        prodURL(s.cfg.GatewayURL, projectID),
	})
}

func (s *Server) handleDestroy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	versionID := chi.URLParam(r, "versionID")
	v, _ := s.store.GetVersion(r.Context(), projectID, versionID)
	if v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version_not_found"})
		return
	}
	if v.Status == registry.StatusPending || v.Status == registry.StatusFetching ||
		v.Status == registry.StatusBranching || v.Status == registry.StatusPreparing ||
		v.Status == registry.StatusDeploying {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "version not ready for destroy"})
		return
	}
	if err := s.orch.Destroy(r.Context(), projectID, versionID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     registry.StatusDraining,
		"id":         versionID,
		"project_id": projectID,
	})
}

func (s *Server) requireDeploy(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+s.cfg.AdminToken && s.cfg.DeployToken != s.cfg.AdminToken {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if auth != "Bearer "+s.cfg.DeployToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+s.cfg.DeployToken && s.cfg.DeployToken != s.cfg.AdminToken {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if auth != "Bearer "+s.cfg.AdminToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func prodURL(gatewayURL, projectID string) string {
	return strings.TrimRight(gatewayURL, "/") + "/" + projectID + "/"
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
