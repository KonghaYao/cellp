package transport_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registryrelay"
	"github.com/cellp/cellp/internal/registry"
)

func TestGetVersionEnvTLSRejectsFailedReplica(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	ctx := context.Background()
	if err := env.store.TerminalizeReplica(ctx, "r1", "n1", 1, contract.ReplicaFailed); err != nil {
		t.Fatal(err)
	}
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvTLSCrossNodeScopeDenied(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	ctx := context.Background()
	scope := env.client.NewRelayScope("n2", time.Minute)
	_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected cross-node deny")
	}
	assertNoSecretLeak(t, err.Error())
}

func TestGetVersionEnvTLSReplayRejected(t *testing.T) {
	env := startRelayEnv(t, "n1")
	seedVersionEnvSecret(t, env.store)
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	if _, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	_, err := env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected replay rejection")
	}
	var relay *registryrelay.RelayError
	if !errors.As(err, &relay) || relay.Reason != contract.ReasonReplayRejected {
		t.Fatalf("expected replay rejected, got %v", err)
	}
	assertNoSecretLeak(t, err.Error())
}

type guardFail struct{}

func (guardFail) EnsureActiveController(context.Context) error {
	return errors.New("controller standby")
}

func TestGetVersionEnvTLSGuardFailureUnavailable(t *testing.T) {
	pki := mustPKI(t, "n1")
	store, err := registry.Open(t.TempDir() + "/guard.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedRelayControllerRegistry(t, store, "n1", pki.NodeURI)
	seedVersionEnvSecret(t, store)
	h := &registryrelay.Handler{Store: store, Allowlist: map[string]string{"n1": pki.NodeURI}, Guard: guardFail{}}
	env := startRelayEnvWithHandler(t, "n1", pki, h, store)
	ctx := context.Background()
	scope := env.client.NewRelayScope("n1", time.Minute)
	_, err = env.client.GetVersionEnv(ctx, scope, "demo", "v1")
	if err == nil {
		t.Fatal("expected guard failure")
	}
	if !registryrelay.IsRegistryUnavailable(err) {
		t.Fatalf("expected unavailable, got %v", err)
	}
	assertNoSecretLeak(t, err.Error())
}
