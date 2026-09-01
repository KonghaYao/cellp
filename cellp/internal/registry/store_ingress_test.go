package registry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestIngressBindingHostLookup(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	host := "v1.demo.ingress.local"
	synth := "synth-v1.demo.ingress.local"
	bid := uuid.NewString()
	if err := s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: synth, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.LookupIngressByHost(ctx, "V1.demo.ingress.local:8787")
	if err != nil || got == nil || got.BindingID != bid {
		t.Fatalf("lookup=%+v err=%v", got, err)
	}
	active, err := s.ListActiveIngressBindings(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("active=%d err=%v", len(active), err)
	}
	byVer, err := s.ListIngressBindingsByVersion(ctx, "demo", "v1")
	if err != nil || len(byVer) != 1 {
		t.Fatalf("by version=%d err=%v", len(byVer), err)
	}
}

func TestIngressBindingUniqueHost(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress-uniq.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v2", ProjectID: "demo"})

	host := "same.demo.ingress.local"
	_ = s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: uuid.NewString(), ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth-a.ingress.local", Active: true,
	})
	err = s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: uuid.NewString(), ProjectID: "demo", VersionID: strPtr("v2"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth-b.ingress.local", Active: true,
	})
	if err != ErrIngressBindingConflict {
		t.Fatalf("expected conflict got %v", err)
	}
}

func TestIngressBindingListenPortOwner(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress-port.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	port := 19081
	ownerA := "gw-a"
	ownerB := "gw-b"
	synth := "synth-port.ingress.local"
	bid := uuid.NewString()
	if err := s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, ListenPort: &port, SyntheticHost: synth,
		OwnerGatewayID: &ownerA, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	gotWrong, err := s.LookupIngressByListenPort(ctx, port, ownerB)
	if err != nil || gotWrong != nil {
		t.Fatalf("wrong owner: got=%+v err=%v", gotWrong, err)
	}
	got, err := s.LookupIngressByListenPort(ctx, port, ownerA)
	if err != nil || got == nil || got.BindingID != bid {
		t.Fatalf("lookup=%+v err=%v", got, err)
	}
}

func TestIngressBindingDeactivate(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress-off.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})
	_, _ = s.CreateVersion(ctx, CreateVersionInput{ID: "v1", ProjectID: "demo"})

	host := "off.demo.ingress.local"
	bid := uuid.NewString()
	_ = s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host, SyntheticHost: "synth-off.ingress.local", Active: true,
	})
	if err := s.SetIngressBindingActive(ctx, bid, false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.LookupIngressByHost(ctx, host)
	if got != nil {
		t.Fatal("inactive binding should not resolve")
	}
	// Same host can bind again when prior row is inactive.
	host2 := host
	if err := s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: uuid.NewString(), ProjectID: "demo", VersionID: strPtr("v1"),
		Role: IngressRolePreview, Host: &host2, SyntheticHost: "synth-off2.ingress.local", Active: true,
	}); err != nil {
		t.Fatalf("reuse host after deactivate: %v", err)
	}
}

func TestIngressBindingValidation(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress-val.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	err = s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: "b1", ProjectID: "demo", Role: IngressRolePreview,
		VersionID: strPtr("v1"), SyntheticHost: "s.ingress.local", Active: true,
	})
	if !errors.Is(err, ErrIngressBindingInvalid) {
		t.Fatalf("expected invalid got %v", err)
	}
}

func TestIngressProdBindingNullableVersion(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "ingress-prod.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, _ = s.CreateProject(ctx, CreateProjectInput{ID: "demo"})

	host := "demo.ingress.local"
	bid := uuid.NewString()
	if err := s.UpsertIngressBinding(ctx, IngressBinding{
		BindingID: bid, ProjectID: "demo", Role: IngressRoleProd,
		Host: &host, SyntheticHost: "prod-synth.ingress.local", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetIngressBinding(ctx, bid)
	if err != nil || got == nil || got.VersionID != nil || got.Role != IngressRoleProd {
		t.Fatalf("prod binding=%+v err=%v", got, err)
	}
}

func strPtr(s string) *string { return &s }
