package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

// ValidateAgentAssignment atomically verifies replica scope, assignment lease,
// assigned node generation, and the current node lease.
func (s *SQLiteStore) ValidateAgentAssignment(ctx context.Context, scope contract.CommandScope, now time.Time) (*contract.RuntimeReplica, error) {
	return withRetry(func() (*contract.RuntimeReplica, error) {
		var rep contract.RuntimeReplica
		var state, validRaw, nodeLeaseRaw string
		var assignedNodeGeneration, currentNodeGeneration int64
		var cordoned int
		err := s.db.QueryRowContext(ctx, `
SELECT r.project_id, r.version_id, r.node_id, r.generation, r.state, r.valid_until,
       r.assigned_node_generation, n.generation, n.lease_expiry, n.cordoned
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
WHERE r.replica_id = ?`, scope.ReplicaID).Scan(
			&rep.ProjectID, &rep.VersionID, &rep.NodeID, &rep.Generation, &state, &validRaw,
			&assignedNodeGeneration, &currentNodeGeneration, &nodeLeaseRaw, &cordoned)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		rep.ReplicaID = scope.ReplicaID
		rep.State = contract.ReplicaState(state)
		validUntil, err := time.Parse(time.RFC3339Nano, validRaw)
		if err != nil {
			return nil, fmt.Errorf("assignment timestamp corrupt")
		}
		nodeLease, err := time.Parse(time.RFC3339Nano, nodeLeaseRaw)
		if err != nil {
			return nil, fmt.Errorf("node timestamp corrupt")
		}
		rep.ValidUntil = &validUntil
		if rep.ProjectID != scope.ProjectID || rep.VersionID != scope.VersionID || rep.NodeID != scope.NodeID ||
			rep.Generation != scope.Generation || assignedNodeGeneration != currentNodeGeneration ||
			cordoned != 0 || !validUntil.After(now) || !nodeLease.After(now) || !validUntil.Equal(scope.LeaseExpiry) {
			return nil, ErrObservationStale
		}
		rep.AssignedNodeGeneration = assignedNodeGeneration
		return &rep, nil
	})
}

// ValidateAgentCleanupAssignment authorizes only exact assigned identity cleanup.
// Assignment and node leases may be expired and the node may be cordoned; the
// assigned node generation must still equal the current node generation.
func (s *SQLiteStore) ValidateAgentCleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	return withRetry(func() (*contract.RuntimeReplica, error) {
		var rep contract.RuntimeReplica
		var state string
		var validRaw sql.NullString
		var assignedNodeGeneration, currentNodeGeneration sql.NullInt64
		err := s.db.QueryRowContext(ctx, `
SELECT r.project_id, r.version_id, r.node_id, r.generation, r.state, r.valid_until,
       r.assigned_node_generation, n.generation
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
WHERE r.replica_id = ?`, scope.ReplicaID).Scan(
			&rep.ProjectID, &rep.VersionID, &rep.NodeID, &rep.Generation, &state, &validRaw,
			&assignedNodeGeneration, &currentNodeGeneration)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		rep.ReplicaID = scope.ReplicaID
		rep.State = contract.ReplicaState(state)
		if validRaw.Valid {
			validUntil, err := time.Parse(time.RFC3339Nano, validRaw.String)
			if err != nil {
				return nil, fmt.Errorf("assignment timestamp corrupt")
			}
			rep.ValidUntil = &validUntil
		}
		if rep.ProjectID != scope.ProjectID || rep.VersionID != scope.VersionID || rep.NodeID != scope.NodeID ||
			rep.Generation != scope.Generation || !assignedNodeGeneration.Valid || !currentNodeGeneration.Valid ||
			assignedNodeGeneration.Int64 != currentNodeGeneration.Int64 {
			return nil, ErrObservationStale
		}
		rep.AssignedNodeGeneration = assignedNodeGeneration.Int64
		return &rep, nil
	})
}
