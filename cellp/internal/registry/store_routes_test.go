package registry

import (
	"context"
	"path/filepath"
	"testing"
)

func TestListAllActiveRoutesAndReady(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "routes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = s.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil)
	_ = s.SetRoute(ctx, Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})
	routes, err := s.ListAllActiveRoutes(ctx)
	if err != nil || len(routes) != 1 {
		t.Fatalf("routes=%d err=%v", len(routes), err)
	}
	ready, err := s.ListAllReadyVersions(ctx)
	if err != nil || len(ready) < 1 {
		t.Fatalf("ready=%d err=%v", len(ready), err)
	}
	n, err := s.CountChildVersions(ctx, "demo", "v-parent")
	if err != nil || n != 0 {
		t.Fatalf("children=%d err=%v", n, err)
	}
}

func TestTouchLastAccess(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "touch.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := s.TouchLastAccess(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	v, _ := s.GetVersion(ctx, "demo", "v1")
	if v == nil || v.LastAccessAt == nil {
		t.Fatal("last access not set")
	}
}
