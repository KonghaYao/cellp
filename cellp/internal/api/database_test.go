package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/registry"
)

func installFakeCelld(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
if [ "$1" = "d1" ] && [ "$2" = "execute" ]; then
  shift 2
  SQL=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --command)
        SQL="$2"
        shift 2
        ;;
      --json)
        shift
        ;;
      --bucket|--endpoint|--region)
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done
  case "$SQL" in
    *sqlite_master*)
      echo '{"name":"users","type":"table"}'
      echo '{"name":"posts","type":"table"}'
      ;;
    *PRAGMA\ table_info*)
      echo '{"name":"id","type":"INTEGER"}'
      echo '{"name":"email","type":"TEXT"}'
      ;;
    *COUNT\(\*\)*)
      echo '{"cnt":42}'
      ;;
    *SELECT\ \*\ FROM\ \"users\"*)
      echo '{"id":1,"email":"alice@example.com"}'
      echo '{"id":2,"email":"bob@example.com"}'
      ;;
    *SELECT\ 1\ FROM\ sqlite_master*)
      echo '{"1":1}'
      ;;
    *)
      ;;
  esac
  echo "Executed 1 statement(s) in 5ms" >&2
  exit 0
fi
exit 0
`
	celld := filepath.Join(bin, "celld")
	if err := os.WriteFile(celld, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

func setupDatabaseVersion(t *testing.T, store *registry.SQLiteStore, artifactsDir string) {
	t.Helper()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	parent := "v-parent"
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: parent, ProjectID: "demo",
	})
	_ = store.UpdateVersionStatus(ctx, "demo", parent, registry.StatusReady, nil)

	versionID := "v1"
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{
		ID: versionID, ProjectID: "demo", ParentVersionID: &parent,
	})
	_ = store.UpdateVersionStatus(ctx, "demo", versionID, registry.StatusReady, nil)

	projectDir := filepath.Join(artifactsDir, "demo", versionID)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrangler := `{
  "d1_databases": [
    { "binding": "DB", "database_name": "main", "database_id": "db-demo-v1" }
  ]
}`
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}

	parentDir := filepath.Join(artifactsDir, "demo", parent)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentDir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseMetadata(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["database_name"] != "main" {
		t.Fatalf("database_name = %v", resp["database_name"])
	}
	if resp["database_id"] != "db-demo-v1" {
		t.Fatalf("database_id = %v", resp["database_id"])
	}
	if resp["branch_method"] != "d1_branch" {
		t.Fatalf("branch_method = %v", resp["branch_method"])
	}
}

func TestDatabaseTables(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Tables []struct {
			Name     string `json:"name"`
			RowCount int64  `json:"row_count"`
		} `json:"tables"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Tables) != 2 {
		t.Fatalf("tables = %+v", resp.Tables)
	}
	if resp.Tables[0].RowCount != 42 {
		t.Fatalf("row_count = %d", resp.Tables[0].RowCount)
	}
}

func TestDatabaseTableRows(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables/users/rows?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Total int64                    `json:"total"`
		Rows  []map[string]interface{} `json:"rows"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 42 {
		t.Fatalf("total = %d", resp.Total)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("rows = %+v", resp.Rows)
	}
}

func TestDatabaseQuery(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	body := bytes.NewBufferString(`{"sql":"SELECT * FROM users LIMIT 5"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/versions/v1/database/query", body)
	req.Header.Set("Authorization", "Bearer admin")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		DurationMS int64 `json:"duration_ms"`
		Rows       []map[string]interface{}
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.DurationMS != 5 {
		t.Fatalf("duration_ms = %d", resp.DurationMS)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("rows = %+v", resp.Rows)
	}
}

func TestDatabaseVersionNotReady(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseNoD1(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)

	projectDir := filepath.Join(artifactsDir, "demo", "v1")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestDatabaseInvalidTableName(t *testing.T) {
	installFakeCelld(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupDatabaseVersion(t, store, artifactsDir)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/database/tables/bad%21drop/rows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}
