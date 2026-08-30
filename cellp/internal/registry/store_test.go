package registry

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryCRUD(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	git := "git@example/demo"
	if _, err := s.CreateProject(ctx, CreateProjectInput{ID: "demo", GitRemote: &git}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVersion(ctx, CreateVersionInput{
		ID: "v1", ProjectID: "demo",
		ArtifactURI: "s3://cellp-artifacts/demo/v1/",
		Env:         map[string]string{"GREETING": "hi"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetVersion(ctx, "demo", "v1")
	if err != nil || got == nil || got.Status != StatusPending {
		t.Fatalf("get version: %+v err=%v", got, err)
	}
	env, err := s.GetVersionEnv(ctx, "demo", "v1")
	if err != nil || env["GREETING"] != "hi" {
		t.Fatalf("env = %#v err=%v", env, err)
	}
	if err := s.UpdateVersionStatus(ctx, "demo", "v1", StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProdVersionCAS(ctx, "demo", "", "v1"); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProject(ctx, "demo")
	if err != nil || p == nil || p.ProdVersionID == nil || *p.ProdVersionID != "v1" {
		t.Fatalf("prod pointer: %+v", p)
	}
}

func TestWALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal.sqlite")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("expected wal got %s", mode)
	}
}

func TestDeleteRoute(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "routes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = s.SetRoute(ctx, Route{ProjectID: "demo", VersionID: "v1", Active: true, UpstreamHost: "127.0.0.1", UpstreamPort: 8792})
	if err := s.DeleteRoute(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	r, _ := s.GetRoute(ctx, "demo", "v1")
	if r != nil {
		t.Fatalf("expected nil route got %+v", r)
	}
}

func TestCountPendingJobs(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "pending.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if n, err := s.CountPendingJobs(ctx); err != nil || n != 0 {
		t.Fatalf("empty pending = %d err=%v", n, err)
	}

	_, _ = s.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	_, _ = s.EnqueueJob(ctx, "demo", "v2", StatusFetching)
	j, _ := s.ClaimJob(ctx, "w1", time.Minute)
	if j == nil {
		t.Fatal("expected claim")
	}

	if n, err := s.CountPendingJobs(ctx); err != nil || n != 1 {
		t.Fatalf("pending after claim = %d err=%v", n, err)
	}
}

func TestClaimJobReclaimsExpiredLease(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "jobs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	j, err := s.EnqueueJob(ctx, "demo", "v1", StatusFetching)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimJob(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != j.ID {
		t.Fatalf("first claim: %+v", claimed)
	}

	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	_, err = s.db.Exec(`UPDATE jobs SET lease_until = ? WHERE id = ?`, expired, j.ID)
	if err != nil {
		t.Fatal(err)
	}

	reclaimed, err := s.ClaimJob(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed == nil || reclaimed.ID != j.ID {
		t.Fatalf("reclaim expired lease: %+v", reclaimed)
	}
	if reclaimed.LeaseUntil == nil || !reclaimed.LeaseUntil.After(time.Now().UTC()) {
		t.Fatalf("expected renewed lease, got %+v", reclaimed.LeaseUntil)
	}
}

func TestPurgeCompletedJobs(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "purge-jobs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	old := time.Now().UTC().Add(-8 * 24 * time.Hour).Format(time.RFC3339Nano)
	recent := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)

	for _, row := range []struct {
		id, status, updated string
	}{
		{"j-old-completed", "completed", old},
		{"j-old-failed", "failed", old},
		{"j-recent-completed", "completed", recent},
		{"j-pending", "pending", old},
		{"j-claimed", "claimed", old},
	} {
		_, err := s.db.Exec(`INSERT INTO jobs (id, project_id, version_id, step, status, updated_at)
			VALUES (?, 'demo', 'v1', 'fetching', ?, ?)`, row.id, row.status, row.updated)
		if err != nil {
			t.Fatal(err)
		}
	}

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	n, err := s.PurgeCompletedJobs(ctx, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 purged jobs, got %d", n)
	}

	var remaining int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 3 {
		t.Fatalf("expected 3 remaining jobs, got %d", remaining)
	}
}

func TestPurgeDestroyedVersionsKeepsActiveRoutesAndProd(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "purge-versions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)

	seedDestroyed := func(id string) {
		v, err := s.CreateVersion(ctx, CreateVersionInput{ID: id, ProjectID: "demo"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.UpdateVersionStatus(ctx, "demo", v.ID, StatusDestroyed, nil); err != nil {
			t.Fatal(err)
		}
		now := old.Format(time.RFC3339Nano)
		_, err = s.db.Exec(`UPDATE versions SET updated_at = ? WHERE project_id = ? AND id = ?`, now, "demo", id)
		if err != nil {
			t.Fatal(err)
		}
	}
	seedDestroyed("v-stale")
	seedDestroyed("v-active-route")
	seedDestroyed("v-prod")

	_ = s.SetRoute(ctx, Route{ProjectID: "demo", VersionID: "v-active-route", Active: true, UpstreamHost: "127.0.0.1", UpstreamPort: 8792})
	_ = s.SetRoute(ctx, Route{ProjectID: "demo", VersionID: "v-stale", Active: false, UpstreamHost: "127.0.0.1", UpstreamPort: 8793})
	_ = s.SetProdVersion(ctx, "demo", "v-prod")

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	n, err := s.PurgeDestroyedVersions(ctx, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged version (v-stale), got %d", n)
	}

	for _, id := range []string{"v-active-route", "v-prod"} {
		v, err := s.GetVersion(ctx, "demo", id)
		if err != nil || v == nil {
			t.Fatalf("version %s should remain: %+v err=%v", id, v, err)
		}
	}
	stale, _ := s.GetVersion(ctx, "demo", "v-stale")
	if stale != nil {
		t.Fatalf("expected v-stale purged, got %+v", stale)
	}
	r, _ := s.GetRoute(ctx, "demo", "v-stale")
	if r != nil {
		t.Fatalf("inactive route for purged version should be removed, got %+v", r)
	}
	active, _ := s.GetRoute(ctx, "demo", "v-active-route")
	if active == nil || !active.Active {
		t.Fatalf("active route must remain: %+v", active)
	}
}
