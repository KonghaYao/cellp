package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cellp/cellp/internal/api"
	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func testAPI(t *testing.T, deployToken, adminToken string) (*api.Server, *registry.SQLiteStore, string) {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/api.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	artifactsDir := t.TempDir()
	cfg := config.Config{
		DeployToken:     deployToken,
		AdminToken:      adminToken,
		GatewayURL:      "http://127.0.0.1:8787",
		ArtifactsBucket: "cellp-artifacts",
		ArtifactsDir:    artifactsDir,
	}
	q := job.NewSQLiteQueue(store)
	bm := branch.New(t.TempDir(), store)
	rm := runtime.New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	as := &artifact.Store{Bucket: "cellp-artifacts", LocalDir: cfg.ArtifactsDir}
	o := orch.New(store, q, bm, rm, as, cfg)
	return api.NewServer(store, q, o, rm, cfg), store, artifactsDir
}

func TestHealth(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHealthDeepPendingJobs(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	_, _ = store.EnqueueJob(ctx, "demo", "v2", registry.StatusFetching)

	req := httptest.NewRequest(http.MethodGet, "/v1/health/deep", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Fatalf("status = %v", resp["status"])
	}
	checks, ok := resp["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks = %v", resp["checks"])
	}
	queue, ok := checks["queue"].(map[string]interface{})
	if !ok {
		t.Fatalf("queue = %v", checks["queue"])
	}
	if queue["pending_jobs"] != float64(2) {
		t.Fatalf("pending_jobs = %v", queue["pending_jobs"])
	}
	if queue["queue_max"] != float64(10000) {
		t.Fatalf("queue_max = %v", queue["queue_max"])
	}
}

func TestRuntimeRoutesSummary(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/runtime/routes", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Summary struct {
			ActiveRoutes int `json:"active_routes"`
		} `json:"summary"`
		Routes []map[string]interface{} `json:"routes"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.ActiveRoutes != 1 || len(resp.Routes) != 1 {
		t.Fatalf("summary = %+v routes = %+v", resp.Summary, resp.Routes)
	}
}

func TestQueueBackpressure503(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "2")
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	_, _ = store.EnqueueJob(ctx, "demo", "v2", registry.StatusFetching)

	body := bytes.NewBufferString(`{"id":"v3","git_ref":"main","git_sha":"abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "queue_full" {
		t.Fatalf("error = %v", resp["error"])
	}
}

