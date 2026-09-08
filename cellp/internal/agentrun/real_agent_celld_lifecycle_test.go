package agentrun

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

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	agenttransport "github.com/cellp/cellp/internal/elastic/agent/transport"
	"github.com/cellp/cellp/internal/elastic/contract"
	registryrelaytransport "github.com/cellp/cellp/internal/elastic/registryrelay/transport"
	"github.com/cellp/cellp/internal/health"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

const (
	envRealAgentCelldTest     = "CELLP_REAL_AGENT_CELLD_TEST"
	envRealAgentCelldProject  = "CELLP_REAL_AGENT_CELLD_PROJECT"
	envRealAgentCelldVersion  = "CELLP_REAL_AGENT_CELLD_VERSION"
	envRealAgentCelldReplica  = "CELLP_REAL_AGENT_CELLD_REPLICA"
	envRealAgentCelldBasePort = "CELLP_REAL_AGENT_CELLD_BASE_PORT"
)

func TestAgentRealCelldLifecycle(t *testing.T) {
	if os.Getenv(envRealAgentCelldTest) != "1" {
		t.Skip("set CELLP_REAL_AGENT_CELLD_TEST=1 to run standalone agent real celld lifecycle acceptance")
	}
	if !runtime.CelldInstalled() {
		t.Fatal("standalone agent real celld lifecycle requires celld on PATH")
	}

	project := strings.TrimSpace(os.Getenv(envRealAgentCelldProject))
	version := strings.TrimSpace(os.Getenv(envRealAgentCelldVersion))
	replicaID := strings.TrimSpace(os.Getenv(envRealAgentCelldReplica))
	if project == "" || version == "" || replicaID == "" {
		t.Fatal("requires CELLP_REAL_AGENT_CELLD_PROJECT, CELLP_REAL_AGENT_CELLD_VERSION, and CELLP_REAL_AGENT_CELLD_REPLICA")
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

	basePort := 19992
	if raw := strings.TrimSpace(os.Getenv(envRealAgentCelldBasePort)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			t.Fatalf("invalid %s: %q", envRealAgentCelldBasePort, raw)
		}
		basePort = parsed
	}

	bindHost := "127.0.0.2"
	advertiseHost := "localhost"
	if err := ensureLoopbackAliasReachable(bindHost); err != nil {
		bindHost = "127.0.0.1"
		advertiseHost = "127.0.0.1"
		t.Logf("127.0.0.2 unavailable; falling back to loopback-only bind/advertise on this host")
	}

	const nodeID = "n-test"
	const generation = int64(1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	store, leaseExpiry, err := seedRealAgentRegistry(ctx, t, nodeID, project, version, replicaID, generation)
	if err != nil {
		t.Fatal(err)
	}
	bucket := fmt.Sprintf("s3://cellp-celld/%s/%s", project, version)

	pki := mustIntegrationPKI(t)
	bind, advertise := freeTCPBind(t)
	nd := &scriptedNodeReg{}

	platformCfg := config.Config{
		CelldBasePort: basePort,
		S3Endpoint:    endpoint,
		S3Region:      region,
		S3AccessKey:   accessKey,
		S3SecretKey:   secretKey,
	}
	elasticCfg := integrationElasticCfg(bind, advertise, pki)
	elasticCfg.CelldBindHost = bindHost
	elasticCfg.CelldAdvertiseHost = advertiseHost

	deps := integrationDepsRealCelld(store, pki, nd)

	runCtx, stopAgent := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- Run(runCtx, elasticCfg, platformCfg, deps) }()

	waitUntil(t, 15*time.Second, func() bool { return nd.heartbeatCalls.Load() >= 1 })

	client, err := agenttransport.NewClient(agenttransport.ClientConfig{
		BaseURL: advertise, ExpectedNodeURI: pki.NodeURI, ControllerIdentityURI: pki.ControllerURI,
		TLS: agenttransport.TLSMaterials{Cert: pki.ClientCert, RootCAs: pki.RootPool, ServerName: "127.0.0.1"},
	})
	if err != nil {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatal(err)
	}

	startScope := agentCommandScope(nodeID, project, version, replicaID, generation, leaseExpiry, contract.ActionStartReplica, "start-1")
	spec := contract.StartReplicaSpec{Scope: startScope, Bucket: bucket}

	ready, err := client.StartReplica(ctx, spec, "cmd-start-1")
	if err != nil || ready.State != contract.ReplicaReady {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatalf("StartReplica: %+v err=%v", ready, err)
	}
	port := expectedCelldPort(platformCfg, elasticCfg, project, version)
	assertCelldHealthyOnBindHost(t, ctx, bindHost, port)

	probeScope := agentCommandScope(nodeID, project, version, replicaID, generation, leaseExpiry, contract.ActionProbeReplica, "probe-1")
	probe, err := client.ProbeReplica(ctx, probeScope)
	if err != nil || probe.State != contract.ReplicaReady {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatalf("ProbeReplica: %+v err=%v", probe, err)
	}

	listScope := agentCommandScope(nodeID, project, version, "", generation, leaseExpiry, contract.ActionListReplicas, "list-1")
	list, err := client.ListReplicas(ctx, listScope)
	if err != nil || len(list) != 1 || list[0].State != contract.ReplicaReady {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatalf("ListReplicas: %+v err=%v", list, err)
	}

	drainScope := agentCommandScope(nodeID, project, version, replicaID, generation, leaseExpiry, contract.ActionDrainReplica, "drain-1")
	drained, err := client.DrainReplica(ctx, drainScope, time.Now().UTC().Add(30*time.Second))
	if err != nil {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatalf("DrainReplica: %v", err)
	}
	if drained.State != contract.ReplicaStopped {
		t.Fatalf("unexpected drain state: %s", drained.State)
	}
	waitForPortClosedOnHost(t, bindHost, port, 30*time.Second)

	stopScope := agentCommandScope(nodeID, project, version, replicaID, generation, leaseExpiry, contract.ActionStopReplica, "stop-1")
	stopped, err := client.StopReplica(ctx, stopScope)
	if err != nil {
		stopAgent()
		_ = waitRunDone(done, 10*time.Second)
		t.Fatalf("StopReplica: %v", err)
	}
	if stopped.State != contract.ReplicaStopped {
		t.Fatalf("StopReplica state=%s", stopped.State)
	}
	waitForPortClosedOnHost(t, bindHost, port, 30*time.Second)

	stopAgent()
	if err := waitRunDone(done, 15*time.Second); err != nil && !isOnlyContextCancellation(err) {
		t.Fatalf("agent shutdown: %v", err)
	}
}

