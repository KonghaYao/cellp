package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

// Only a connect failure proves the request never reached an upstream. Everything after
// dispatch keeps the 502 path, so a mutation is never re-sent and never replayed through
// the cold-start path.
func TestPreDispatchDialFailureOnlyMatchesConnectFailures(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	if !preDispatchDialFailure(refused) {
		t.Fatal("connect failure must be a cold miss")
	}
	timeoutDial := &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}
	if !preDispatchDialFailure(timeoutDial) {
		t.Fatal("dial timeout never reached the upstream and must be a cold miss")
	}
	for name, err := range map[string]error{
		"nil":         nil,
		"read reset":  &net.OpError{Op: "read", Net: "tcp", Err: io.EOF},
		"write reset": &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE},
		"plain":       errors.New("bad gateway"),
		"context":     errors.New("context canceled"),
	} {
		if preDispatchDialFailure(err) {
			t.Fatalf("%s: %v must not be treated as a pre-dispatch cold miss", name, err)
		}
	}
}

type countingEnsureClient struct {
	calls atomic.Int32
}

func (c *countingEnsureClient) EnsureCapacity(context.Context, string, string, int) error {
	c.calls.Add(1)
	return nil
}

// closedPortAddress reserves a loopback port and releases it, so connecting to it is
// refused exactly like a snapshot endpoint whose process retired.
func closedPortAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

// publishSnapshot writes the immutable snapshot the Gateway routes from, as the revision
// poller would after applying a registry revision.
func publishSnapshot(t *testing.T, gw *Gateway, projectID, versionID, address string) {
	t.Helper()
	valid := time.Now().UTC().Add(time.Minute)
	h := gw.snapshots
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snap = contract.RouteSnapshot{
		Revision: h.lastAppliedRev + 1,
		EndpointSets: []contract.EndpointSet{{
			ProjectID: projectID, VersionID: versionID,
			Endpoints: []contract.Endpoint{{ReplicaID: "rep-1", Address: address, State: contract.EndpointReady, ValidUntil: &valid}},
		}},
	}
	h.hasLKG = true
	h.lastAppliedRev++
}

const coldMissHost = "v1.demo.ingress.local"

// newColdMissGateway builds an enrolled, ready version whose only replica is scaled to
// zero: the state a first request after deploy qualification lands in. ensureFor is nil
// for a Gateway without the elastic activator.
func newColdMissGateway(t *testing.T, ensureFor func(*registry.SQLiteStore) activator.EnsureCapacityClient) (*Gateway, *registry.SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	store, err := registry.Open(t.TempDir() + "/cold-miss.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: "v1", ProjectID: "demo"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: "demo", VersionID: "v1", Revision: 1,
		MinReplicas: 0, MaxReplicas: 8, BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, "demo", "v1", registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, "demo", "v1", 0, registry.ServingDesireRow{
		DesiredReplicas: 0, Generation: 1, Reason: "idle",
	}); err != nil {
		t.Fatal(err)
	}
	host, versionID := coldMissHost, "v1"
	if err := store.UpsertIngressBinding(ctx, registry.IngressBinding{
		BindingID: "preview:demo:v1", ProjectID: "demo", VersionID: &versionID,
		Role: registry.IngressRolePreview, Host: &host, SyntheticHost: host, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	// One eligible node: the activation ensure refuses to bump without capacity.
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: "n1", CapacityUnits: 2, Generation: 1, LeaseExpiry: time.Now().UTC().Add(time.Minute),
		AgentBaseURL: "https://n1.agent.example", IdentityURI: "spiffe://cellp.test/node/n1", Zone: "test",
	}); err != nil {
		t.Fatal(err)
	}
	gw := NewWithConfig(store, GatewayConfig{GatewayPort: 8787})
	if ensureFor != nil {
		cfg := activator.DefaultConfig()
		cfg.WakeTimeout = 2 * time.Second
		cfg.PollInterval = time.Millisecond
		if err := gw.ConfigureElasticActivator(ensureFor(store), cfg); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = gw.ShutdownElasticActivator(context.Background()) })
	return gw, store
}

func coldMissRequest(t *testing.T, gw *Gateway, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "http://"+coldMissHost+"/e2e-r2-branch.txt", strings.NewReader(body))
	req.Host = coldMissHost
	rr := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rr, req)
	return rr
}

