package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeclaredQueueNamesDedup(t *testing.T) {
	dir := t.TempDir()
	wrangler := `{
	  "queues": {
	    "producers": [{ "binding": "TASKS", "queue": "tasks" }],
	    "consumers": [
	      { "queue": "tasks", "dead_letter_queue": "tasks-dlq" },
	      { "queue": "events", "dead_letter_queue": "events-dlq" }
	    ]
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "wrangler.json"), []byte(wrangler), 0o644); err != nil {
		t.Fatal(err)
	}
	names := DeclaredQueueNames(dir)
	want := map[string]bool{"tasks": true, "tasks-dlq": true, "events": true, "events-dlq": true}
	if len(names) != len(want) {
		t.Fatalf("names = %v want keys %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected queue %q in %v", n, names)
		}
	}
}

func TestDeclaredQueueNamesEmptyDir(t *testing.T) {
	if got := DeclaredQueueNames(t.TempDir()); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
