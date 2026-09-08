package agent

import (
	"context"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	runtimepkg "github.com/cellp/cellp/internal/runtime"
)

// ManagerBackend adapts runtime.Manager to the elastic lifecycle boundary.
type ManagerBackend struct {
	Manager *runtimepkg.Manager
}

func (b ManagerBackend) ExpectedReplicaBucket(scope contract.CommandScope) (string, error) {
	return b.Manager.ExpectedReplicaBucket(runtimeKey(scope))
}

func (b ManagerBackend) Diagnose(ctx context.Context, spec contract.StartReplicaSpec) error {
	return b.Manager.DiagnoseReplica(ctx, runtimeKey(spec.Scope), spec.Bucket)
}

func (b ManagerBackend) Start(ctx context.Context, spec contract.StartReplicaSpec) (string, int, error) {
	return b.Manager.StartReplica(ctx, runtimeKey(spec.Scope), spec.Bucket)
}

func (b ManagerBackend) Probe(ctx context.Context, scope contract.CommandScope) (BackendReplica, error) {
	inst, err := b.Manager.ProbeReplica(ctx, runtimeKey(scope))
	if err != nil {
		return BackendReplica{}, err
	}
	return BackendReplica{
		ReplicaID: inst.Key.ReplicaID, ProjectID: inst.Key.ProjectID, VersionID: inst.Key.VersionID,
		Host: inst.Host, Port: inst.Port, Healthy: inst.Healthy,
	}, nil
}

func (b ManagerBackend) Drain(ctx context.Context, scope contract.CommandScope, deadline time.Time) error {
	return b.Manager.DrainReplica(ctx, runtimeKey(scope), deadline)
}

func (b ManagerBackend) Stop(ctx context.Context, scope contract.CommandScope) error {
	return b.Manager.StopReplica(ctx, runtimeKey(scope))
}

func (b ManagerBackend) List(ctx context.Context) ([]BackendReplica, error) {
	instances := b.Manager.ListReplicas(ctx)
	out := make([]BackendReplica, 0, len(instances))
	for _, inst := range instances {
		out = append(out, BackendReplica{
			ReplicaID: inst.Key.ReplicaID, ProjectID: inst.Key.ProjectID, VersionID: inst.Key.VersionID,
			Host: inst.Host, Port: inst.Port, Healthy: inst.Healthy,
		})
	}
	return out, nil
}

func runtimeKey(scope contract.CommandScope) runtimepkg.ReplicaKey {
	return runtimepkg.ReplicaKey{ProjectID: scope.ProjectID, VersionID: scope.VersionID, ReplicaID: scope.ReplicaID}
}
