package contract

import "time"

// LifecycleAction is a transport-neutral Node Agent command class.
type LifecycleAction string

const (
	ActionStartReplica  LifecycleAction = "start_replica"
	ActionProbeReplica  LifecycleAction = "probe_replica"
	ActionDrainReplica  LifecycleAction = "drain_replica"
	ActionStopReplica   LifecycleAction = "stop_replica"
	ActionListReplicas  LifecycleAction = "list_replicas"
)

// CommandScope binds an action to project/version/node and fencing generation.
type CommandScope struct {
	NodeID     string          `json:"node_id"`
	ProjectID  string          `json:"project_id"`
	VersionID  string          `json:"version_id"`
	ReplicaID  string          `json:"replica_id,omitempty"`
	Generation int64           `json:"generation"`
	LeaseExpiry time.Time      `json:"lease_expiry"`
	Nonce      string          `json:"nonce"`
	Action     LifecycleAction `json:"action"`
}

// SecretRef names a credential without embedding material (CF-MTLS).
type SecretRef struct {
	Name string `json:"name"`
}

// StartReplicaSpec is the start payload (watch paths are ephemeral per AD-1).
type StartReplicaSpec struct {
	Scope      CommandScope `json:"scope"`
	Bucket     string       `json:"bucket"`
	SecretRefs []SecretRef  `json:"secret_refs,omitempty"`
}
