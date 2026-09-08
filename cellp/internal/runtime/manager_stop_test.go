package runtime

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStopAfterStartWithoutCelld(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	if _, _, err := m.Start(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(ctx, "demo", "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestStopAllTerminatesTrackedProcesses(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	commands := make([]*exec.Cmd, 0, 2)
	for _, version := range []string{"v1", "v2"} {
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
		m.processes[m.key("demo", version)] = &celldProc{cmd: cmd}
	}

	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range commands {
		if processAlive(cmd) {
			t.Fatalf("process %d still alive", cmd.Process.Pid)
		}
	}
	if len(m.processes) != 0 {
		t.Fatalf("tracked processes remain: %d", len(m.processes))
	}
}

func TestStopAllMixedInventoryReleasesElasticPorts(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://ignored", "k", "s")
	ctx := context.Background()
	if _, _, err := m.Start(ctx, "demo", "legacy"); err != nil {
		t.Fatal(err)
	}
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	_, port, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.StopAll(ctx); err != nil {
		t.Fatal(err)
	}
	if len(m.processes) != 0 || len(m.ports) != 0 || len(m.lifecycle) != 0 {
		t.Fatalf("inventory retained: processes=%d ports=%d lifecycle=%d", len(m.processes), len(m.ports), len(m.lifecycle))
	}
	_, reused, err := m.StartReplica(ctx, ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r2"}, "s3://cellp-celld/demo/v1")
	if err != nil {
		t.Fatal(err)
	}
	if reused >= port {
		t.Fatalf("released inventory ports not reusable: old=%d new=%d", port, reused)
	}
}

func TestStopReplicaCompletesWithCanceledCallerContext(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r-canceled"}
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	k := m.replicaKey(key)
	m.processes[k] = &celldProc{cmd: cmd, port: 9911}
	m.ports[k] = 9911
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.StopReplica(ctx, key); err != nil {
		t.Fatal(err)
	}
	if processAlive(cmd) {
		t.Fatal("process still alive")
	}
	if _, ok := m.processes[k]; ok {
		t.Fatal("process tracking retained after confirmed exit")
	}
	if _, ok := m.ports[k]; ok {
		t.Fatal("port retained after confirmed exit")
	}
}

func TestStartReadinessCancellationReleasesProcessWatchAndPort(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexec /bin/sleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	watchRoot := t.TempDir()
	t.Setenv("CELLP_CELLD_WATCH_TMP", watchRoot)
	t.Setenv("CELLP_SKIP_CELLD_DIAGNOSE", "1")
	t.Setenv("CELLP_CELLD_PORT_SETTLE", "50ms")
	m := New(48982, "", "us-east-1", "s3://ignored", "k", "s")
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err == nil {
		t.Fatal("canceled readiness unexpectedly succeeded")
	}
	k := m.replicaKey(key)
	if _, ok := m.processes[k]; ok {
		t.Fatal("failed start retained process")
	}
	if _, ok := m.ports[k]; ok {
		t.Fatal("failed start retained port")
	}
	entries, err := os.ReadDir(watchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed start retained watch: entries=%d err=%v", len(entries), err)
	}
	t.Setenv("PATH", t.TempDir())
	_, reused, err := m.StartReplica(context.Background(), key, "s3://cellp-celld/demo/v1")
	wantPort := m.basePort + 11
	if err != nil || reused != wantPort {
		t.Fatalf("restart did not reuse first free port: want=%d new=%d err=%v", wantPort, reused, err)
	}
}

func TestStopPropagatesWatchRemovalErrorAfterUntracking(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://ignored", "k", "s")
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	k := m.replicaKey(key)
	m.processes[k] = &celldProc{port: 9901, watchDir: "/non-sensitive/test-watch"}
	m.ports[k] = 9901
	original := removeEphemeralWatch
	removeEphemeralWatch = func(string) error { return errors.New("remove watch failed") }
	t.Cleanup(func() { removeEphemeralWatch = original })
	if err := m.StopReplica(context.Background(), key); err == nil {
		t.Fatal("watch deletion error was swallowed")
	}
	if _, ok := m.processes[k]; ok {
		t.Fatal("confirmed-stopped process remained tracked")
	}
	if _, ok := m.ports[k]; ok {
		t.Fatal("confirmed-stopped port remained tracked")
	}
}

func TestWaitForTCPPortFreeTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	start := time.Now()
	if err := waitForTCPPortFree("127.0.0.1", port, 25*time.Millisecond); err == nil {
		t.Fatal("expected occupied port error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("port wait took %v", elapsed)
	}
}

func TestWaitForTCPPortFreeSucceeds(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := waitForTCPPortFree("127.0.0.1", port, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestStartOnPortRejectsOccupiedPort(t *testing.T) {
	t.Setenv("CELLP_CELLD_PORT_SETTLE", "500ms")
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "celld"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	_, _, err = m.StartOnPort(context.Background(), "demo", "v1", "127.0.0.1", port)
	if err == nil || !strings.Contains(err.Error(), "still in use") {
		t.Fatalf("expected occupied port error, got %v", err)
	}
}

func TestLifecycleLockSerializesAndCleansUp(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	unlock := m.lockLifecycle("demo", "v1")
	started := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		close(started)
		unlockSecond := m.lockLifecycle("demo", "v1")
		close(acquired)
		unlockSecond()
	}()
	<-started
	select {
	case <-acquired:
		t.Fatal("second lifecycle operation was not serialized")
	case <-time.After(25 * time.Millisecond):
	}
	unlock()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second lifecycle operation did not resume")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.lifecycle) != 0 {
		t.Fatalf("lifecycle locks leaked: %d", len(m.lifecycle))
	}
}

func TestSetWorkerEnvLoader(t *testing.T) {
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	m.SetWorkerEnvLoader(func(ctx context.Context, project, version string) (map[string]string, error) {
		return map[string]string{"GREETING": "hi"}, nil
	})
	if m == nil {
		t.Fatal("nil manager")
	}
}