func waitForEnsure(t *testing.T, ensure *countingEnsureClient, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ensure.calls.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ensure calls=%d, want>=%d", ensure.calls.Load(), want)
}

// The warm-entry defect: the snapshot still names a retired replica, the connect is
// refused, and the old code asked that same stale snapshot whether the version was warm,
// got "yes", and answered 502. The refused connect is proof the endpoint is gone, so the
// request must reach the cold contract instead — and must not be dispatched again.
func TestDeadSnapshotEndpointRefusedConnectReturnsColdContract(t *testing.T) {
	var liveCalls atomic.Int32
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		liveCalls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer live.Close()

	ensure := &countingEnsureClient{}
	gw, _ := newColdMissGateway(t, func(*registry.SQLiteStore) activator.EnsureCapacityClient { return ensure })
	publishSnapshot(t, gw, "demo", "v1", closedPortAddress(t))

	rr := coldMissRequest(t, gw, "parent-r2-body")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q headers=%v", rr.Code, rr.Body.String(), rr.Header())
	}
	if got := rr.Header().Get(activator.HeaderCellpReason); got != activator.ReasonWakeRetry {
		t.Fatalf("reason=%q headers=%v", got, rr.Header())
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatalf("missing Retry-After: %v", rr.Header())
	}
	waitForEnsure(t, ensure, 1)
	time.Sleep(50 * time.Millisecond)
	if calls := ensure.calls.Load(); calls != 1 {
		t.Fatalf("one refused connect refreshed capacity %d times", calls)
	}
	if calls := liveCalls.Load(); calls != 0 {
		t.Fatalf("mutation was dispatched %d time(s) despite a refused connect", calls)
	}
}

// Without the activator the Gateway keeps the legacy answer: a dead upstream is a bad
// gateway. Elastic deployments never lose their cold contract to a missing writer.
func TestDeadSnapshotEndpointWithoutActivatorKeepsBadGateway(t *testing.T) {
	gw, _ := newColdMissGateway(t, nil)
	publishSnapshot(t, gw, "demo", "v1", closedPortAddress(t))

	rr := coldMissRequest(t, gw, "parent-r2-body")
	if rr.Code != http.StatusBadGateway || rr.Body.String() != "bad gateway\n" {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

// Scale-to-zero recovery: the refused connect is answered with the cold contract, the
// capacity bump is recorded in the registry, and once the snapshot carries the
// replacement replica the same request is served exactly once.
func TestDeadSnapshotEndpointRecoversThroughColdContract(t *testing.T) {
	t.Setenv(contract.EnvElasticRuntime, "1")
	var delivered atomic.Int32
	var got atomic.Value
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered.Add(1)
		body, _ := io.ReadAll(r.Body)
		got.Store(string(body))
		w.WriteHeader(http.StatusCreated)
	}))
	defer live.Close()
	_, livePortRaw, err := net.SplitHostPort(strings.TrimPrefix(live.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.Atoi(livePortRaw); err != nil {
		t.Fatal(err)
	}

	gw, store := newColdMissGateway(t, func(store *registry.SQLiteStore) activator.EnsureCapacityClient {
		return &activator.RegistryEnsureClient{Store: store, Guard: activatorTestGuard{}}
	})
	publishSnapshot(t, gw, "demo", "v1", closedPortAddress(t))

	rr := coldMissRequest(t, gw, "parent-r2-body")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("first attempt status=%d body=%q", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	var desire *registry.ServingDesireRow
	for time.Now().Before(deadline) {
		if desire, err = store.GetServingDesire(context.Background(), "demo", "v1"); err != nil {
			t.Fatal(err)
		}
		if desire != nil && desire.DesiredReplicas >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if desire == nil || desire.DesiredReplicas != 1 || desire.Reason != "activator_ensure" {
		t.Fatalf("capacity not refreshed: %+v", desire)
	}

	// The poller applies the retire revision and then the replacement endpoint.
	publishSnapshot(t, gw, "demo", "v1", strings.TrimPrefix(live.URL, "http://"))
	rr = coldMissRequest(t, gw, "parent-r2-body")
	if rr.Code != http.StatusCreated {
		t.Fatalf("recovered attempt status=%d body=%q", rr.Code, rr.Body.String())
	}
	if calls := delivered.Load(); calls != 1 {
		t.Fatalf("upstream deliveries=%d", calls)
	}
	if body, _ := got.Load().(string); body != "parent-r2-body" {
		t.Fatalf("upstream body=%q", body)
	}
}
