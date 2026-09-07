package contract

import "time"

// ReplicaIsTerminal reports stopped/failed replica states.
func ReplicaIsTerminal(state ReplicaState) bool {
	return state == ReplicaStopped || state == ReplicaFailed
}

// AssignmentOccupiesSlot reports whether a replica row still consumes scheduler/registry
// assignment capacity (per-version active limit and per-node capacity). Terminal replicas
// and expired assignment leases do not occupy slots. Rows without a positive
// assigned_node_generation are treated as not occupying (fail-closed on claim).
// Stale node-generation rows still occupy until reconciliation terminalizes them.
func AssignmentOccupiesSlot(rep RuntimeReplica, now time.Time) bool {
	if ReplicaIsTerminal(rep.State) {
		return false
	}
	if rep.AssignedNodeGeneration <= 0 {
		return false
	}
	if rep.ValidUntil == nil || !rep.ValidUntil.After(now) {
		return false
	}
	return true
}
