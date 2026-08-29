package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cellListFixture = `{"scope":"__Workflow.full:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","class":"__Workflow.full","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reserved":true}
{"scope":"__D1Database:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","class":"__D1Database","id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","reserved":true}
{"scope":"Room:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","class":"Room","id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","reserved":false}
`

func TestParseCellListNDJSONFiltersWorkflow(t *testing.T) {
	got, err := parseCellListNDJSON([]byte(cellListFixture + "\n{\"scope\":\"orphan\",\"class\":null,\"id\":\"dd\",\"reserved\":true}\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d want 1: %+v", len(got), got)
	}
	if got[0].Class != "__Workflow.full" || !got[0].Reserved {
		t.Fatalf("record = %+v", got[0])
	}
	if got[0].ID != strings.Repeat("a", 64) {
		t.Fatalf("id = %q", got[0].ID)
	}
}

func TestParseCellListNDJSONEmpty(t *testing.T) {
	got, err := parseCellListNDJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got = %#v", got)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]" {
		t.Fatalf("marshal = %s", body)
	}
}

func TestListWorkflowInstancesArgvAndFilter(t *testing.T) {
	root, argsLog := installFakeCellList(t)
	projectDir := writeWrangler(t, root, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	got, err := m.ListWorkflowInstances(context.Background(), "demo", "v1", projectDir, "wf")
	if err != nil {
		t.Fatalf("ListWorkflowInstances: %v", err)
	}
	if got.Filter != "workflow" {
		t.Fatalf("filter = %q", got.Filter)
	}
	if got.Limitation != nil {
		t.Fatalf("limitation = %v want nil", got.Limitation)
	}
	if len(got.Instances) != 1 || got.Instances[0].Class != "__Workflow.full" {
		t.Fatalf("instances = %+v", got.Instances)
	}
	if got.ScriptName != "full" || got.Binding != "WF" || got.WorkflowName != "wf" {
		t.Fatalf("meta = %+v", got)
	}
	if len(got.WranglerWorkflows) != 1 || got.WranglerWorkflows[0] != "wf" {
		t.Fatalf("wrangler_workflows = %v", got.WranglerWorkflows)
	}

	args := readArgv(t, argsLog)
	assertContains(t, args, "cell", "list", "--json", "--all", "--bucket", "s3://cellp-celld/demo/v1",
		"--endpoint", "http://127.0.0.1:9000", "--region", "us-east-1", "__Workflow.full")
}

func TestListWorkflowInstancesMultiFallback(t *testing.T) {
	root, _ := installFakeCellList(t)
	projectDir := writeWrangler(t, root, `{
  "name": "full",
  "workflows": [
    { "binding": "A", "name": "wf-a", "class_name": "WfA" },
    { "binding": "B", "name": "wf-b", "class_name": "WfB" }
  ]
}`)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	got, err := m.ListWorkflowInstances(context.Background(), "demo", "v1", projectDir, "wf-a")
	if err != nil {
		t.Fatalf("ListWorkflowInstances: %v", err)
	}
	if got.Filter != "script" {
		t.Fatalf("filter = %q", got.Filter)
	}
	if got.Limitation == nil || *got.Limitation != WorkflowFilterLimitation {
		t.Fatalf("limitation = %v", got.Limitation)
	}
	if len(got.Instances) != 1 {
		t.Fatalf("instances = %+v", got.Instances)
	}
	if len(got.WranglerWorkflows) != 2 {
		t.Fatalf("wrangler_workflows = %v", got.WranglerWorkflows)
	}
	joined := strings.Join(got.WranglerWorkflows, ",")
	if !strings.Contains(joined, "wf-a") || !strings.Contains(joined, "wf-b") {
		t.Fatalf("wrangler_workflows = %v", got.WranglerWorkflows)
	}
}

func TestListWorkflowInstancesUnknownNameSkipsCelld(t *testing.T) {
	root, argsLog := installFakeCellList(t)
	projectDir := writeWrangler(t, root, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	_, err := m.ListWorkflowInstances(context.Background(), "demo", "v1", projectDir, "missing")
	if !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatal("celld should not run for unknown workflow name")
	}
}

func TestListWorkflowInstancesNoCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	projectDir := writeWrangler(t, root, `{
  "name": "full",
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	got, err := m.ListWorkflowInstances(context.Background(), "demo", "v1", projectDir, "wf")
	if err != nil {
		t.Fatalf("ListWorkflowInstances: %v", err)
	}
	if got.Filter != "workflow" || got.Limitation != nil {
		t.Fatalf("filter/limitation = %q %v", got.Filter, got.Limitation)
	}
	if got.Instances == nil || len(got.Instances) != 0 {
		t.Fatalf("instances = %#v", got.Instances)
	}
}

func TestListWorkflowInstancesEmptyWorkerNameOmitsClass(t *testing.T) {
	root, argsLog := installFakeCellList(t)
	projectDir := writeWrangler(t, root, `{
  "workflows": [{ "binding": "WF", "name": "wf", "class_name": "WfClass" }]
}`)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	got, err := m.ListWorkflowInstances(context.Background(), "demo", "v1", projectDir, "WF")
	if err != nil {
		t.Fatal(err)
	}
	if got.ScriptName != "" {
		t.Fatalf("script_name = %q", got.ScriptName)
	}
	args := readArgv(t, argsLog)
	for _, a := range args {
		if strings.HasPrefix(a, "__Workflow.") {
			t.Fatalf("unexpected CLASS %q in %q", a, args)
		}
	}
	assertContains(t, args, "cell", "list", "--json", "--all")
}

func TestListCellsNonZeroExit(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho fail >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	m := New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	_, err := m.ListCells(context.Background(), "demo", "v1", "__Workflow.full")
	if !errors.Is(err, ErrCelldCellListFailed) {
		t.Fatalf("err = %v", err)
	}
}

func installFakeCellList(t *testing.T) (root, argsLog string) {
	t.Helper()
	root = t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog = filepath.Join(root, "celld-args.log")
	fixturePath := filepath.Join(root, "fixture.ndjson")
	if err := os.WriteFile(fixturePath, []byte(cellListFixture), 0o644); err != nil {
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
	return root, argsLog
}

func writeWrangler(t *testing.T, root, content string) string {
	t.Helper()
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "wrangler.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return projectDir
}

func assertContains(t *testing.T, args []string, want ...string) {
	t.Helper()
	have := make(map[string]bool, len(args))
	for _, a := range args {
		have[a] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Fatalf("argv missing %q: %q", w, args)
		}
	}
}
