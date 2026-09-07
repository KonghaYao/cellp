package transport_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/registry"
)

const testSecretValue = "LEAKME-xyzzy-token"

type relayStoreWithFault struct {
	registry.ServingStore
	authorizedEnvErr error
}

func (s *relayStoreWithFault) GetAuthorizedAgentVersionEnv(ctx context.Context, nodeID, projectID, versionID string, now time.Time) (map[string]string, error) {
	if s.authorizedEnvErr != nil {
		return nil, s.authorizedEnvErr
	}
	type hasAuthorized interface {
		GetAuthorizedAgentVersionEnv(context.Context, string, string, string, time.Time) (map[string]string, error)
	}
	if inner, ok := s.ServingStore.(hasAuthorized); ok {
		return inner.GetAuthorizedAgentVersionEnv(ctx, nodeID, projectID, versionID, now)
	}
	return nil, errors.New("inner store missing GetAuthorizedAgentVersionEnv")
}

func seedVersionEnvSecret(t *testing.T, store *registry.SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"GREETING": testSecretValue}); err != nil {
		t.Fatal(err)
	}
}

func assertNoSecretLeak(t *testing.T, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(p, testSecretValue) {
			t.Fatalf("secret leaked in %q", p)
		}
	}
}

func TestGetVersionEnvTLSHappyPathPendingAssignment(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	got, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got["GREETING"] != testSecretValue {
		t.Fatalf("env=%v", got)
	}
}

func TestGetVersionEnvRejectsStoppedReplica(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	ctx := context.Background()
	if err := env.store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaStopped); err != nil {
		t.Fatal(err)
	}
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvRejectsDrainingAndWrongVersion(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	seedRelayReplicaReady(t, env.store, "n1")
	ctx := context.Background()
	rep, err := env.store.GetRuntimeReplica(ctx, "r1")
	if err != nil || rep == nil || rep.ValidUntil == nil {
		t.Fatal(err)
	}
	exp := *rep.ValidUntil
	if err := env.store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: "r1", ProjectID: "demo", VersionID: "v1", NodeID: "n1", Generation: 1,
		State: contract.ReplicaDraining, ListenHost: "127.0.0.1", ListenPort: 9101,
		EndpointState: contract.EndpointDraining, AssignmentValidUntil: &exp, EndpointValidUntil: &exp,
	}); err != nil {
		t.Fatal(err)
	}
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err = env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("draining should deny")
	}
	assertNoSecretLeak(t, err.Error())

	_, err = env.client.GetVersionEnv(ctx, scope, "demo", "missing")
	if err == nil {
		t.Fatal("wrong version should deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvRejectsExpiredAssignmentAndCordon(t *testing.T) {
	ctx := context.Background()

	t.Run("expired_assignment", func(t *testing.T) {
		env := startRelayEnv(t, "n1")
		seedVersionEnvSecret(t, env.store)
		anchor := time.Now().UTC()
		env.relayHandler.Now = func() time.Time { return anchor.Add(3 * time.Hour) }
		scope := env.client.NewRelayScope("n1", time.Minute)
		_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
		if err == nil {
			t.Fatal("expired assignment should deny")
		}
		assertNoSecretLeak(t, err.Error())
	})

	t.Run("cordoned_node", func(t *testing.T) {
		env := startRelayEnv(t, "n1")
		seedVersionEnvSecret(t, env.store)
		if err := env.store.CordonRuntimeNode(ctx, "n1", 1); err != nil {
			t.Fatal(err)
		}
		scope := env.client.NewRelayScope("n1", time.Minute)
		_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
		if err == nil {
			t.Fatal("cordoned node should deny")
		}
		assertNoSecretLeak(t, err.Error())
	})
}

func TestGetVersionEnvRejectsGenerationMismatch(t *testing.T) {
	ctx := context.Background()
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := env.store.ActivateRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 2, LeaseExpiry: exp,
		AgentBaseURL: "https://n1", IdentityURI: env.relayHandler.Allowlist["n1"], Zone: "z",
	}, 1); err != nil {
		t.Fatal(err)
	}
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("generation mismatch should deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvRejectsExpiredNodeLease(t *testing.T) {
	ctx := context.Background()
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	past := time.Now().UTC().Add(-time.Minute)
	node, err := env.store.GetRuntimeNode(ctx, "n1")
	if err != nil || node == nil {
		t.Fatal(err)
	}
	node.LeaseExpiry = past
	if err := env.store.UpsertRuntimeNode(ctx, *node); err != nil {
		t.Fatal(err)
	}
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err = env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expired node lease should deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvTransientDoesNotReturnEnv(t *testing.T) {
	pki := mustPKI(t, "n1")
	store, err := registry.Open(t.TempDir() + "/fault.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedRelayControllerRegistry(t, store, "n1", pki.NodeURI)
	seedVersionEnvSecret(t, store)
	wrapped := &relayStoreWithFault{ServingStore: store, authorizedEnvErr: errors.New("sqlite disk io fault")}
	env := startRelayEnvWithStore(t, "n1", pki, wrapped)
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err = env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected transient failure")
	}
	if !registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("expected registry unavailable, got %v", err)
	}
	assertNoSecretLeak(t, err.Error())
}

func startRelayEnvWithStore(t *testing.T, nodeID string, pki testPKI, store registry.ServingStore) relayEnv {
	t.Helper()
	h := &registryrelay.Handler{Store: store, Allowlist: map[string]string{nodeID: pki.NodeURI}, Guard: guardOK{}}
	var sqlite *registry.SQLiteStore
	if s, ok := store.(*registry.SQLiteStore); ok {
		sqlite = s
	}
	return startRelayEnvWithHandler(t, nodeID, pki, h, sqlite)
}
