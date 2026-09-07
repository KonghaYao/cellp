package orch

import (
	"errors"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

func TestDeployQualificationReasonAmbiguous(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{"", true},
		{"deploy_qualification", true},
		{"deploy_qualification:", true},
		{"deploy_qualification:job-1", false},
		{"deploy_failed", false},
		{"scale-up", false},
	}
	for _, tc := range cases {
		if got := deployQualificationReasonAmbiguous(tc.reason); got != tc.want {
			t.Fatalf("reason=%q want ambiguous=%v got %v", tc.reason, tc.want, got)
		}
	}
}

func TestServingDesireIndicatesNewOwner_Table(t *testing.T) {
	self := "job-self"
	cases := []struct {
		reason string
		want   bool
	}{
		{"", false},
		{"deploy_failed", false},
		{"deploy_qualification", false},
		{"deploy_qualification:", false},
		{"deploy_qualification:job-self", false},
		{"deploy_qualification:other-job", true},
		{"autoscaler", true},
		{"activator", true},
		{"desired0", true},
	}
	for _, tc := range cases {
		cur := &registry.ServingDesireRow{Reason: tc.reason, DesiredReplicas: 1, Generation: 1}
		if got := servingDesireIndicatesNewOwner(cur, self); got != tc.want {
			t.Fatalf("reason=%q want newOwner=%v got %v", tc.reason, tc.want, got)
		}
	}
	if servingDesireIndicatesNewOwner(nil, self) {
		t.Fatal("nil desire")
	}
}

func TestCompensationSuperseded_LegacyReasonIncomplete(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	j := &registry.Job{ID: "job-legacy", ProjectID: "demo", VersionID: "v1"}
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "deploy_qualification",
	})
	o := &Orchestrator{store: store}
	superseded, err := o.compensationSuperseded(ctx, j)
	if superseded || !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("legacy reason must wait for recovery: superseded=%v err=%v", superseded, err)
	}
}

func TestCompensationSuperseded_InactiveOtherHolderIncomplete(t *testing.T) {
	store, ctx := openQualificationTestStore(t)
	_, _ = store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	jobB := claimOrchJob(t, store, ctx, "w-b")
	attB := registry.JobDeployAttempt(jobB)
	if err := store.ClaimVersionDeployOperation(ctx, "demo", "v1", attB, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	_ = store.FailJob(ctx, jobB.ID)
	jobA, _ := store.EnqueueJob(ctx, "demo", "v1", registry.StatusFetching)
	if err := store.MarkJobCompensating(ctx, jobA.ID); err != nil {
		t.Fatal(err)
	}
	_ = store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: deployQualificationReasonForJob(jobA.ID),
	})
	jA, _ := store.GetJob(ctx, jobA.ID)
	o := &Orchestrator{store: store}
	superseded, err := o.compensationSuperseded(ctx, jA)
	if superseded || !errors.Is(err, ErrDeployCompensationIncomplete) {
		t.Fatalf("inactive other holder must not supersede: superseded=%v err=%v", superseded, err)
	}
}
