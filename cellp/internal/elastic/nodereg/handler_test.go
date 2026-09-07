package nodereg_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/nodereg"
	"github.com/cellp/cellp/internal/registry"
)

const testAgentBaseURL = "https://n1.agent.example"

func testAllowlist(nodeID, uri, agentURL string) map[string]contract.NodeRegistrationBinding {
	return map[string]contract.NodeRegistrationBinding{
		nodeID: {IdentityURI: uri, AgentBaseURL: agentURL},
	}
}

type guardOK struct{}

func (guardOK) EnsureActiveController(context.Context) error { return nil }

func TestHandlerActivateAndHeartbeatCAS(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{
		Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{},
		HeartbeatTTL: 30 * time.Second,
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{
		NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1",
	}
	act, err := h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: testAgentBaseURL,
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	if err != nil || act.Generation != 1 {
		t.Fatalf("activate: %+v err=%v", act, err)
	}
	hbScope := scope
	hbScope.Generation = 1
	hbScope.Nonce = "hb1"
	if err := h.Heartbeat(context.Background(), uri, contract.HeartbeatNodeRequest{
		Scope: hbScope, LeaseExpiry: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	stale := hbScope
	stale.Generation = 0
	stale.Nonce = "stale"
	if err := h.Heartbeat(context.Background(), uri, contract.HeartbeatNodeRequest{
		Scope: stale, LeaseExpiry: now.Add(time.Hour),
	}); err == nil {
		t.Fatal("expected stale heartbeat rejection")
	}
}

func TestHandlerActivateConcurrentSingleWinner(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-conc.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{
		Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{},
		HeartbeatTTL: 30 * time.Second,
	}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{
		NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "base",
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var seq int64
	wins := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := scope
			s.Nonce = fmt.Sprintf("act-%d", atomic.AddInt64(&seq, 1))
			_, err := h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
				Scope: s, CapacityUnits: 2, AgentBaseURL: testAgentBaseURL,
				IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
			})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("expected one activate winner, got %d", wins)
	}
}

func TestHandlerActivateRejectsHTTPAgentBaseURL(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-badurl.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{}}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1"}
	_, err = h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: "http://127.0.0.1:9443",
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	if err == nil {
		t.Fatal("expected https agent base url rejection")
	}
	var he *nodereg.HandlerError
	if !errors.As(err, &he) || he.Reason != contract.ReasonAuthFailed {
		t.Fatalf("expected auth_failed, got %v", err)
	}
}

func TestHandlerActivatePrivateConfiguredAccepted(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-private.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	privateURL := "https://10.0.0.5:9443"
	h := &nodereg.Handler{Store: store, Allowlist: testAllowlist("n1", uri, privateURL), Guard: guardOK{}}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1"}
	act, err := h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: privateURL,
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	if err != nil || act.Generation != 1 {
		t.Fatalf("activate private: %+v err=%v", act, err)
	}
}

func TestHandlerActivateRejectsAgentURLMismatch(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-mismatch.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{}}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1"}
	_, err = h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: "https://10.0.0.9:9443",
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	var he *nodereg.HandlerError
	if !errors.As(err, &he) || he.Reason != contract.ReasonAuthFailed || he.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("expected bad request auth_failed, got %v", err)
	}
}

func TestHandlerScopeExpiredRejected(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-expired.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{}}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{
		NodeID: "n1", IssuedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-time.Minute), Nonce: "exp",
	}
	_, err = h.Activate(context.Background(), uri, contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: testAgentBaseURL,
		IdentityURI: uri, Zone: "z", ExpectedGeneration: 0,
	})
	var he *nodereg.HandlerError
	if !errors.As(err, &he) || he.Reason != contract.ReasonAuthFailed {
		t.Fatalf("expected scope auth_failed, got %v", err)
	}
	if nodereg.IsRegistryUnavailable(err) {
		t.Fatal("scope expiry must not be registry unavailable")
	}
}

func TestHandlerAllowlistAuthNotRegistryUnavailable(t *testing.T) {
	store, err := registry.Open(t.TempDir() + "/nodereg-auth.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uri := "spiffe://cellp/test/node/n1"
	h := &nodereg.Handler{Store: store, Allowlist: testAllowlist("n1", uri, testAgentBaseURL), Guard: guardOK{}}
	now := time.Now().UTC()
	scope := contract.NodeRegistrationScope{NodeID: "n1", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Nonce: "n1"}
	_, err = h.Activate(context.Background(), "spiffe://cellp/test/node/other", contract.ActivateNodeRequest{
		Scope: scope, CapacityUnits: 2, AgentBaseURL: testAgentBaseURL,
		IdentityURI: "spiffe://cellp/test/node/other", Zone: "z", ExpectedGeneration: 0,
	})
	mapped := nodereg.MapHandlerError(err)
	var he *nodereg.HandlerError
	if !errors.As(mapped, &he) || he.Reason != contract.ReasonAuthFailed {
		t.Fatalf("mapped: %v", mapped)
	}
	if nodereg.IsRegistryUnavailable(mapped) {
		t.Fatal("auth must not map to registry unavailable")
	}
}