func TestCreateVersion202(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	body := bytes.NewBufferString(`{"id":"v1","git_ref":"main","git_sha":"abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["poll_url"] == nil {
		t.Fatal("missing poll_url")
	}
	v, _ := store.GetVersion(context.Background(), "demo", "v1")
	if v == nil || v.Status != registry.StatusPending {
		t.Fatalf("version status = %v", v)
	}
}

func TestAuthSeparation(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy-token", "admin-token")
	defer store.Close()

	// wrong token → 401
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", bytes.NewBufferString(`{"id":"v1"}`))
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d", w.Code)
	}

	// admin on POST versions → 403
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", bytes.NewBufferString(`{"id":"v1"}`))
	req.Header.Set("Authorization", "Bearer admin-token")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin on versions: status = %d", w.Code)
	}

	// create project + version with deploy token first
	_, _ = store.CreateProject(context.Background(), registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(context.Background(), registry.CreateVersionInput{
		ID: "v1", ProjectID: "demo",
	})
	_ = store.UpdateVersionStatus(context.Background(), "demo", "v1", registry.StatusReady, nil)

	// deploy on promote → 403
	req = httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/promote", nil)
	req.Header.Set("Authorization", "Bearer deploy-token")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("deploy on promote: status = %d", w.Code)
	}
}

func TestRejectForkProd(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	prod := "v-prod"
	_, _ = store.CreateProject(context.Background(), registry.CreateProjectInput{ID: "demo"})
	_ = store.SetProdVersion(context.Background(), "demo", prod)

	body := bytes.NewBufferString(`{"id":"v-pr","parent_version_id":"v-prod","git_ref":"refs/pull/1/merge"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("fork prod: status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestPromoteConflict(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	_, _ = store.CreateProject(context.Background(), registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(context.Background(), registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/promote", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("promote pending: status = %d", w.Code)
	}
}

func TestReadyVersionLimitExceeded429(t *testing.T) {
	t.Setenv("CELLP_MAX_READY_VERSIONS", "5")
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	_, _ = store.CreateProject(context.Background(), registry.CreateProjectInput{ID: "demo"})
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("v%d", i)
		_, _ = store.CreateVersion(context.Background(), registry.CreateVersionInput{ID: id, ProjectID: "demo"})
		_ = store.UpdateVersionStatus(context.Background(), "demo", id, registry.StatusReady, nil)
	}

	body := bytes.NewBufferString(`{"id":"v6","git_ref":"main","git_sha":"abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "ready_version_limit_exceeded" {
		t.Fatalf("error = %q", resp["error"])
	}
}

func TestIgnoreClientArtifactURI(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	body := bytes.NewBufferString(`{"id":"v1","artifact_uri":"http://169.254.169.254/"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions", body)
	req.Header.Set("Authorization", "Bearer deploy")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d", w.Code)
	}
	v, _ := store.GetVersion(context.Background(), "demo", "v1")
	if v == nil || v.ArtifactURI != "s3://cellp-artifacts/demo/v1/" {
		t.Fatalf("artifact_uri = %q", v.ArtifactURI)
	}
}

func TestListProjectsPaginationAPI(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: fmt.Sprintf("p%d", i)})
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects?limit=2", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var page1 struct {
		Projects   []map[string]interface{} `json:"projects"`
		NextCursor string                   `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page1)
	if len(page1.Projects) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1: %+v", page1)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects?limit=2&cursor="+page1.NextCursor, nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var page2 struct {
		Projects   []map[string]interface{} `json:"projects"`
		NextCursor string                   `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page2)
	if len(page2.Projects) != 1 {
		t.Fatalf("page2: %+v", page2)
	}
}

func TestListProjectsQueryAPI(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	for _, id := range []string{"alpha-app", "beta-app", "cellp-dashboard", "demo-app"} {
		_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: id})
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects?q=cellp", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Projects) != 1 {
		t.Fatalf("expected 1 match, got %+v", resp.Projects)
	}
	if resp.Projects[0]["id"] != "cellp-dashboard" {
		t.Fatalf("unexpected id: %+v", resp.Projects[0])
	}
}

func TestGetProjectVersionsURL(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["versions_url"] != "/v1/projects/demo/versions" {
		t.Fatalf("versions_url = %v", resp["versions_url"])
	}
	if resp["version_count"] != float64(1) {
		t.Fatalf("version_count = %v", resp["version_count"])
	}
	if _, ok := resp["versions"]; ok {
		t.Fatal("expected no embedded versions by default")
	}
}

func TestGetProjectIncludeVersions(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo?include=versions", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Versions []map[string]interface{} `json:"versions"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Versions) != 1 {
		t.Fatalf("versions = %+v", resp.Versions)
	}
}

func TestListVersionsAPI(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("v%d", i)
		_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: id, ProjectID: "demo"})
		_ = store.UpdateVersionStatus(ctx, "demo", id, registry.StatusReady, nil)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions?limit=2&status=ready", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var page1 struct {
		Versions   []map[string]interface{} `json:"versions"`
		NextCursor string                   `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page1)
	if len(page1.Versions) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1: %+v", page1)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions?limit=2&cursor="+page1.NextCursor, nil)
	req.Header.Set("Authorization", "Bearer admin")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var page2 struct {
		Versions []map[string]interface{} `json:"versions"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page2)
	if len(page2.Versions) != 1 {
		t.Fatalf("page2: %+v", page2)
	}
}

func TestInvalidCursorAPI(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects?cursor=bad", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}
