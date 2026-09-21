package runtime

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReplicaManagerIsolatesSameVersionReplicasAndReleasesPorts(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	k1 := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	k2 := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r2"}

	_, p1, err := m.StartReplica(ctx, k1, "s3://cellp-celld/demo/v1")
	if err != nil {
		t.Fatal(err)
	}
	_, p2, err := m.StartReplica(ctx, k2, "s3://cellp-celld/demo/v1")
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("replica ports collided: %d", p1)
	}
	if got := m.ListReplicas(ctx); len(got) != 2 {
		t.Fatalf("inventory: %+v", got)
	}
	if err := m.StopReplica(ctx, k1); err != nil {
		t.Fatal(err)
	}
	if got := m.ListReplicas(ctx); len(got) != 1 || got[0].Key.ReplicaID != "r2" {
		t.Fatalf("post-stop inventory: %+v", got)
	}
	_, p3, err := m.StartReplica(ctx, ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r3"}, "s3://cellp-celld/demo/v1")
	if err != nil {
		t.Fatal(err)
	}
	if p3 != p1 {
		t.Fatalf("released port not reusable: old=%d new=%d", p1, p3)
	}
	if legacy := m.AllocatePort("demo", "v1"); legacy == p2 || legacy == p3 {
		t.Fatalf("legacy port collided: %d", legacy)
	}
}

func TestReplicaKeysAndLogsDoNotCollide(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	a := ReplicaKey{ProjectID: "demo", VersionID: "a-b", ReplicaID: "c"}
	b := ReplicaKey{ProjectID: "demo", VersionID: "a", ReplicaID: "b-c"}
	if m.replicaKey(a) == m.replicaKey(b) {
		t.Fatal("replica keys collided")
	}
	if celldLogPath(a.ProjectID, replicaComponent(a.VersionID, a.ReplicaID)) == celldLogPath(b.ProjectID, replicaComponent(b.VersionID, b.ReplicaID)) {
		t.Fatal("replica logs collided")
	}
	wantDir := filepath.Dir(celldLogPath("safe", "safe"))
	if got := filepath.Dir(celldLogPath("../escape", "v/../../x")); got != wantDir {
		t.Fatalf("log escaped temp directory: %s", got)
	}
}

func TestReplicaBucketMustBeCanonical(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://ignored-single-project", "k", "s")
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	if got, err := m.ExpectedReplicaBucket(key); err != nil || got != "s3://cellp-celld/demo/v1" {
		t.Fatalf("canonical bucket=%q err=%v", got, err)
	}
	for _, bucket := range []string{
		"s3://cellp-celld/demo/v1/../other",
		"s3://cellp-celld/demo/v1?x=1",
		"s3://cellp-celld/demo/v10",
		"s3://cellp-celld/demo/v1/",
	} {
		if err := m.ValidateReplicaBucket(key, bucket); err == nil {
			t.Fatalf("accepted non-canonical bucket %q", bucket)
		}
	}
}

func TestReplicaRuntimeIdentifiersFailClosed(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://ignored", "k", "s")
	bad := []string{"a/b+c", "a+b/c", ".", "..", "%2F", "?", "#", "雪", "/prefix", "trailing/", `back\\slash`, "a.b", "-prefix", "trailing-"}
	for _, value := range bad {
		key := ReplicaKey{ProjectID: value, VersionID: "v1", ReplicaID: "r1"}
		if _, err := m.ExpectedReplicaBucket(key); err == nil {
			t.Fatalf("accepted unsafe project_id %q", value)
		}
		key = ReplicaKey{ProjectID: "demo", VersionID: value, ReplicaID: "r1"}
		if _, err := m.ExpectedReplicaBucket(key); err == nil {
			t.Fatalf("accepted unsafe version_id %q", value)
		}
	}
	for _, value := range []string{"a", "A1", "a-b", "a_b", "a-b_C9"} {
		key := ReplicaKey{ProjectID: value, VersionID: value, ReplicaID: value}
		if _, err := m.ExpectedReplicaBucket(key); err != nil {
			t.Fatalf("rejected safe identifier %q: %v", value, err)
		}
	}
}

func TestCappedContextUsesShorterDeadline(t *testing.T) {
	caller, cancelCaller := context.WithTimeout(context.Background(), time.Hour)
	defer cancelCaller()
	ctx, cancel := cappedContext(caller, 20*time.Millisecond)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 100*time.Millisecond {
		t.Fatalf("internal cap not applied: %v", deadline)
	}

	shortCaller, cancelShort := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelShort()
	ctx, cancel = cappedContext(shortCaller, time.Hour)
	defer cancel()
	deadline, ok = ctx.Deadline()
	if !ok || time.Until(deadline) > 100*time.Millisecond {
		t.Fatalf("caller deadline not preserved: %v", deadline)
	}
}

