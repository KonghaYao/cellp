package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cellp/cellp/internal/runtime"
)

const workflowCellFixture = `{"scope":"__Workflow.full:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","class":"__Workflow.full","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reserved":true}
{"scope":"__D1Database:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","class":"__D1Database","id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","reserved":true}
{"scope":"Room:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","class":"Room","id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","reserved":false}
`

func TestListWorkflowsFromWrangler(t *testing.T) {
	argsLog := installFakeCellList(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Workflows []struct {
			Binding      string `json:"binding"`
			WorkflowName string `json:"workflow_name"`
			ClassName    string `json:"class_name"`
		} `json:"workflows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Workflows) != 1 {
		t.Fatalf("workflows = %+v", resp.Workflows)
	}
	wf := resp.Workflows[0]
	if wf.Binding != "WF" || wf.WorkflowName != "wf" || wf.ClassName != "WfClass" {
		t.Fatalf("workflow = %+v", wf)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatal("GET /workflows must not invoke celld")
	}
}

func TestListWorkflowsEmpty(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{"name":"x"}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	arr, ok := resp["workflows"].([]interface{})
	if !ok || len(arr) != 0 {
		t.Fatalf("workflows = %v", resp["workflows"])
	}
}

func TestListWorkflowsNoWrangler(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{"name":"x"}`)
	_ = os.Remove(filepath.Join(artifactsDir, "demo", "v1", "wrangler.json"))

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "bindings_not_found")
}

func TestListWorkflowInstancesSingle(t *testing.T) {
	installFakeCellList(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows/wf/instances", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp runtime.WorkflowInstances
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Filter != "workflow" || resp.Limitation != nil {
		t.Fatalf("filter/limitation = %q %v", resp.Filter, resp.Limitation)
	}
	if len(resp.Instances) != 1 {
		t.Fatalf("instances = %+v", resp.Instances)
	}
	if resp.Instances[0].Class != "__Workflow.full" || strings.Contains(resp.Instances[0].Class, "D1") {
		t.Fatalf("instance = %+v", resp.Instances[0])
	}
}

func TestListWorkflowInstancesMultiFallback(t *testing.T) {
	installFakeCellList(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [
    { "binding": "A", "name": "wf-a", "class_name": "WfA" },
    { "binding": "B", "name": "wf-b", "class_name": "WfB" }
  ]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows/wf-a/instances", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp runtime.WorkflowInstances
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Filter != "script" {
		t.Fatalf("filter = %q", resp.Filter)
	}
	if resp.Limitation == nil || *resp.Limitation == "" {
		t.Fatal("limitation should be set")
	}
	if len(resp.Instances) != 1 {
		t.Fatalf("instances = %+v", resp.Instances)
	}
	if len(resp.WranglerWorkflows) != 2 {
		t.Fatalf("wrangler_workflows = %v", resp.WranglerWorkflows)
	}
}

func TestListWorkflowInstancesUnknownName(t *testing.T) {
	argsLog := installFakeCellList(t)
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows/nope/instances", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "workflow_not_found")
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatal("unknown name must not invoke celld")
	}
}

func TestListWorkflowInstancesNoCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows/wf/instances", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var resp runtime.WorkflowInstances
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Instances == nil || len(resp.Instances) != 0 {
		t.Fatalf("instances = %#v", resp.Instances)
	}
}

func TestListWorkflowInstancesCelldFailed(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/v1/workflows/wf/instances", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "celld_cell_list_failed")
}

func TestWorkflowsReadOnlyAndNoCronR2Routes(t *testing.T) {
	srv, store, artifactsDir := testAPI(t, "deploy", "admin")
	defer store.Close()
	setupReadyVersion(t, store, artifactsDir, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }],
  "triggers": { "crons": ["0 * * * *"] },
  "r2_buckets": [{ "binding": "FILES", "bucket_name": "files" }]
}`)

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/projects/demo/versions/v1/workflows/wf/pause"},
		{http.MethodPost, "/v1/projects/demo/versions/v1/workflows/wf/resume"},
		{http.MethodPost, "/v1/projects/demo/versions/v1/workflows/wf/restart"},
		{http.MethodDelete, "/v1/projects/demo/versions/v1/workflows/wf"},
		{http.MethodGet, "/v1/projects/demo/versions/v1/crons"},
		{http.MethodGet, "/v1/projects/demo/versions/v1/r2/files/objects"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer admin")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code == http.StatusOK || w.Code == http.StatusCreated || w.Code == http.StatusAccepted {
			t.Fatalf("%s %s status = %d (want non-2xx)", tc.method, tc.path, w.Code)
		}
	}
}

func TestListWorkflowsVersionNotReady(t *testing.T) {
	srv, store, _ := testAPI(t, "deploy", "admin")
	defer store.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/demo/versions/missing/workflows", nil)
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertAPIError(t, w, "version_not_found")
}

func installFakeCellList(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(root, "celld-args.log")
	fixturePath := filepath.Join(root, "fixture.ndjson")
	if err := os.WriteFile(fixturePath, []byte(workflowCellFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" >> " + argsLog + "\n" +
		"has_cell=0\n" +
		"has_list=0\n" +
		"for a in \"$@\"; do\n" +
		"  [ \"$a\" = \"cell\" ] && has_cell=1\n" +
		"  [ \"$a\" = \"list\" ] && has_list=1\n" +
		"done\n" +
		"if [ \"$has_cell\" = 1 ] && [ \"$has_list\" = 1 ]; then\n" +
		"  /bin/cat " + fixturePath + "\n" +
		"  echo note >&2\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return argsLog
}
