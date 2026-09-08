package transport

import (
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// wireRuntimeNode carries relay fields omitted from public contract JSON tags.
type wireRuntimeNode struct {
	NodeID        string    `json:"node_id"`
	CapacityUnits int       `json:"capacity_units"`
	Cordoned      bool      `json:"cordoned"`
	LeaseExpiry   time.Time `json:"lease_expiry"`
	Generation    int64     `json:"generation"`
	AgentBaseURL  string    `json:"agent_base_url"`
	IdentityURI   string    `json:"identity_uri"`
	Zone          string    `json:"zone"`
}

type wireRuntimeReplica struct {
	ReplicaID              string                `json:"replica_id"`
	ProjectID              string                `json:"project_id"`
	VersionID              string                `json:"version_id"`
	NodeID                 string                `json:"node_id"`
	Generation             int64                 `json:"generation"`
	AssignedNodeGeneration int64                 `json:"assigned_node_generation"`
	State                  contract.ReplicaState `json:"state"`
	ValidUntil             *time.Time            `json:"valid_until,omitempty"`
}

func RuntimeNodeToWire(n *contract.RuntimeNode) *wireRuntimeNode   { return runtimeNodeToWire(n) }
func RuntimeNodeFromWire(w *wireRuntimeNode) *contract.RuntimeNode { return runtimeNodeFromWire(w) }
func RuntimeReplicaToWire(r *contract.RuntimeReplica) *wireRuntimeReplica {
	return runtimeReplicaToWire(r)
}
func RuntimeReplicaFromWire(w *wireRuntimeReplica) *contract.RuntimeReplica {
	return runtimeReplicaFromWire(w)
}

func runtimeNodeToWire(n *contract.RuntimeNode) *wireRuntimeNode {
	if n == nil {
		return nil
	}
	return &wireRuntimeNode{
		NodeID: n.NodeID, CapacityUnits: n.CapacityUnits, Cordoned: n.Cordoned,
		LeaseExpiry: n.LeaseExpiry, Generation: n.Generation,
		AgentBaseURL: n.AgentBaseURL, IdentityURI: n.IdentityURI, Zone: n.Zone,
	}
}

func runtimeNodeFromWire(w *wireRuntimeNode) *contract.RuntimeNode {
	if w == nil {
		return nil
	}
	return &contract.RuntimeNode{
		NodeID: w.NodeID, CapacityUnits: w.CapacityUnits, Cordoned: w.Cordoned,
		LeaseExpiry: w.LeaseExpiry, Generation: w.Generation,
		AgentBaseURL: w.AgentBaseURL, IdentityURI: w.IdentityURI, Zone: w.Zone,
	}
}

func runtimeReplicaToWire(r *contract.RuntimeReplica) *wireRuntimeReplica {
	if r == nil {
		return nil
	}
	return &wireRuntimeReplica{
		ReplicaID: r.ReplicaID, ProjectID: r.ProjectID, VersionID: r.VersionID,
		NodeID: r.NodeID, Generation: r.Generation, AssignedNodeGeneration: r.AssignedNodeGeneration,
		State: r.State, ValidUntil: r.ValidUntil,
	}
}

func runtimeReplicaFromWire(w *wireRuntimeReplica) *contract.RuntimeReplica {
	if w == nil {
		return nil
	}
	return &contract.RuntimeReplica{
		ReplicaID: w.ReplicaID, ProjectID: w.ProjectID, VersionID: w.VersionID,
		NodeID: w.NodeID, Generation: w.Generation, AssignedNodeGeneration: w.AssignedNodeGeneration,
		State: w.State, ValidUntil: w.ValidUntil,
	}
}

func runtimeReplicasToWire(reps []contract.RuntimeReplica) []wireRuntimeReplica {
	out := make([]wireRuntimeReplica, 0, len(reps))
	for _, r := range reps {
		if w := runtimeReplicaToWire(&r); w != nil {
			out = append(out, *w)
		}
	}
	return out
}

func runtimeReplicasFromWire(reps []wireRuntimeReplica) []contract.RuntimeReplica {
	out := make([]contract.RuntimeReplica, 0, len(reps))
	for _, w := range reps {
		if r := runtimeReplicaFromWire(&w); r != nil {
			out = append(out, *r)
		}
	}
	return out
}
