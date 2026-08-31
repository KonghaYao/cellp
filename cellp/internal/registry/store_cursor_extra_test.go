package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestListProjectsInvalidCursor(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cur.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.ListProjects(context.Background(), ListProjectsOpts{
		Limit:  10,
		Cursor: "not-valid-cursor",
	})
	if err == nil {
		t.Fatal("expected cursor error")
	}
}
