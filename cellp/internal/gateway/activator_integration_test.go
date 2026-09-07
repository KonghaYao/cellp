package gateway

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/scheduler"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

type activationSchedulerClient struct {
	store    registry.ServingStore
	upstream *url.URL
	starts   atomic.Int32
}

func (c *activationSchedulerClient) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, _ string) (contract.RuntimeReplica, error) {
	c.starts.Add(1)
	host, portRaw, err := net.SplitHostPort(c.upstream.Host)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	lease := spec.Scope.LeaseExpiry
	for _, obs := range []registry.ReplicaObservation{
		{
			ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
			NodeID: spec.Scope.NodeID, Generation: spec.Scope.Generation, State: contract.ReplicaStarting,
			AssignmentValidUntil: &lease,
		},
		{
			ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
			NodeID: spec.Scope.NodeID, Generation: spec.Scope.Generation, State: contract.ReplicaReady,
			ListenHost: host, ListenPort: port, EndpointState: contract.EndpointReady,
			AssignmentValidUntil: &lease, EndpointValidUntil: &lease,
		},
	} {
		if err := c.store.RecordObservation(ctx, obs); err != nil {
			return contract.RuntimeReplica{}, err
		}
	}
	return contract.RuntimeReplica{
		ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
		NodeID: spec.Scope.NodeID, Generation: spec.Scope.Generation, State: contract.ReplicaReady, ValidUntil: &lease,
	}, nil
}

func (c *activationSchedulerClient) ProbeReplica(context.Context, contract.CommandScope) (agent.ProbeResult, error) {
	return agent.ProbeResult{State: contract.ReplicaReady}, nil
}

func (c *activationSchedulerClient) DrainReplica(context.Context, contract.CommandScope, time.Time) (contract.RuntimeReplica, error) {
	return contract.RuntimeReplica{}, nil
}

func (c *activationSchedulerClient) StopReplica(context.Context, contract.CommandScope) (contract.RuntimeReplica, error) {
	return contract.RuntimeReplica{}, nil
}

func TestActivatorRequestPathZeroToOneForwardsBodyOnce(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := registry.Open(t.TempDir() + "/activator-e2e.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"})
	_, _ = store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"})
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 1, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusDeployReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{DesiredReplicas: 0, Generation: 1, Reason: "idle"}); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().UTC().Add(time.Minute)
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 1, Generation: 1, LeaseExpiry: valid,
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp.test/node/n1", Zone: "test",
	}); err != nil {
		t.Fatal(err)
	}
	host := "v1.demo.ingress.local"
	versionID := "v1"
	if err := store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: "preview:demo:v1", ProjectID: "demo", VersionID: &versionID,
		Role: registry.IngressRolePreview, Host: &host, SyntheticHost: host, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	gw := NewWithConfig(store, GatewayConfig{GatewayPort: 8787})
	actCfg := activator.DefaultConfig()
	actCfg.WakeTimeout = 2 * time.Second
	actCfg.PollInterval = time.Millisecond
	if err := gw.ConfigureElasticActivator(&activator.RegistryEnsureClient{Store: store, Guard: activatorTestGuard{}}, actCfg); err != nil {
		t.Fatal(err)
	}
	defer gw.ShutdownElasticActivator(context.Background())
	snapshotDone := gw.StartRouteSnapshotPoller(ctx, time.Millisecond)

	client := &activationSchedulerClient{store: store, upstream: upstreamURL}
	ctrl := &scheduler.Controller{
		Store: scheduler.RegistryStore{ServingStore: store}, Guard: scheduler.NoopGuard{},
		Clients: func(contract.RuntimeNode) (scheduler.RuntimeNodeClient, error) { return client, nil },
	}
	schedulerDone := scheduler.Start(ctx, ctrl, scheduler.Config{Interval: time.Millisecond, Background: true}, nil)

	body := "write-once"
	req := httptest.NewRequest(http.MethodPost, "http://"+host+"/submit", strings.NewReader(body))
	req.Host = host
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated || rr.Body.String() != body {
		t.Fatalf("response status=%d body=%q headers=%v", rr.Code, rr.Body.String(), rr.Header())
	}
	if calls := upstreamCalls.Load(); calls != 1 {
		t.Fatalf("upstream attempts=%d", calls)
	}
	if starts := client.starts.Load(); starts != 1 {
		t.Fatalf("scheduler starts=%d", starts)
	}
	desire, _ := store.GetServingDesire(ctx, "demo", "v1")
	version, _ := store.GetVersion(ctx, "demo", "v1")
	if desire == nil || desire.DesiredReplicas != 1 || desire.Reason != "activator_ensure" || version == nil || version.Status != registry.StatusReady {
		t.Fatalf("desire=%+v version=%+v", desire, version)
	}
	cancel()
	select {
	case <-schedulerDone:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not quiesce")
	}
	select {
	case <-snapshotDone:
	case <-time.After(time.Second):
		t.Fatal("snapshot poller did not quiesce")
	}
}
