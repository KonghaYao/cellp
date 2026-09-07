package runtime

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