func TestHealthIPv6Listener(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Shutdown(context.Background())

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	m := New(8792, "", "us-east-1", "s3://ignored", "k", "s")
	if !m.Health(context.Background(), "::1", listener.Addr().(*net.TCPAddr).Port) {
		t.Fatal("IPv6 health probe failed")
	}
}

// A per-version env change restarts the serving processes. In the single scheduler+agent
// track those are the elastic replicas, so Restart must re-spawn them on their own ports
// instead of spinning up the legacy project/version key that nothing serves from.
func TestManagerRestartTargetsElasticReplicas(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	if _, port, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1"); err != nil {
		t.Fatal(err)
	} else if port <= 0 {
		t.Fatalf("replica port: %d", port)
	}
	before, err := m.ProbeReplica(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(ctx, "demo", "v1"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	after, err := m.ProbeReplica(ctx, key)
	if err != nil {
		t.Fatalf("replica lost by restart: %v", err)
	}
	if after.Port != before.Port {
		t.Fatalf("restart moved the replica port: before=%d after=%d", before.Port, after.Port)
	}
	if got := m.ListReplicas(ctx); len(got) != 1 || got[0].Key.ReplicaID != "r1" {
		t.Fatalf("replica inventory after restart: %+v", got)
	}
	if _, ok := m.ports[m.key("demo", "v1")]; ok {
		t.Fatal("restart allocated the legacy project/version key behind the elastic track")
	}
}

// A restart must never bring back a replica that was stopped while the env change was
// being applied: the reconciler, the scheduler or an archive can stop the process
// between the snapshot and the lifecycle lock, and re-spawning then would create a
// process for an identity the registry already terminalized.
func TestManagerRestartDoesNotResurrectStoppedReplica(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	if _, port, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1"); err != nil {
		t.Fatal(err)
	} else if port <= 0 {
		t.Fatalf("replica port: %d", port)
	}
	// The stop lands between the restart's snapshot and its lock.
	if err := m.StopReplica(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(ctx, "demo", "v1"); err != nil {
		t.Fatalf("restart after stop: %v", err)
	}
	if got := m.ListReplicas(ctx); len(got) != 0 {
		t.Fatalf("restart resurrected a stopped replica: %+v", got)
	}
	m.mu.Lock()
	_, tracked := m.processes[m.replicaKey(key)]
	m.mu.Unlock()
	if tracked {
		t.Fatal("restart recreated the stopped replica's process entry")
	}
}

// Operator commands that reach a version's namespace through a live celld must wake a
// cold version first and drop the wake pin afterwards, so scale-to-zero applies again.
// Bucket-level commands (the D1 CLI) do not need the fleet and must not wake anything.
func TestManagerOperatorCLIEnsuresServingAndReleases(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	var wakes, releases int
	m.SetEnsureServing(func(_ context.Context, project, version string) (func(), error) {
		wakes++
		return func() { releases++ }, nil
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.jsonc"), []byte(
		`{"name":"app","kv_namespaces":[{"binding":"KV","id":"ns"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// celld is absent, so the command itself fails; the wake and the release still run.
	_, _ = m.KvGet(context.Background(), "demo", "v1", dir, "ns", "key")
	if wakes != 1 || releases != 1 {
		t.Fatalf("operator command did not pin and release the fleet: wakes=%d releases=%d", wakes, releases)
	}
	_, _ = m.KvList(context.Background(), "demo", "v1", dir, "ns", "", "", 0)
	if wakes != 2 || releases != 2 {
		t.Fatalf("second operator command did not pin and release: wakes=%d releases=%d", wakes, releases)
	}
}

// A mutation may only be re-sent when celld answered before dispatching the work. A lost
// response after dispatch is not retried (it would double-apply), while a read may be.
func TestExecCelldOnFleetRetriesOnlyPreDispatchForMutations(t *testing.T) {
	dir := t.TempDir()
	writeStub := func(name, script string) {
		bin := filepath.Join(dir, name)
		if err := os.Mkdir(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "celld"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 1. Pre-dispatch refusal: nothing ran, so a mutation is retried after a fresh wake.
	writeStub("predispatch", `#!/bin/sh
state="$(dirname "$0")/count"
count="$(cat "$state" 2>/dev/null || echo 0)"
count=$((count + 1))
echo "$count" > "$state"
if [ "$count" -lt 2 ]; then
  echo "Error: no node leases in the bucket; celld kv needs a running fleet" >&2
  exit 1
fi
echo done
`)
	// 2. Transport lost after dispatch: the request may have reached a node.
	writeStub("transport", `#!/bin/sh
state="$(dirname "$0")/count"
count="$(cat "$state" 2>/dev/null || echo 0)"
count=$((count + 1))
echo "$count" > "$state"
echo "Error: reach the namespace at http://127.0.0.1:1/runtime/__KvNamespace:x" >&2
echo "Caused by: 0: error sending request for url (http://127.0.0.1:1/runtime/__KvNamespace:x)" >&2
exit 1
`)
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	var wakes, releases int
	m.SetEnsureServing(func(context.Context, string, string) (func(), error) {
		wakes++
		return func() { releases++ }, nil
	})
	m.fleetRetryGap = 0

	t.Setenv("PATH", filepath.Join(dir, "predispatch"))
	out, err := m.execCelldOnFleet(context.Background(), "demo", "v1", []string{"kv", "put", "ns", "k"}, false)
	if err != nil || strings.TrimSpace(string(out)) != "done" {
		t.Fatalf("pre-dispatch retry: out=%q err=%v", out, err)
	}
	if wakes != 2 || releases != 2 {
		t.Fatalf("pre-dispatch retry did not re-wake and re-release: wakes=%d releases=%d", wakes, releases)
	}

	t.Setenv("PATH", filepath.Join(dir, "transport"))
	wakes, releases = 0, 0
	if _, err := m.execCelldOnFleet(context.Background(), "demo", "v1", []string{"kv", "put", "ns", "k"}, false); err == nil {
		t.Fatal("post-dispatch mutation failure must surface")
	}
	if wakes != 1 {
		t.Fatalf("mutation was retried after the request was dispatched: wakes=%d", wakes)
	}
	attempts, err := os.ReadFile(filepath.Join(dir, "transport", "count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(attempts)) != "1" {
		t.Fatalf("mutation reached celld %s times", strings.TrimSpace(string(attempts)))
	}
	// The same failure is retried for a read, which is idempotent.
	if _, err := m.execCelldOnFleet(context.Background(), "demo", "v1", []string{"kv", "get", "ns", "k"}, true); err == nil {
		t.Fatal("read failure must surface after its attempts")
	}
	attempts, err = os.ReadFile(filepath.Join(dir, "transport", "count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(attempts)) == "1" {
		t.Fatal("idempotent read was not retried after a lost response")
	}
}

// Restart and StopReplica serialize on the same replica lifecycle lock: whichever lands
// second sees the other's effect. A restart that loses the race must not re-create the
// process the stop removed, which is what a snapshot-then-act restart would do.
func TestManagerRestartConcurrentWithStopDoesNotResurrect(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	if _, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	start := make(chan struct{})
	stopErr := make(chan error, 1)
	restartErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		<-start
		stopErr <- m.StopReplica(ctx, key)
	}()
	go func() {
		defer wg.Done()
		<-start
		restartErr <- m.Restart(ctx, "demo", "v1")
	}()
	close(start)
	wg.Wait()
	if err := <-stopErr; err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := <-restartErr; err != nil {
		t.Fatalf("restart: %v", err)
	}
	m.mu.Lock()
	_, tracked := m.processes[m.replicaKey(key)]
	m.mu.Unlock()
	if tracked {
		t.Fatal("restart resurrected the replica its concurrent stop removed")
	}
}

// An inventory read must not fall into the middle of a restart: reconcile turns a live
// replica that looks gone into a terminal assignment, so the read shares the replica's
// lifecycle lock with restart and stop.
func TestListReplicasTakesReplicaLifecycleLock(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	if _, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1"); err != nil {
		t.Fatal(err)
	}
	unlock := m.lockLifecycleKey(m.replicaKey(key))
	done := make(chan []ReplicaInstance, 1)
	go func() { done <- m.ListReplicas(ctx) }()
	select {
	case <-done:
		unlock()
		t.Fatal("inventory read observed a replica while its lifecycle was held")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case got := <-done:
		if len(got) != 1 || got[0].Key.ReplicaID != "r1" {
			t.Fatalf("inventory after release: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inventory read did not resume after the lifecycle lock was released")
	}
}
