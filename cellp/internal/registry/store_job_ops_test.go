package registry

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPingAndRouteActive(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ops.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = s.SetRoute(ctx, Route{ProjectID: "demo", VersionID: "v1", Active: true, UpstreamHost: "127.0.0.1", UpstreamPort: 8792})
	_ = s.SetRouteActive(ctx, "demo", "v1", false)
	route, _ := s.GetRoute(ctx, "demo", "v1")
	if route == nil || route.Active {
		t.Fatalf("route=%+v", route)
	}
	active, err := s.ListActiveRoutes(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active=%+v", active)
	}
}

func TestJobCompleteFailUpdateStep(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "jobops.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	j, err := s.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimJob(ctx, "w1", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	if err := s.UpdateJobStep(ctx, j.ID, StatusDeploying); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}

	j2, _ := s.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	_, _ = s.ClaimJob(ctx, "w1", time.Minute)
	if err := s.FailJob(ctx, j2.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCountReadyVersions(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ready.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"})
	_ = s.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil)
	n, err := s.CountReadyVersions(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ready=%d", n)
	}
}

func TestNormalizePageLimit(t *testing.T) {
	if normalizePageLimit(0) != DefaultPageLimit {
		t.Fatal("zero")
	}
	if normalizePageLimit(MaxPageLimit+100) != MaxPageLimit {
		t.Fatal("cap")
	}
}
