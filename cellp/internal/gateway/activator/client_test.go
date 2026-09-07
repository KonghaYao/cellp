package activator_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

type allowGuard struct{ err error }

func (g allowGuard) HoldsWriteLock(context.Context) error { return g.err }

func setupEnsureClient(t *testing.T, enrolled bool) (*registry.SQLiteStore, *activator.RegistryEnsureClient) {
	t.Helper()
	store, err := registry.Open(t.TempDir() + "/act.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "proj"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "ver", ProjectID: "proj"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "proj", "ver", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "proj", "ver", registry.StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if enrolled {
		if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
			ProjectID: "proj", VersionID: "ver", Revision: 1,
			MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
			NodeID: "n1", CapacityUnits: 1, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Minute),
			AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp.test/node/n1", Zone: "test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return store, &activator.RegistryEnsureClient{Store: store, Guard: allowGuard{}}
}

func TestRegistryEnsureClientIdempotent(t *testing.T) {
	store, client := setupEnsureClient(t, true)
	ctx := context.Background()
	if err := client.EnsureCapacity(ctx, "proj", "ver", 1); err != nil {
		t.Fatal(err)
	}
	d, err := store.GetServingDesire(ctx, "proj", "ver")
	if err != nil || d == nil || d.DesiredReplicas != 1 {
		t.Fatalf("desire %+v err %v", d, err)
	}
	if err := client.EnsureCapacity(ctx, "proj", "ver", 1); err != nil {
		t.Fatal(err)
	}
	d2, _ := store.GetServingDesire(ctx, "proj", "ver")
	if d2.Generation != d.Generation {
		t.Fatalf("idempotent ensure should not bump generation: %d -> %d", d.Generation, d2.Generation)
	}
}

func TestRegistryEnsureClientEligibilityAndGuardFailClosed(t *testing.T) {
	_, notEnrolled := setupEnsureClient(t, false)
	if err := notEnrolled.EnsureCapacity(context.Background(), "proj", "ver", 1); !errors.Is(err, activator.ErrActivationNotEligible) {
		t.Fatalf("non-enrolled: %v", err)
	}
	store, _ := setupEnsureClient(t, true)
	lost := &activator.RegistryEnsureClient{Store: store, Guard: allowGuard{err: errors.New("lost")}}
	if err := lost.EnsureCapacity(context.Background(), "proj", "ver", 1); !errors.Is(err, activator.ErrActivationGuardLost) {
		t.Fatalf("guard loss: %v", err)
	}
}

func TestRegistryEnsureClientConcurrentInstancesOneGeneration(t *testing.T) {
	store, client := setupEnsureClient(t, true)
	ctx := context.Background()
	const n = 64
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- client.EnsureCapacity(ctx, "proj", "ver", 1)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	desire, err := store.GetServingDesire(ctx, "proj", "ver")
	if err != nil || desire == nil || desire.DesiredReplicas != 1 || desire.Generation != 1 {
		t.Fatalf("desire=%+v err=%v", desire, err)
	}
}

func TestRegistryEnsureClientNotQualifiedWithoutReadyAt(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/act.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "proj"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "ver", ProjectID: "proj"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "proj", "ver", registry.StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "proj", VersionID: "ver", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	client := &activator.RegistryEnsureClient{Store: store, Guard: allowGuard{}}
	if err := client.EnsureCapacity(ctx, "proj", "ver", 1); !errors.Is(err, activator.ErrActivationNotQualified) {
		t.Fatalf("got %v", err)
	}
}

func TestRegistryEnsureClientReportsCapacityExhausted(t *testing.T) {
	store, client := setupEnsureClient(t, true)
	node, err := store.GetRuntimeNode(context.Background(), "n1")
	if err != nil || node == nil {
		t.Fatalf("node=%+v err=%v", node, err)
	}
	node.Cordoned = true
	if err := store.UpsertRuntimeNode(context.Background(), *node); err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureCapacity(context.Background(), "proj", "ver", 1); !errors.Is(err, activator.ErrActivationCapacityUnavailable) {
		t.Fatalf("capacity result: %v", err)
	}
	if desire, err := store.GetServingDesire(context.Background(), "proj", "ver"); err != nil || desire != nil {
		t.Fatalf("capacity failure wrote desire: %+v err=%v", desire, err)
	}
}
