package agentrun

import (
	"context"
	"testing"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/agent"
	"github.com/cellp/cellp/internal/runtime"
)

func TestDefaultBackendIsManagerBackend(t *testing.T) {
	m := DefaultManager(config.Config{}, config.ElasticConfig{})
	var d Deps
	b := d.backend(m)
	mb, ok := b.(agent.ManagerBackend)
	if !ok {
		t.Fatalf("production Deps must use agent.ManagerBackend, got %T", b)
	}
	if mb.Manager != m {
		t.Fatal("ManagerBackend must wrap the same manager instance")
	}
}

func TestDefaultManagerAppliesReplicaHostConfig(t *testing.T) {
	elastic := config.ElasticConfig{
		Enabled:            true,
		CelldBindHost:      "10.0.0.2",
		CelldAdvertiseHost: "node-b.internal",
	}
	m := DefaultManager(config.Config{}, elastic)
	ctx := context.Background()
	key := runtime.ReplicaKey{ProjectID: "demo", VersionID: "v1", ReplicaID: "r1"}
	t.Setenv("PATH", t.TempDir())
	host, _, err := m.StartReplica(ctx, key, "s3://cellp-celld/demo/v1")
	if err != nil || host != "node-b.internal" {
		t.Fatalf("host=%q err=%v", host, err)
	}
}
