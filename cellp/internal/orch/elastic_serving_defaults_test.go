package orch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

func TestEnsureDefaultElasticServingPolicy(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(filepath.Join(t.TempDir(), "defaults.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	o := &Orchestrator{store: store, cfg: config.Config{Serving: config.LoadServingDefaults()}}

	if err := o.ensureDefaultElasticServingPolicy(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	pol, err := store.GetServingPolicy(ctx, "demo", "v1")
	if err != nil || pol == nil {
		t.Fatalf("policy: %+v err=%v", pol, err)
	}
	if !pol.ElasticEnrolled || pol.MinReplicas != 0 || pol.MaxReplicas < 1 {
		t.Fatalf("unexpected defaults: %+v", pol)
	}
	if pol.BackgroundMode != contract.BackgroundModeNone {
		t.Fatalf("background: %q", pol.BackgroundMode)
	}

	// Idempotent: operator overrides are not clobbered.
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v2", ProjectID: "demo"})
	_ = store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v2", Revision: 1,
		MinReplicas: 2, MaxReplicas: 4, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	})
	if err := o.ensureDefaultElasticServingPolicy(ctx, "demo", "v2"); err != nil {
		t.Fatal(err)
	}
	p2, _ := store.GetServingPolicy(ctx, "demo", "v2")
	if p2 == nil || p2.MinReplicas != 2 {
		t.Fatalf("existing policy mutated: %+v", p2)
	}
}

func TestCommitQualificationIdleDesireOnStore(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(filepath.Join(t.TempDir(), "idle.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	jobID := "job-1"
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		ProjectID: "demo", VersionID: "v1", DesiredReplicas: 1, Generation: 1,
		Reason: deployQualificationReasonForJob(jobID),
	}); err != nil {
		t.Fatal(err)
	}
	if err := commitQualificationIdleDesireOnStore(ctx, store, "demo", "v1", jobID, 0, contract.BackgroundModeNone); err != nil {
		t.Fatal(err)
	}
	d, _ := store.GetServingDesire(ctx, "demo", "v1")
	if d == nil || d.DesiredReplicas != 0 || d.Reason != desireReasonIdle {
		t.Fatalf("desire: %+v", d)
	}
}
