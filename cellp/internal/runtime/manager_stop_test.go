package runtime

import (
	"context"
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
