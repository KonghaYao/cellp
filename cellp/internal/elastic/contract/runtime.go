package contract

import "time"

// ReplicaState is the lifecycle of a single celld replica.
type ReplicaState string

const (
	ReplicaPending   ReplicaState = "pending"
	ReplicaStarting  ReplicaState = "starting"
	ReplicaReady     ReplicaState = "ready"
	ReplicaDraining  ReplicaState = "draining"
	ReplicaStopped   ReplicaState = "stopped"
	ReplicaFailed    ReplicaState = "failed"
)

// RuntimeNode is a registered compute node (local or remote).
type RuntimeNode struct {
	NodeID        string    `json:"node_id"`
	CapacityUnits int       `json:"capacity_units"`
	Cordoned      bool      `json:"cordoned"`
	LeaseExpiry   time.Time `json:"lease_expiry"`
	Generation    int64     `json:"generation"`
}

// RuntimeReplica is a placement record (Scheduler writes assignment).
type RuntimeReplica struct {
	ReplicaID  string       `json:"replica_id"`
	ProjectID  string       `json:"project_id"`
	VersionID  string       `json:"version_id"`
	NodeID     string       `json:"node_id"`
	Generation int64        `json:"generation"`
	State      ReplicaState `json:"state"`
	ValidUntil *time.Time   `json:"valid_until,omitempty"`
}
