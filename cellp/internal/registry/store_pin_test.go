package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPinVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "pin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	if err := s.SetVersionPinned(ctx, "demo", "v1", true); err != nil {
		t.Fatal(err)
	}
	v, _ := s.GetVersion(ctx, "demo", "v1")
	if v == nil || !v.Pinned {
		t.Fatalf("pinned=%v", v)
	}
	if err := s.SetVersionPinned(ctx, "demo", "v1", false); err != nil {
		t.Fatal(err)
	}
}

func TestListVersionsByProject(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "list.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	for _, id := range []string{"v1", "v2"} {
		_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: id, ProjectID: "demo"})
	}
	page, err := s.ListVersions(ctx, "demo", ListVersionsOpts{Limit: 10})
	if err != nil || len(page.Versions) < 2 {
		t.Fatalf("versions=%d err=%v", len(page.Versions), err)
	}
}
