package orch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

type noopRouteSnapshotAck struct{}

func (noopRouteSnapshotAck) WaitPublished(context.Context, registry.Store, int64, string, string) error {
	return nil
}

func (noopRouteSnapshotAck) WaitPublicServingPublished(context.Context, registry.Store, int64, string, string) error {
	return nil
}

func ensureQualificationTestNode(ctx context.Context, store registry.Store) error {
	return store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID:        "n1",
		CapacityUnits: 64,
		Generation:    1,
		LeaseExpiry:   time.Now().UTC().Add(time.Hour),
		AgentBaseURL:  "https://127.0.0.1:19443",
		IdentityURI:   "spiffe://cellp.test/node/n1",
		Zone:          "test",
	})
}

func seedElasticQualificationEndpoint(t *testing.T, ctx context.Context, store registry.Store, projectID, versionID string, listenPort int) {
	t.Helper()
	replicaID := fmt.Sprintf("r-%s-%s", projectID, versionID)
	valid := time.Now().UTC().Add(time.Hour)
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: replicaID, ProjectID: projectID, VersionID: versionID, NodeID: "n1",
		Generation: 1, ExpectedNodeGeneration: 1, ValidUntil: valid,
	}); err != nil {
		t.Fatal(err)
	}
	epUntil := time.Now().UTC().Add(time.Hour)
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: replicaID, ProjectID: projectID, VersionID: versionID, NodeID: "n1",
		Generation: 1, State: contract.ReplicaStarting, AssignmentValidUntil: &valid,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordObservation(ctx, registry.ReplicaObservation{
		ReplicaID: replicaID, ProjectID: projectID, VersionID: versionID, NodeID: "n1",
		Generation: 1, State: contract.ReplicaReady,
		ListenHost: "127.0.0.1", ListenPort: listenPort,
		EndpointState: contract.EndpointReady, EndpointValidUntil: &epUntil,
		AssignmentValidUntil: &valid,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedQualifyingVersionsFromStore(t *testing.T, ctx context.Context, store registry.Store, listenPort int, seeded *sync.Map) {
	t.Helper()
	policies, err := store.ListElasticServingPolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pol := range policies {
		key := pol.ProjectID + "/" + pol.VersionID
		if _, ok := seeded.Load(key); ok {
			continue
		}
		d, err := store.GetServingDesire(ctx, pol.ProjectID, pol.VersionID)
		if err != nil || d == nil || !strings.HasPrefix(d.Reason, desireReasonDeployQualificationPrefix) {
			continue
		}
		seedElasticQualificationEndpoint(t, ctx, store, pol.ProjectID, pol.VersionID, listenPort)
		seeded.Store(key, struct{}{})
	}
}

// wireElasticDeployTestFixtures satisfies elastic deploy qualification in unit tests without celld.
func wireElasticDeployTestFixtures(t *testing.T, o *Orchestrator, store registry.Store) {
	t.Helper()
	if err := ensureQualificationTestNode(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/celld/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	var seeded sync.Map
	o.SetElasticSchedulerTick(func(ctx context.Context) error {
		seedQualifyingVersionsFromStore(t, ctx, store, port, &seeded)
		return nil
	})
	o.SetRouteSnapshotAck(noopRouteSnapshotAck{})
}
