package scheduler

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/health"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

const (
	envSingleNodeSchedulerTest     = "CELLP_SINGLE_NODE_SCHEDULER_TEST"
	envSingleNodeSchedulerProject  = "CELLP_SINGLE_NODE_SCHED_PROJECT"
	envSingleNodeSchedulerVersion  = "CELLP_SINGLE_NODE_SCHED_VERSION"
	envSingleNodeSchedulerBasePort = "CELLP_SINGLE_NODE_SCHED_BASE_PORT"
)

func TestControllerSingleNodeRealCelldScheduler(t *testing.T) {
	if os.Getenv(envSingleNodeSchedulerTest) != "1" {
		t.Skip("set CELLP_SINGLE_NODE_SCHEDULER_TEST=1 to run single-node scheduler + real celld acceptance")
	}
	t.Setenv(contract.EnvElasticRuntime, "1")
	if !runtime.CelldInstalled() {
		t.Fatal("single-node scheduler acceptance requires celld on PATH")
	}

	project := strings.TrimSpace(os.Getenv(envSingleNodeSchedulerProject))
	version := strings.TrimSpace(os.Getenv(envSingleNodeSchedulerVersion))
	if project == "" || version == "" {
		t.Fatal("requires CELLP_SINGLE_NODE_SCHED_PROJECT and CELLP_SINGLE_NODE_SCHED_VERSION")
	}

	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	accessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("requires S3_ENDPOINT, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY")
	}
	if region == "" {
		region = "us-east-1"
	}

	basePort := 20992
	if raw := strings.TrimSpace(os.Getenv(envSingleNodeSchedulerBasePort)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			t.Fatalf("invalid %s: %q", envSingleNodeSchedulerBasePort, raw)
		}
		basePort = parsed
	}

	bindHost := "127.0.0.2"
	advertiseHost := "localhost"
	if err := ensureSchedulerLoopbackAlias(bindHost); err != nil {
		bindHost = "127.0.0.1"
		advertiseHost = "127.0.0.1"
		t.Logf("127.0.0.2 unavailable; falling back to loopback-only bind/advertise")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	store, err := registry.Open(t.TempDir() + "/single-node-sched.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: project}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: version, ProjectID: project}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateVersionStatus(ctx, project, version, registry.StatusReady, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertServingPolicy(ctx, registry.ServingPolicyRow{
		ProjectID: project, VersionID: version, Revision: 1, MinReplicas: 0, MaxReplicas: 8,
		BackgroundMode: contract.BackgroundModeNone, ElasticEnrolled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, project, version, 0, registry.ServingDesireRow{
		DesiredReplicas: 1, Generation: 1, Reason: "single-node-sched-test",
	}); err != nil {
		t.Fatal(err)
	}

	pki := newSchedulerTestPKI(t)
	m := runtime.New(basePort, endpoint, region, "", accessKey, secretKey)
	m.SetReplicaHostConfig(runtime.ReplicaHostConfig{BindHost: bindHost, AdvertiseHost: advertiseHost})
	backend := agent.ManagerBackend{Manager: m}
	handler := agent.NewLifecycleFromRegistry(true, store, backend)

	server, err := transport.NewServer(transport.ServerConfig{
		Enabled: true, NodeID: "solo", BindAddr: "127.0.0.1:0", NodeIdentityURI: pki.nodeURI,
		AllowedControllerURIs: []string{pki.controller},
		TLS:                   transport.TLSMaterials{Cert: pki.server, RootCAs: pki.pool, ServerName: "127.0.0.1"},
	}, handler)
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, stopServer := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(serverCtx) }()
	select {
	case <-server.Ready():
	case <-time.After(15 * time.Second):
		t.Fatal("agent server not ready")
	}
	t.Cleanup(func() {
		stopServer()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		_ = m.StopAll(shutdownCtx)
		<-serverDone
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	node := contract.RuntimeNode{
		NodeID: "solo", CapacityUnits: 4, Generation: 1, LeaseExpiry: now.Add(time.Hour),
		AgentBaseURL: "https://" + server.Addr(), IdentityURI: pki.nodeURI, Zone: "local",
	}
	if err := store.UpsertRuntimeNode(ctx, node); err != nil {
		t.Fatal(err)
	}

	clientTLS := transport.TLSMaterials{Cert: pki.client, RootCAs: pki.pool, ServerName: "127.0.0.1"}
	ctrl := &Controller{
		Store: RegistryStoreFromServing(store), Guard: NoopGuard{}, Now: func() time.Time { return now },
		Clients: func(node contract.RuntimeNode) (RuntimeNodeClient, error) {
			return transport.NewClient(transport.ClientConfig{
				BaseURL: node.AgentBaseURL, ExpectedNodeURI: node.IdentityURI,
				ControllerIdentityURI: pki.controller, TLS: clientTLS,
			})
		},
	}

	var reps []contract.RuntimeReplica
	var soloNode contract.RuntimeNode
	converged := false
	for attempt := 0; attempt < 120; attempt++ {
		if _, err := ctrl.Tick(ctx); err != nil {
			t.Fatalf("scale-up tick: %v", err)
		}
		reps, err = store.ListRuntimeReplicas(ctx, project, version)
		if err != nil {
			t.Fatal(err)
		}
		nodes, err := store.ListRuntimeNodes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nodes {
			if n.NodeID == "solo" {
				soloNode = n
				break
			}
		}
		if len(reps) == 1 && reps[0].State == contract.ReplicaReady && CountMatchingActive(reps, now) == 1 && reps[0].ValidUntil != nil {
			if err := schedulerProbeReplica(ctx, ctrl, soloNode, reps[0]); err == nil {
				converged = true
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !converged {
		t.Fatalf("0->1 converge with agent probe failed: %+v", reps)
	}
	if reps[0].NodeID != "solo" {
		t.Fatalf("unexpected node: %+v", reps[0])
	}

	desire, err := store.GetServingDesire(ctx, project, version)
	if err != nil || desire == nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSetDesired(ctx, project, version, desire.Generation, registry.ServingDesireRow{
		DesiredReplicas: 0, Generation: desire.Generation + 1, Reason: "scale-to-zero",
	}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 60; attempt++ {
		if _, err := ctrl.Tick(ctx); err != nil {
			t.Fatalf("scale-down tick: %v", err)
		}
		reps, err = store.ListRuntimeReplicas(ctx, project, version)
		if err != nil {
			t.Fatal(err)
		}
		allTerminal := true
		for _, rep := range reps {
			if !contract.ReplicaIsTerminal(rep.State) {
				allTerminal = false
				break
			}
		}
		if allTerminal && len(reps) >= 1 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	for _, rep := range reps {
		if !contract.ReplicaIsTerminal(rep.State) {
			t.Fatalf("scale-to-zero: %+v", reps)
		}
	}
}

func schedulerProbeReplica(ctx context.Context, ctrl *Controller, node contract.RuntimeNode, rep contract.RuntimeReplica) error {
	if rep.ValidUntil == nil {
		return fmt.Errorf("replica missing valid_until")
	}
	client, err := ctrl.clientFor(node)
	if err != nil {
		return err
	}
	scope := commandScope(node, rep, *rep.ValidUntil, contract.ActionProbeReplica)
	_, err = client.ProbeReplica(ctx, scope)
	return err
}

func ensureSchedulerLoopbackAlias(host string) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return err
	}
	_ = ln.Close()
	return nil
}

func probeSchedulerCelldHealth(ctx context.Context, host string, port int) bool {
	url := fmt.Sprintf("http://%s/.well-known/celld/health", net.JoinHostPort(host, strconv.Itoa(port)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return health.CelldHealthResponseOK(resp.StatusCode, body)
}

func waitForSchedulerPortClosed(t *testing.T, host string, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("port still open on %s:%d after %s", host, port, timeout)
}
