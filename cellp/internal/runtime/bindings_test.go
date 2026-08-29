package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBindings(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		noFile   bool
		wantErr  bool
		wantNoW  bool
		check    func(t *testing.T, got *Bindings)
	}{
		{
			name:     "kv-only",
			filename: "wrangler.json",
			content: `{
  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }]
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.KV) != 1 || got.KV[0].Binding != "KV" || got.KV[0].ID != "ns-1" {
					t.Fatalf("kv = %+v", got.KV)
				}
				assertEmptyExcept(t, got, "kv")
			},
		},
		{
			name:     "queues-producers-consumers",
			filename: "wrangler.json",
			content: `{
  "queues": {
    "producers": [{ "binding": "TASKS", "queue": "tasks" }],
    "consumers": [
      { "queue": "tasks", "dead_letter_queue": "tasks-dlq" },
      { "queue": "events" }
    ]
  }
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.Queues) != 2 {
					t.Fatalf("queues = %+v", got.Queues)
				}
				tasks := got.Queues[0]
				if tasks.Name != "tasks" || tasks.Binding != "TASKS" || !tasks.Consumer {
					t.Fatalf("tasks = %+v", tasks)
				}
				if tasks.DeadLetterQueue == nil || *tasks.DeadLetterQueue != "tasks-dlq" {
					t.Fatalf("tasks dlq = %+v", tasks.DeadLetterQueue)
				}
				events := got.Queues[1]
				if events.Name != "events" || events.Binding != "" || !events.Consumer {
					t.Fatalf("events = %+v", events)
				}
				if events.DeadLetterQueue != nil {
					t.Fatalf("events dlq = %+v", events.DeadLetterQueue)
				}
			},
		},
		{
			name:     "workflows",
			filename: "wrangler.json",
			content: `{
  "workflows": [{ "binding": "WF", "name": "order-flow", "class_name": "OrderWorkflow" }]
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.Workflows) != 1 {
					t.Fatalf("workflows = %+v", got.Workflows)
				}
				wf := got.Workflows[0]
				if wf.Binding != "WF" || wf.Name != "order-flow" || wf.ClassName != "OrderWorkflow" {
					t.Fatalf("workflow = %+v", wf)
				}
				if len(got.KV) != 0 || len(got.R2) != 0 {
					t.Fatalf("kv/r2 should be empty: kv=%+v r2=%+v", got.KV, got.R2)
				}
			},
		},
		{
			name:     "r2",
			filename: "wrangler.json",
			content: `{
  "r2_buckets": [{ "binding": "FILES", "bucket_name": "files" }]
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.R2) != 1 || got.R2[0].BucketName != "files" {
					t.Fatalf("r2 = %+v", got.R2)
				}
				if got.R2[0].Binding != "FILES" {
					t.Fatalf("r2 binding = %+v", got.R2[0])
				}
			},
		},
		{
			name:     "crons",
			filename: "wrangler.json",
			content: `{
  "triggers": { "crons": ["0 * * * *", "*/5 * * * *"] }
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.Crons) != 2 || got.Crons[0] != "0 * * * *" || got.Crons[1] != "*/5 * * * *" {
					t.Fatalf("crons = %+v", got.Crons)
				}
			},
		},
		{
			name:     "empty",
			filename: "wrangler.json",
			content:  `{"name":"empty","main":"index.js"}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				assertAllEmpty(t, got)
			},
		},
		{
			name:     "invalid-jsonc",
			filename: "wrangler.jsonc",
			content:  `{ binding:`,
			wantErr:  true,
		},
		{
			name:     "jsonc-comments",
			filename: "wrangler.jsonc",
			content: `// comment
{
  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }]
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.KV) != 1 || got.KV[0].Binding != "KV" || got.KV[0].ID != "ns-1" {
					t.Fatalf("kv = %+v", got.KV)
				}
				assertEmptyExcept(t, got, "kv")
			},
		},
		{
			name:    "no-file",
			noFile:  true,
			wantErr: true,
			wantNoW: true,
		},
		{
			name:     "d1-passthrough",
			filename: "wrangler.json",
			content: `{
  "d1_databases": [
    { "binding": "DB", "database_name": "main", "database_id": "db-demo-v1" }
  ]
}`,
			check: func(t *testing.T, got *Bindings) {
				t.Helper()
				if len(got.D1) != 1 {
					t.Fatalf("d1 = %+v", got.D1)
				}
				d1 := got.D1[0]
				if d1.Binding != "DB" || d1.DatabaseName != "main" || d1.DatabaseID != "db-demo-v1" {
					t.Fatalf("d1 = %+v", d1)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if !tt.noFile {
				if err := os.WriteFile(filepath.Join(dir, tt.filename), []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ParseBindings(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantNoW {
					if !errors.Is(err, ErrNoWrangler) {
						t.Fatalf("errors.Is(ErrNoWrangler) = false; err = %v", err)
					}
				} else if errors.Is(err, ErrNoWrangler) {
					t.Fatalf("errors.Is(ErrNoWrangler) = true; err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, got)
		})
	}
}

func TestParseBindingsJSONNotNull(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(`{"name":"empty","main":"index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ParseBindings(dir)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{`"d1":[]`, `"kv":[]`, `"queues":[]`, `"workflows":[]`, `"r2":[]`, `"crons":[]`} {
		if !strings.Contains(s, want) {
			t.Fatalf("marshal missing %s: %s", want, s)
		}
	}
}

func assertAllEmpty(t *testing.T, got *Bindings) {
	t.Helper()
	if len(got.D1) != 0 || len(got.KV) != 0 || len(got.Queues) != 0 ||
		len(got.Workflows) != 0 || len(got.R2) != 0 || len(got.Crons) != 0 {
		t.Fatalf("expected empty arrays: %+v", got)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{`"d1":[]`, `"kv":[]`, `"queues":[]`, `"workflows":[]`, `"r2":[]`, `"crons":[]`} {
		if !strings.Contains(s, want) {
			t.Fatalf("marshal missing %s: %s", want, s)
		}
	}
}

func assertEmptyExcept(t *testing.T, got *Bindings, keep string) {
	t.Helper()
	if keep != "d1" && len(got.D1) != 0 {
		t.Fatalf("d1 = %+v", got.D1)
	}
	if keep != "kv" && len(got.KV) != 0 {
		t.Fatalf("kv = %+v", got.KV)
	}
	if keep != "queues" && len(got.Queues) != 0 {
		t.Fatalf("queues = %+v", got.Queues)
	}
	if keep != "workflows" && len(got.Workflows) != 0 {
		t.Fatalf("workflows = %+v", got.Workflows)
	}
	if keep != "r2" && len(got.R2) != 0 {
		t.Fatalf("r2 = %+v", got.R2)
	}
	if keep != "crons" && len(got.Crons) != 0 {
		t.Fatalf("crons = %+v", got.Crons)
	}
}
