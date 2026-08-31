package orch_test

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/artifact"
	"github.com/cellp/cellp/internal/branch"
	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/job"
	"github.com/cellp/cellp/internal/orch"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func newTestOrch(t *testing.T) (*orch.Orchestrator, *registry.SQLiteStore) {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/orch.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		GatewayURL:   "http://127.0.0.1:8787",
		ArtifactsDir: t.TempDir(),
	}
	q := job.NewSQLiteQueue(store)
	bm := branch.New(t.TempDir(), store)
	rm := runtime.New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")
	as := &artifact.Store{Bucket: "cellp-artifacts", LocalDir: cfg.ArtifactsDir}
	return orch.New(store, q, bm, rm, as, cfg), store
}

func TestArchiveIdleReaper(t *testing.T) {
	t.Setenv("CELLP_ARCHIVE_GRACE", "0")
	t.Setenv("CELLP_ARCHIVE_IDLE", "1ms")
	t.Setenv("CELLP_ROLLBACK_KEEP", "0")

	o, store := newTestOrch(t)
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8793,
	})
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := store.ExecTestSQL(ctx, `UPDATE versions SET last_access_at = ?, ready_at = ? WHERE project_id = ? AND id = ?`,
		old, old, "demo", "v1"); err != nil {
		t.Fatal(err)
	}

	cfg := orch.LoadArchiveConfig()
	n, err := o.RunArchiveReaperOnce(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("archived = %d, want 1", n)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusArchived {
		t.Fatalf("status = %v", v)
	}
	route, _ := store.GetRoute(ctx, "demo", "v1")
	if route != nil && route.Active {
		t.Fatal("route should be inactive after archive")
	}
}

func TestMayArchiveTable(t *testing.T) {
	cfg := orch.ArchiveConfig{
		Grace:        15 * time.Minute,
		Idle:         45 * time.Minute,
		RollbackKeep: 60 * time.Minute,
	}
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	readyAt := now.Add(-2 * time.Hour)
	oldAccess := now.Add(-2 * time.Hour)
	prod := "v-prod"
	prev := "v-prev"

	cases := []struct {
		name string
		proj *registry.Project
		v    *registry.Version
		want bool
	}{
		{
			name: "idle ready non-prod",
			proj: &registry.Project{ID: "demo"},
			v:    &registry.Version{ID: "v1", Status: registry.StatusReady, ReadyAt: &readyAt, LastAccessAt: &oldAccess},
			want: true,
		},
		{
			name: "never archive prod",
			proj: &registry.Project{ID: "demo", ProdVersionID: &prod},
			v:    &registry.Version{ID: prod, Status: registry.StatusReady, ReadyAt: &readyAt, LastAccessAt: &oldAccess},
			want: false,
		},
		{
			name: "pinned",
			proj: &registry.Project{ID: "demo"},
			v:    &registry.Version{ID: "v1", Status: registry.StatusReady, Pinned: true, ReadyAt: &readyAt, LastAccessAt: &oldAccess},
			want: false,
		},
		{
			name: "within grace",
			proj: &registry.Project{ID: "demo"},
			v: func() *registry.Version {
				recent := now.Add(-time.Minute)
				return &registry.Version{ID: "v1", Status: registry.StatusReady, ReadyAt: &recent, LastAccessAt: &recent}
			}(),
			want: false,
		},
		{
			name: "previous prod within rollback keep",
			proj: func() *registry.Project {
				at := now.Add(-10 * time.Minute)
				return &registry.Project{ID: "demo", ProdVersionID: &prod, PreviousProdVersionID: &prev, PreviousProdAt: &at}
			}(),
			v:    &registry.Version{ID: prev, Status: registry.StatusReady, ReadyAt: &readyAt, LastAccessAt: &oldAccess},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := orch.MayArchive(tc.proj, tc.v, cfg, now)
			if got != tc.want {
				t.Fatalf("MayArchive = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWakeRestoresReady(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store := newTestOrch(t)
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusArchived, nil)

	if err := o.Wake(ctx, "demo", "v1"); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusReady {
		t.Fatalf("status = %v", v)
	}
}

func TestArchiveReadyVersion(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	o, store := newTestOrch(t)
	defer store.Close()
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_ = store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil)
	_ = store.SetRoute(ctx, registry.Route{
		ProjectID: "demo", VersionID: "v1", Active: true,
		UpstreamHost: "127.0.0.1", UpstreamPort: 8792,
	})

	if err := o.Archive(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	v, _ := store.GetVersion(ctx, "demo", "v1")
	if v == nil || v.Status != registry.StatusArchived {
		t.Fatalf("status=%v", v)
	}
	route, _ := store.GetRoute(ctx, "demo", "v1")
	if route != nil && route.Active {
		t.Fatal("route should be inactive")
	}
}
