package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestListProjectsAndVersionEnv(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "misc.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	page, err := s.ListProjects(ctx, ListProjectsOpts{Limit: 10})
	if err != nil || len(page.Projects) != 1 {
		t.Fatalf("projects=%d err=%v", len(page.Projects), err)
	}
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := s.SetVersionEnv(ctx, "demo", "v1", map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	env, err := s.GetVersionEnv(ctx, "demo", "v1")
	if err != nil || env["A"] != "1" {
		t.Fatalf("env=%v err=%v", env, err)
	}
}
