package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ReplicaKey identifies one elastic celld instance independently of the legacy
// project/version instance.
type ReplicaKey struct {
	ProjectID string
	VersionID string
	ReplicaID string
}

// ReplicaInstance is non-sensitive local process inventory.
type ReplicaInstance struct {
	Key     ReplicaKey
	Host    string
	Port    int
	Healthy bool
}

var safeReplicaPathID = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]{0,126}[A-Za-z0-9])?$`)

func (k ReplicaKey) validate() error {
	for name, value := range map[string]string{
		"project_id": k.ProjectID,
		"version_id": k.VersionID,
		"replica_id": k.ReplicaID,
	} {
		if !safeReplicaPathID.MatchString(value) {
			return fmt.Errorf("%s is not a safe runtime identifier", name)
		}
	}
	return nil
}

// DiagnoseReplica probes storage before an elastic replica start.
func (m *Manager) DiagnoseReplica(ctx context.Context, key ReplicaKey, bucket string) error {
	if err := key.validate(); err != nil {
		return err
	}
	if err := m.ValidateReplicaBucket(key, bucket); err != nil {
		return err
	}
	return m.diagnoseBucket(ctx, bucket)
}

// StartReplica starts an isolated process/watch/port for one replica.
func (m *Manager) StartReplica(ctx context.Context, key ReplicaKey, bucket string) (string, int, error) {
	if err := key.validate(); err != nil {
		return "", 0, err
	}
	if err := m.ValidateReplicaBucket(key, bucket); err != nil {
		return "", 0, err
	}
	k := m.replicaKey(key)
	port := m.allocatePortKey(k)
	unlock := m.lockLifecycleKey(k)
	defer unlock()
	bindHost, advertiseHost := m.replicaBindAdvertise()
	return m.startManagedOnPortLocked(ctx, k, key.ProjectID, key.VersionID, replicaComponent(key.VersionID, key.ReplicaID), bucket, bindHost, advertiseHost, port)
}

// ProbeReplica checks both tracked process ownership and celld health.
func (m *Manager) ProbeReplica(ctx context.Context, key ReplicaKey) (ReplicaInstance, error) {
	if err := key.validate(); err != nil {
		return ReplicaInstance{}, err
	}
	k := m.replicaKey(key)
	m.mu.Lock()
	proc, ok := m.processes[k]
	m.mu.Unlock()
	if !ok || proc == nil {
		return ReplicaInstance{}, fmt.Errorf("replica not running")
	}
	alive := proc.cmd == nil || processAlive(proc.cmd)
	inst := ReplicaInstance{Key: key, Host: proc.externalHost(), Port: proc.port}
	inst.Healthy = alive && m.Health(ctx, proc.bindHostOrDefault(), inst.Port)
	return inst, nil
}

// DrainReplica implements the currently available celld semantics: there is no
// graceful drain RPC, so the caller first withdraws the endpoint and this method
// stops the process. An expired deadline still forces the same fail-closed stop.
func (m *Manager) DrainReplica(ctx context.Context, key ReplicaKey, deadline time.Time) error {
	return m.StopReplica(ctx, key)
}

// StopReplica stops one elastic process and releases its port/watch state.
func (m *Manager) StopReplica(ctx context.Context, key ReplicaKey) error {
	if err := key.validate(); err != nil {
		return err
	}
	k := m.replicaKey(key)
	unlock := m.lockLifecycleKey(k)
	defer unlock()
	return m.stopManagedLocked(ctx, k, true)
}

// ListReplicas returns only elastic instances currently tracked by this manager.
func (m *Manager) ListReplicas(ctx context.Context) []ReplicaInstance {
	m.mu.Lock()
	keys := make([]string, 0, len(m.processes))
	for k := range m.processes {
		if strings.Contains(k, "/replicas/") {
			keys = append(keys, k)
		}
	}
	m.mu.Unlock()

	out := make([]ReplicaInstance, 0, len(keys))
	for _, raw := range keys {
		parts := strings.SplitN(raw, "/", 4)
		if len(parts) != 4 || parts[2] != "replicas" {
			continue
		}
		inst, err := m.ProbeReplica(ctx, ReplicaKey{ProjectID: parts[0], VersionID: parts[1], ReplicaID: parts[3]})
		if err == nil {
			out = append(out, inst)
		}
	}
	return out
}

func replicaComponent(versionID, replicaID string) string {
	encode := func(v string) string { return base64.RawURLEncoding.EncodeToString([]byte(v)) }
	return encode(versionID) + "." + encode(replicaID)
}

func (m *Manager) allocatePortKey(k string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.ports[k]; ok {
		return p
	}
	used := make(map[int]struct{}, len(m.ports))
	for _, p := range m.ports {
		used[p] = struct{}{}
	}
	for n := 1; n <= 1000; n++ {
		port := m.basePort + 10 + n
		if _, taken := used[port]; taken {
			continue
		}
		m.ports[k] = port
		if n > m.nextN {
			m.nextN = n
		}
		return port
	}
	m.nextN++
	port := m.basePort + 10 + m.nextN
	m.ports[k] = port
	return port
}
