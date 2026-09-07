package scheduler

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/google/uuid"
)

const replicaCapacityUnit = 1

// PlacementInput is the node/replica view for one scheduling decision.
type PlacementInput struct {
	ProjectID string
	VersionID string
	Nodes     []contract.RuntimeNode
	Replicas  []contract.RuntimeReplica
	Now       time.Time
}

// PickNodeResult is the outcome of node selection.
type PickNodeResult struct {
	Node     *contract.RuntimeNode
	Degraded bool // anti-affinity violated (e.g. single-node dev)
}

// PickNode selects a node for a new replica using capacity, lease, cordon and version anti-affinity filters.
// Returns nil when no node can accept the placement (capacity_exhausted).
func PickNode(in PlacementInput) *contract.RuntimeNode {
	res := PickNodeDetailed(in)
	if res.Node == nil {
		return nil
	}
	return res.Node
}

// PickNodeDetailed is like PickNode but reports anti-affinity degradation.
func PickNodeDetailed(in PlacementInput) PickNodeResult {
	if len(in.Nodes) == 0 {
		return PickNodeResult{}
	}
	usage := nodeUsage(in.Nodes, in.Replicas, in.Now)
	versionOnNode := versionReplicaCount(in.ProjectID, in.VersionID, in.Nodes, in.Replicas, in.Now)
	type candidate struct {
		node     contract.RuntimeNode
		versionN int
		free     int
	}
	var candidates []candidate
	for _, node := range in.Nodes {
		if !nodeEligible(node, in.Now) {
			continue
		}
		used := usage[node.NodeID]
		free := node.CapacityUnits - used
		if free < replicaCapacityUnit {
			continue
		}
		candidates = append(candidates, candidate{
			node:     node,
			versionN: versionOnNode[node.NodeID],
			free:     free,
		})
	}
	if len(candidates) == 0 {
		return PickNodeResult{}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.versionN != b.versionN {
			return a.versionN < b.versionN
		}
		if a.free != b.free {
			return a.free > b.free
		}
		return strings.Compare(a.node.NodeID, b.node.NodeID) < 0
	})
	chosen := candidates[0]
	out := chosen.node
	degraded := chosen.versionN > 0
	return PickNodeResult{Node: &out, Degraded: degraded}
}

func nodeEligible(node contract.RuntimeNode, now time.Time) bool {
	if strings.TrimSpace(node.NodeID) == "" || node.Generation <= 0 || node.Cordoned || node.CapacityUnits <= 0 {
		return false
	}
	if node.LeaseExpiry.IsZero() || !node.LeaseExpiry.After(now) {
		return false
	}
	if err := contract.ValidateRuntimeNodeMetadataIfPresent(node); err != nil {
		return false
	}
	return true
}

func nodeUsage(nodes []contract.RuntimeNode, replicas []contract.RuntimeReplica, now time.Time) map[string]int {
	known := make(map[string]contract.RuntimeNode, len(nodes))
	for _, n := range nodes {
		known[n.NodeID] = n
	}
	usage := make(map[string]int, len(nodes))
	for _, rep := range replicas {
		if _, ok := known[rep.NodeID]; !ok {
			continue
		}
		if !contract.AssignmentOccupiesSlot(rep, now) {
			continue
		}
		usage[rep.NodeID] += replicaCapacityUnit
	}
	return usage
}

func versionReplicaCount(projectID, versionID string, nodes []contract.RuntimeNode, replicas []contract.RuntimeReplica, now time.Time) map[string]int {
	out := map[string]int{}
	known := make(map[string]contract.RuntimeNode, len(nodes))
	for _, node := range nodes {
		known[node.NodeID] = node
	}
	for _, rep := range replicas {
		if rep.ProjectID != projectID || rep.VersionID != versionID {
			continue
		}
		if _, ok := known[rep.NodeID]; !ok || !contract.AssignmentOccupiesSlot(rep, now) {
			continue
		}
		out[rep.NodeID]++
	}
	return out
}

// CountMatchingActive counts replicas that occupy a version active slot (see contract.AssignmentOccupiesSlot).
func CountMatchingActive(replicas []contract.RuntimeReplica, now time.Time) int {
	n := 0
	for _, rep := range replicas {
		if contract.AssignmentOccupiesSlot(rep, now) {
			n++
		}
	}
	return n
}

func isTerminalReplica(state contract.ReplicaState) bool {
	return contract.ReplicaIsTerminal(state)
}

// PickScaleDownTargets chooses replicas to drain when active count exceeds desired.
func PickScaleDownTargets(replicas []contract.RuntimeReplica, _ int64, desired int) []contract.RuntimeReplica {
	var active []contract.RuntimeReplica
	for _, rep := range replicas {
		if isTerminalReplica(rep.State) {
			continue
		}
		active = append(active, rep)
	}
	excess := len(active) - desired
	if excess <= 0 {
		return nil
	}
	sort.Slice(active, func(i, j int) bool {
		pi, pj := scaleDownPriority(active[i]), scaleDownPriority(active[j])
		if pi != pj {
			return pi < pj
		}
		return strings.Compare(active[i].ReplicaID, active[j].ReplicaID) < 0
	})
	if excess > len(active) {
		excess = len(active)
	}
	return append([]contract.RuntimeReplica(nil), active[:excess]...)
}

func scaleDownPriority(rep contract.RuntimeReplica) int {
	switch rep.State {
	case contract.ReplicaPending:
		return 0
	case contract.ReplicaStarting:
		return 1
	case contract.ReplicaFailed:
		return 2
	case contract.ReplicaReady:
		return 3
	case contract.ReplicaDraining:
		return 4
	default:
		return 5
	}
}

// PickStaleGenerationReplicas is retained for compatibility. Replica generation is
// an immutable assignment identity and is not made stale by a later desire update.
func PickStaleGenerationReplicas([]contract.RuntimeReplica, int64) []contract.RuntimeReplica {
	return nil
}

// NextReplicaID allocates a new, non-reusable replica identity. Existing identities
// are never recycled; a random suffix keeps concurrent/restarted controllers from
// colliding while assignment CAS remains the authoritative fence.
func NextReplicaID(_, _ string, _ int64, existing []contract.RuntimeReplica) string {
	used := make(map[string]struct{}, len(existing))
	for _, rep := range existing {
		used[rep.ReplicaID] = struct{}{}
	}
	for {
		id := "rep-" + uuid.NewString()
		if _, exists := used[id]; !exists {
			return id
		}
	}
}

// ErrCapacityExhausted is returned when placement cannot find a node.
var ErrCapacityExhausted = errors.New("capacity_exhausted")