func integrationDepsRealCelld(store *registry.SQLiteStore, pki integrationPKI, nd *scriptedNodeReg) Deps {
	adapter := agent.RegistryStores{Store: store}
	return Deps{
		LoadTLS: func(cfg config.ElasticConfig) (TLSBundle, error) {
			client := pki.clientTLS()
			client.ServerName = cfg.ResolvedControllerTLSServerName()
			return TLSBundle{
				Server: agenttransport.TLSMaterials{Cert: pki.ServerCert, RootCAs: pki.RootPool},
				Client: client,
			}, nil
		},
		DialNodeReg: func(_ config.ElasticConfig, _ agenttransport.TLSMaterials) (nodeRegistrationClient, error) {
			return nd, nil
		},
		DialRegistryRelay: func(_ config.ElasticConfig, _ agenttransport.TLSMaterials) (*registryrelaytransport.Client, error) {
			return nil, nil
		},
		LifecycleStores: func(_ *registryrelaytransport.Client, _ config.ElasticConfig) (agent.NodeStore, agent.LifecycleStore) {
			return adapter, adapter
		},
	}
}

func seedRealAgentRegistry(ctx context.Context, t *testing.T, nodeID, project, version, replicaID string, generation int64) (*registry.SQLiteStore, time.Time, error) {
	store, err := registry.OpenWithOptions(t.TempDir()+"/real-agent-celld.sqlite", registry.OpenOptions{})
	if err != nil {
		return nil, time.Time{}, err
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.CreateProject(ctx, registry.CreateProjectInput{ID: project}); err != nil {
		return nil, time.Time{}, err
	}
	if _, err := store.CreateVersion(ctx, registry.CreateVersionInput{ID: version, ProjectID: project}); err != nil {
		return nil, time.Time{}, err
	}
	if err := store.CompareAndSetDesired(ctx, project, version, 0, registry.ServingDesireRow{DesiredReplicas: 1, Generation: generation}); err != nil {
		return nil, time.Time{}, err
	}

	leaseExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	nodeLease := leaseExpiry
	if err := store.UpsertRuntimeNode(ctx, contract.RuntimeNode{
		NodeID: nodeID, CapacityUnits: 2, Generation: generation, LeaseExpiry: nodeLease,
	}); err != nil {
		return nil, time.Time{}, err
	}
	if err := store.ClaimAssignment(ctx, registry.AssignmentClaim{
		ReplicaID: replicaID, ProjectID: project, VersionID: version, NodeID: nodeID,
		Generation: generation, ExpectedNodeGeneration: generation, ValidUntil: leaseExpiry,
	}); err != nil {
		return nil, time.Time{}, err
	}
	return store, leaseExpiry, nil
}

func agentCommandScope(nodeID, project, version, replicaID string, generation int64, leaseExpiry time.Time, action contract.LifecycleAction, nonce string) contract.CommandScope {
	return contract.CommandScope{
		NodeID: nodeID, ProjectID: project, VersionID: version, ReplicaID: replicaID,
		Generation: generation, LeaseExpiry: leaseExpiry, Nonce: nonce, Action: action,
	}
}

func ensureLoopbackAliasReachable(host string) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return err
	}
	_ = ln.Close()
	return nil
}

func assertCelldHealthyOnBindHost(t *testing.T, ctx context.Context, host string, port int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if probeCelldHealthOnHost(ctx, host, port) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("celld health not ready on %s:%d", host, port)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitForPortClosedOnHost(t *testing.T, host string, port int, timeout time.Duration) {
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

func expectedCelldPort(platformCfg config.Config, elasticCfg config.ElasticConfig, project, version string) int {
	m := DefaultManager(platformCfg, elasticCfg)
	return m.AllocatePort(project, version)
}

func probeCelldHealthOnHost(ctx context.Context, host string, port int) bool {
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
