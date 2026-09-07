package runtime

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

	"github.com/cellp/cellp/internal/health"
)

const (
	envRealCelldTest     = "CELLP_REAL_CELLD_TEST"
	envRealCelldProject  = "CELLP_REAL_CELLD_PROJECT"
	envRealCelldVersion  = "CELLP_REAL_CELLD_VERSION"
	envRealCelldReplica  = "CELLP_REAL_CELLD_REPLICA"
	envRealCelldBasePort = "CELLP_REAL_CELLD_BASE_PORT"
)

func TestReplicaRealCelldLifecycle(t *testing.T) {
	if os.Getenv(envRealCelldTest) != "1" {
		t.Skip("set CELLP_REAL_CELLD_TEST=1 to run real celld lifecycle acceptance")
	}
	if !CelldInstalled() {
		t.Fatal("real celld lifecycle requires celld on PATH")
	}

	project := strings.TrimSpace(os.Getenv(envRealCelldProject))
	version := strings.TrimSpace(os.Getenv(envRealCelldVersion))
	replicaID := strings.TrimSpace(os.Getenv(envRealCelldReplica))
	if project == "" || version == "" || replicaID == "" {
		t.Fatal("real celld lifecycle requires CELLP_REAL_CELLD_PROJECT, CELLP_REAL_CELLD_VERSION, and CELLP_REAL_CELLD_REPLICA")
	}

	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	accessKey := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY"))
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Fatal("real celld lifecycle requires S3_ENDPOINT, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY")
	}
	if region == "" {
		region = "us-east-1"
	}

	basePort := 18992
	if raw := strings.TrimSpace(os.Getenv(envRealCelldBasePort)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			t.Fatalf("invalid %s: %q", envRealCelldBasePort, raw)
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

	m := New(basePort, endpoint, region, "", accessKey, secretKey)
	m.SetReplicaHostConfig(ReplicaHostConfig{
		BindHost:      bindHost,
		AdvertiseHost: advertiseHost,
	})

	key := ReplicaKey{ProjectID: project, VersionID: version, ReplicaID: replicaID}
	bucket, err := m.ExpectedReplicaBucket(key)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = m.StopReplica(cleanupCtx, key)
	})

	if err := m.DiagnoseReplica(ctx, key, bucket); err != nil {
		t.Fatalf("DiagnoseReplica: %v", err)
	}

	host, port, err := m.StartReplica(ctx, key, bucket)
	if err != nil {
		t.Fatalf("StartReplica: %v", err)
	}
	if host != advertiseHost {
		t.Fatalf("StartReplica host=%q want advertise=%q", host, advertiseHost)
	}
	assertCelldHealthyOnBind(t, ctx, bindHost, port)
	if bindHost != "127.0.0.1" {
		assertCelldUnreachableOnHost(t, ctx, "127.0.0.1", port)
	}

	probe, err := m.ProbeReplica(ctx, key)
	if err != nil {
		t.Fatalf("ProbeReplica: %v", err)
	}
	if probe.Host != advertiseHost || !probe.Healthy || probe.Port != port {
		t.Fatalf("ProbeReplica=%+v", probe)
	}

	list := m.ListReplicas(ctx)
	if len(list) != 1 || list[0].Host != advertiseHost || list[0].Port != port || !list[0].Healthy {
		t.Fatalf("ListReplicas=%+v", list)
	}

	if err := m.DrainReplica(ctx, key, time.Now().Add(15*time.Second)); err != nil {
		t.Fatalf("DrainReplica: %v", err)
	}
	waitForPortClosed(t, bindHost, port, 20*time.Second)

	host2, port2, err := m.StartReplica(ctx, key, bucket)
	if err != nil {
		t.Fatalf("restart StartReplica: %v", err)
	}
	if host2 != advertiseHost || port2 != port {
		t.Fatalf("restart endpoint changed: host=%q port=%d want host=%q port=%d", host2, port2, advertiseHost, port)
	}
	assertCelldHealthyOnBind(t, ctx, bindHost, port2)

	host3, port3, err := m.StartReplica(ctx, key, bucket)
	if err != nil {
		t.Fatalf("idempotent StartReplica: %v", err)
	}
	if host3 != advertiseHost || port3 != port2 {
		t.Fatalf("idempotent endpoint changed: host=%q port=%d want host=%q port=%d", host3, port3, advertiseHost, port2)
	}

	if err := m.StopReplica(ctx, key); err != nil {
		t.Fatalf("StopReplica: %v", err)
	}
	waitForPortClosed(t, bindHost, port2, 20*time.Second)
}

func ensureLoopbackAliasReachable(host string) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return err
	}
	_ = ln.Close()
	return nil
}

func assertCelldHealthyOnBind(t *testing.T, ctx context.Context, host string, port int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if probeCelldHealth(ctx, host, port) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("celld health not ready on %s:%d", host, port)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func assertCelldUnreachableOnHost(t *testing.T, ctx context.Context, host string, port int) {
	t.Helper()
	if probeCelldHealth(ctx, host, port) {
		t.Fatalf("unexpected celld health on %s:%d", host, port)
	}
}

func waitForPortClosed(t *testing.T, host string, port int, timeout time.Duration) {
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

func probeCelldHealth(ctx context.Context, host string, port int) bool {
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
	body, _ := ioReadAllLimit(resp.Body, 4096)
	return health.CelldHealthResponseOK(resp.StatusCode, body)
}

func ioReadAllLimit(r io.Reader, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("invalid limit")
	}
	return io.ReadAll(io.LimitReader(r, n))
}
