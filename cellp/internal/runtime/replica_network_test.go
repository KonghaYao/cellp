package runtime

import (
	"context"
	"testing"
)

func TestReplicaNetworkDefaultCompatible(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	host, port, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err != nil || host != "127.0.0.1" || port <= 0 {
		t.Fatalf("host=%q port=%d err=%v", host, port, err)
	}
	inst, err := m.ProbeReplica(ctx, key)
	if err != nil || inst.Host != "127.0.0.1" {
		t.Fatalf("probe=%+v err=%v", inst, err)
	}
}

func TestReplicaNetworkAdvertiseBindSplit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	m := New(8792, "", "us-east-1", "s3://cellp-celld", "k", "s")
	m.SetReplicaHostConfig(ReplicaHostConfig{BindHost: "10.0.0.5", AdvertiseHost: "node-a.internal"})
	ctx := context.Background()
	key := ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	host, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err != nil || host != "node-a.internal" {
		t.Fatalf("host=%q err=%v", host, err)
	}
	host2, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err != nil || host2 != "node-a.internal" {
		t.Fatalf("idempotent host=%q err=%v", host2, err)
	}
	inst, err := m.ProbeReplica(ctx, key)
	if err != nil || inst.Host != "node-a.internal" {
		t.Fatalf("probe=%+v err=%v", inst, err)
	}
	if got := m.ListReplicas(ctx); len(got) != 1 || got[0].Host != "node-a.internal" {
		t.Fatalf("list=%+v", got)
	}
}
