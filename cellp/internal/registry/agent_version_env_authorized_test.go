package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

func TestGetAuthorizedAgentVersionEnvReturnsEnvForLiveAssignment(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/authz-env.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, state := range []contract.ReplicaState{contract.ReplicaPending, contract.ReplicaStarting, contract.ReplicaReady} {
		t.Run(string(state), func(t *testing.T) {
			if err := setReplicaState(t, store, "r1", state); err != nil {
				t.Fatal(err)
			}
			env, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
			if err != nil {
				t.Fatalf("state %s: %v", state, err)
			}
			if env["K"] != "v" {
				t.Fatalf("env=%v", env)
			}
		})
	}
}

func TestGetAuthorizedAgentVersionEnvRejectsWhenAuthorizeWould(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("stopped", func(t *testing.T) {
		store, err := Open(t.TempDir() + "/stopped.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
		_ = store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"SECRET": "x"})
		if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaStopped); err != nil {
			t.Fatal(err)
		}
		_, err = store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
		if !errors.Is(err, ErrAgentVersionEnvForbidden) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("failed", func(t *testing.T) {
		store, err := Open(t.TempDir() + "/failed.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
		_ = store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"SECRET": "x"})
		if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaFailed); err != nil {
			t.Fatal(err)
		}
		_, err = store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
		if !errors.Is(err, ErrAgentVersionEnvForbidden) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("draining", func(t *testing.T) {
		store, err := Open(t.TempDir() + "/drain.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
		_ = store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"SECRET": "x"})
		if err := markReplicaReady(t, store, "r1"); err != nil {
			t.Fatal(err)
		}
		if err := markReplicaDraining(t, store, "r1"); err != nil {
			t.Fatal(err)
		}
		_, err = store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
		if !errors.Is(err, ErrAgentVersionEnvForbidden) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestGetAuthorizedAgentVersionEnvAnyLiveReplicaAmongMany(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/multi.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 1, ServingDesireRow{DesiredReplicas: 2, Generation: 2}); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	if err := store.ClaimAssignment(ctx, AssignmentClaim{
		ReplicaID: "r2", ProjectID: "demo", VersionID: "v1", NodeID: "n1",
		Generation: 2, ExpectedNodeGeneration: 1, ValidUntil: exp,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"K": "v"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaStopped); err != nil {
		t.Fatal(err)
	}
	env, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
	if err != nil {
		t.Fatal(err)
	}
	if env["K"] != "v" {
		t.Fatalf("env=%v", env)
	}
}

// Concurrent overlap may still return env for read transactions that opened before
// terminalize committed (SQLite snapshot isolation). After terminalize commits,
// every new GetAuthorizedAgentVersionEnv must be forbidden.
func TestGetAuthorizedAgentVersionEnvAfterTerminalCommitAllReadsForbidden(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir() + "/race.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedAuthorizeEnvFixture(t, store, "n1", "demo", "v1", "r1")
	secret := "race-secret-value"
	if err := store.SetVersionEnv(ctx, "demo", "v1", map[string]string{"TOKEN": secret}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	const readers = 8
	start := make(chan struct{})
	terminalDone := make(chan struct{})
	readerErr := make(chan error, readers+1)
	var readersReady sync.WaitGroup
	readersReady.Add(readers)

	for i := 0; i < readers; i++ {
		go func() {
			readersReady.Done()
			<-start
			for j := 0; j < 50; j++ {
				env, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
				if err != nil {
					if errors.Is(err, ErrAgentVersionEnvForbidden) {
						return
					}
					readerErr <- err
					return
				}
				if env["TOKEN"] != secret {
					readerErr <- errors.New("bad env during overlap")
					return
				}
			}
		}()
	}
	readersReady.Wait()

	go func() {
		if err := store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaStopped); err != nil {
			readerErr <- err
		}
		close(terminalDone)
	}()
	close(start)
	<-terminalDone

	for round := 0; round < 3; round++ {
		for i := 0; i < 40; i++ {
			_, err := store.GetAuthorizedAgentVersionEnv(ctx, "n1", "demo", "v1", now)
			if !errors.Is(err, ErrAgentVersionEnvForbidden) {
				t.Fatalf("round %d read %d after terminal commit: %v", round, i, err)
			}
		}
	}
	select {
	case err := <-readerErr:
		t.Fatal(err)
	default:
	}
}
