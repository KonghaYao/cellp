package registryrelay_test

import (
	"context"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/registry"
)

type bareGetVersionEnvSpy struct {
	*registry.SQLiteStore
	getVersionEnvCalls int
}

func (s *bareGetVersionEnvSpy) GetVersionEnv(ctx context.Context, projectID, versionID string) (map[string]string, error) {
	s.getVersionEnvCalls++
	return s.SQLiteStore.GetVersionEnv(ctx, projectID, versionID)
}

func TestHandlerGetVersionEnvUsesAuthorizedAtomicRead(t *testing.T) {
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/handler-spy.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedHandlerEnvFixture(t, store)
	spy := &bareGetVersionEnvSpy{SQLiteStore: store}
	h := &registryrelay.Handler{
		Store:     spy,
		Allowlist: map[string]string{"n1": "spiffe://test/n1"},
	}
	now := time.Now().UTC()
	scope := contract.RegistryRelayScope{
		NodeID: "n1", Nonce: "nonce-1",
		IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	env, err := h.GetVersionEnv(ctx, "spiffe://test/n1", scope, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if env["GREETING"] != "hi" {
		t.Fatalf("env=%v", env)
	}
	if spy.getVersionEnvCalls != 0 {
		t.Fatalf("handler must not call bare GetVersionEnv, calls=%d", spy.getVersionEnvCalls)
	}
}

func seedHandlerEnvFixture(t *testing.T, store *registry.SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"GREETING": "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{DesiredReplicas: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 1, LeaseExpiry: exp,
		AgentBaseURL: "https://n1", IdentityURI: "spiffe://test/n1", Zone: "z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: exp,
	}); err != nil {
		t.Fatal(err)
	}
}
