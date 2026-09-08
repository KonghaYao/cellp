package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

var replicaTransitionAllowed = map[contract.ReplicaState]map[contract.ReplicaState]bool{
	contract.ReplicaPending: {
		contract.ReplicaStarting: true,
		contract.ReplicaFailed:   true,
		contract.ReplicaStopped:  true,
	},
	contract.ReplicaStarting: {
		contract.ReplicaReady:   true,
		contract.ReplicaFailed:  true,
		contract.ReplicaStopped: true,
	},
	contract.ReplicaReady: {
		contract.ReplicaDraining: true,
		contract.ReplicaFailed:   true,
		contract.ReplicaStopped:  true,
	},
	contract.ReplicaDraining: {
		contract.ReplicaStopped: true,
	},
	contract.ReplicaFailed: {
		contract.ReplicaStarting: true,
		contract.ReplicaStopped:  true,
	},
	contract.ReplicaStopped: {
		contract.ReplicaStarting: true,
	},
}

func (s *SQLiteStore) RenewRuntimeNodeLease(ctx context.Context, nodeID string, expectGen int64, leaseExpiry time.Time) error {
	if strings.TrimSpace(nodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if leaseExpiry.IsZero() {
		return fmt.Errorf("lease_expiry required")
	}
	now := time.Now().UTC()
	if !leaseExpiry.After(now) {
		return ErrLeaseExpired
	}
	return withRetryErr(func() error {
		nowStr := now.Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE runtime_nodes SET lease_expiry = ?, updated_at = ?
WHERE node_id = ? AND generation = ?`,
			leaseExpiry.UTC().Format(time.RFC3339Nano), nowStr, nodeID, expectGen)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNodeLeaseCASConflict
		}
		return nil
	})
}

func (s *SQLiteStore) ReleaseRuntimeNodeLease(ctx context.Context, nodeID string, expectGen int64) error {
	if strings.TrimSpace(nodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if expectGen <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	return withRetryErr(func() error {
		now := time.Now().UTC()
		expired := now.Add(-time.Second).Format(time.RFC3339Nano)
		nowStr := now.Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE runtime_nodes SET lease_expiry = ?, updated_at = ?
WHERE node_id = ? AND generation = ? AND lease_expiry > ?`, expired, nowStr, nodeID, expectGen, nowStr)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNodeLeaseCASConflict
		}
		return nil
	})
}

func (s *SQLiteStore) CordonRuntimeNode(ctx context.Context, nodeID string, expectGen int64) error {
	if strings.TrimSpace(nodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if expectGen <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	return withRetryErr(func() error {
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
UPDATE runtime_nodes SET cordoned = 1, updated_at = ?
WHERE node_id = ? AND generation = ? AND lease_expiry > ?`, nowStr, nodeID, expectGen, nowStr)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrNodeLeaseCASConflict
		}
		return nil
	})
}

func (s *SQLiteStore) ClaimAssignment(ctx context.Context, claim AssignmentClaim) error {
	if err := validateAssignmentClaim(claim); err != nil {
		return err
	}
	now := time.Now().UTC()
	if !claim.ValidUntil.After(now) {
		return ErrLeaseExpired
	}
	validUntil := claim.ValidUntil.UTC()
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var desireGen int64
		err = tx.QueryRowContext(ctx, `
SELECT generation FROM serving_desires WHERE project_id = ? AND version_id = ?`,
			claim.ProjectID, claim.VersionID).Scan(&desireGen)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAssignmentCASConflict
		}
		if err != nil {
			return err
		}
		if desireGen != claim.Generation {
			return ErrAssignmentCASConflict
		}
		if claim.ActiveLimit > 0 {
			var activeAssignments int
			nowStr := now.Format(time.RFC3339Nano)
			if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM runtime_replicas
WHERE project_id = ? AND version_id = ?
  AND state NOT IN (?, ?)
  AND assigned_node_generation > 0
  AND valid_until > ?`,
				claim.ProjectID, claim.VersionID, string(contract.ReplicaStopped), string(contract.ReplicaFailed), nowStr).Scan(&activeAssignments); err != nil {
				return err
			}
			if activeAssignments >= claim.ActiveLimit {
				return ErrAssignmentCASConflict
			}
		}
		// Existing assignments retain their immutable command generation across later
		// desire updates; only new claims are fenced by the current desire generation.

		var cordoned int
		var nodeLease string
		var nodeGen int64
		var nodeCapacity int
		err = tx.QueryRowContext(ctx, `
SELECT cordoned, lease_expiry, generation, capacity_units FROM runtime_nodes WHERE node_id = ?`, claim.NodeID).
			Scan(&cordoned, &nodeLease, &nodeGen, &nodeCapacity)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseExpired
		}
		if err != nil {
			return err
		}
		if nodeGen != claim.ExpectedNodeGeneration {
			return ErrAssignmentCASConflict
		}
		if cordoned != 0 {
			return ErrLeaseExpired
		}
		if nodeExpiry, err := parseRFC3339NanoRequired(nodeLease); err != nil || !nodeExpiry.After(now) {
			return ErrLeaseExpired
		}

		nowStr := now.Format(time.RFC3339Nano)

		var curProj, curVer, curNode, curState string
		var curGen int64
		var curValid sql.NullString
		var curAssigned sql.NullInt64
		err = tx.QueryRowContext(ctx, `
SELECT project_id, version_id, node_id, generation, state, valid_until, assigned_node_generation FROM runtime_replicas WHERE replica_id = ?`,
			claim.ReplicaID).Scan(&curProj, &curVer, &curNode, &curGen, &curState, &curValid, &curAssigned)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if err == nil {
			if isTerminalReplicaState(contract.ReplicaState(curState)) {
				return ErrAssignmentCASConflict
			}
			if curGen != claim.Generation {
				return ErrAssignmentCASConflict
			}
			if curProj != claim.ProjectID || curVer != claim.VersionID || curNode != claim.NodeID {
				return ErrAssignmentCASConflict
			}
			if !curValid.Valid {
				return ErrAssignmentCASConflict
			}
			curUntil, perr := parseRFC3339NanoRequired(curValid.String)
			if perr != nil || !assignmentLeaseEqual(curUntil, validUntil) {
				return ErrAssignmentCASConflict
			}
			if !curAssigned.Valid || curAssigned.Int64 != claim.ExpectedNodeGeneration {
				return ErrAssignmentCASConflict
			}
			return tx.Commit()
		}

		var nodeUsed int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM runtime_replicas
WHERE node_id = ?
  AND state NOT IN (?, ?)
  AND assigned_node_generation > 0
  AND valid_until > ?`,
			claim.NodeID, string(contract.ReplicaStopped), string(contract.ReplicaFailed), nowStr).Scan(&nodeUsed); err != nil {
			return err
		}
		if nodeCapacity <= 0 || nodeUsed >= nodeCapacity {
			return ErrAssignmentCASConflict
		}

		validStr := validUntil.Format(time.RFC3339Nano)

		_, err = tx.ExecContext(ctx, `
INSERT INTO runtime_replicas (replica_id, project_id, version_id, node_id, generation, assigned_node_generation, state, valid_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			claim.ReplicaID, claim.ProjectID, claim.VersionID, claim.NodeID, claim.Generation,
			claim.ExpectedNodeGeneration, string(contract.ReplicaPending), validStr, nowStr)
		if err != nil {
			return err
		}
		return tx.Commit()
	})
}

// RenewAssignmentLease extends an existing assignment and endpoint lease only while
// the desire, replica, and runtime-node generations still match. It never creates
// assignments and fails closed on cordon, lease expiry, or concurrent replacement.
func (s *SQLiteStore) RenewAssignmentLease(ctx context.Context, replicaID, nodeID string, generation, expectedNodeGeneration int64, expectedExpiry, newExpiry time.Time) error {
	if strings.TrimSpace(replicaID) == "" || strings.TrimSpace(nodeID) == "" || generation <= 0 || expectedNodeGeneration <= 0 || expectedExpiry.IsZero() {
		return fmt.Errorf("invalid assignment lease renewal")
	}
	now := time.Now().UTC()
	if !newExpiry.After(now) || !newExpiry.After(expectedExpiry) {
		return ErrLeaseExpired
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var state, validRaw, nodeLeaseRaw string
		var currentGeneration, assignedNodeGeneration, nodeGeneration int64
		var cordoned int
		err = tx.QueryRowContext(ctx, `
SELECT r.generation, r.assigned_node_generation, r.state, r.valid_until,
       n.generation, n.lease_expiry, n.cordoned
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
WHERE r.replica_id = ? AND r.node_id = ?`, replicaID, nodeID).Scan(
			&currentGeneration, &assignedNodeGeneration, &state, &validRaw,
			&nodeGeneration, &nodeLeaseRaw, &cordoned)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAssignmentCASConflict
		}
		if err != nil {
			return err
		}
		currentExpiry, err := parseRFC3339NanoRequired(validRaw)
		if err != nil {
			return fmt.Errorf("assignment timestamp corrupt")
		}
		nodeExpiry, err := parseRFC3339NanoRequired(nodeLeaseRaw)
		if err != nil {
			return fmt.Errorf("node timestamp corrupt")
		}
		if currentGeneration != generation ||
			assignedNodeGeneration != expectedNodeGeneration || nodeGeneration != expectedNodeGeneration ||
			cordoned != 0 || isTerminalReplicaState(contract.ReplicaState(state)) ||
			!assignmentLeaseEqual(currentExpiry, expectedExpiry) || !currentExpiry.After(now) ||
			!nodeExpiry.After(now) || newExpiry.After(nodeExpiry) {
			return ErrAssignmentCASConflict
		}

		newRaw := newExpiry.UTC().Format(time.RFC3339Nano)
		updatedRaw := now.Format(time.RFC3339Nano)
		res, err := tx.ExecContext(ctx, `
UPDATE runtime_replicas SET valid_until = ?, updated_at = ?
WHERE replica_id = ? AND node_id = ? AND generation = ? AND valid_until = ?`,
			newRaw, updatedRaw, replicaID, nodeID, generation, validRaw)
		if err != nil {
			return err
		}
		if rows, _ := res.RowsAffected(); rows != 1 {
			return ErrAssignmentCASConflict
		}
		endpoint, err := tx.ExecContext(ctx, `
UPDATE runtime_replica_endpoints SET valid_until = ?, updated_at = ? WHERE replica_id = ?`,
			newRaw, updatedRaw, replicaID)
		if err != nil {
			return err
		}
		if rows, _ := endpoint.RowsAffected(); rows > 0 {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func isTerminalReplicaState(s contract.ReplicaState) bool {
	return s == contract.ReplicaStopped || s == contract.ReplicaFailed
}

func validateAssignmentClaim(claim AssignmentClaim) error {
	if strings.TrimSpace(claim.ReplicaID) == "" || strings.TrimSpace(claim.ProjectID) == "" ||
		strings.TrimSpace(claim.VersionID) == "" || strings.TrimSpace(claim.NodeID) == "" {
		return fmt.Errorf("replica_id, project_id, version_id, node_id required")
	}
	if claim.Generation <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	if claim.ExpectedNodeGeneration <= 0 {
		return fmt.Errorf("expected_node_generation must be positive")
	}
	return nil
}

func (s *SQLiteStore) GetRuntimeReplica(ctx context.Context, replicaID string) (*contract.RuntimeReplica, error) {
	return withRetry(func() (*contract.RuntimeReplica, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT project_id, version_id, node_id, generation, assigned_node_generation, state, valid_until
FROM runtime_replicas WHERE replica_id = ?`, replicaID)
		var rep contract.RuntimeReplica
		rep.ReplicaID = replicaID
		var state string
		var assignedNodeGeneration sql.NullInt64
		var validUntil sql.NullString
		if err := row.Scan(&rep.ProjectID, &rep.VersionID, &rep.NodeID, &rep.Generation, &assignedNodeGeneration, &state, &validUntil); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		if assignedNodeGeneration.Valid {
			rep.AssignedNodeGeneration = assignedNodeGeneration.Int64
		}
		rep.State = contract.ReplicaState(state)
		if validUntil.Valid {
			if t, err := time.Parse(time.RFC3339Nano, validUntil.String); err == nil {
				rep.ValidUntil = &t
			}
		}
		return &rep, nil
	})
}

func validateObservation(obs ReplicaObservation) error {
	if strings.TrimSpace(obs.ReplicaID) == "" || strings.TrimSpace(obs.ProjectID) == "" ||
		strings.TrimSpace(obs.VersionID) == "" || strings.TrimSpace(obs.NodeID) == "" {
		return fmt.Errorf("replica_id, project_id, version_id, node_id required")
	}
	if obs.Generation <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	switch obs.State {
	case contract.ReplicaPending, contract.ReplicaStarting, contract.ReplicaReady,
		contract.ReplicaDraining, contract.ReplicaStopped, contract.ReplicaFailed:
	default:
		return fmt.Errorf("invalid replica state")
	}
	return nil
}

func (s *SQLiteStore) ListRuntimeReplicasByNode(ctx context.Context, nodeID string) ([]contract.RuntimeReplica, error) {
	return withRetry(func() ([]contract.RuntimeReplica, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT replica_id, project_id, version_id, generation, assigned_node_generation, state, valid_until
FROM runtime_replicas WHERE node_id = ? ORDER BY replica_id`, nodeID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRuntimeReplicaRows(rows, nodeID)
	})
}

func (s *SQLiteStore) ListRuntimeReplicasForReconcile(ctx context.Context) ([]contract.RuntimeReplica, error) {
	return withRetry(func() ([]contract.RuntimeReplica, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT replica_id, project_id, version_id, node_id, generation, assigned_node_generation, state, valid_until
FROM runtime_replicas
WHERE state NOT IN (?, ?) ORDER BY project_id, version_id, replica_id`,
			string(contract.ReplicaStopped), string(contract.ReplicaFailed))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []contract.RuntimeReplica
		for rows.Next() {
			var rep contract.RuntimeReplica
			var state string
			var assignedNodeGeneration sql.NullInt64
			var validUntil sql.NullString
			if err := rows.Scan(&rep.ReplicaID, &rep.ProjectID, &rep.VersionID, &rep.NodeID, &rep.Generation, &assignedNodeGeneration, &state, &validUntil); err != nil {
				return nil, err
			}
			if assignedNodeGeneration.Valid {
				rep.AssignedNodeGeneration = assignedNodeGeneration.Int64
			}
			rep.State = contract.ReplicaState(state)
			if validUntil.Valid {
				if t, err := time.Parse(time.RFC3339Nano, validUntil.String); err == nil {
					rep.ValidUntil = &t
				}
			}
			out = append(out, rep)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) ListExpiredAssignments(ctx context.Context, now time.Time) ([]contract.RuntimeReplica, error) {
	return withRetry(func() ([]contract.RuntimeReplica, error) {
		nowStr := now.UTC().Format(time.RFC3339Nano)
		rows, err := s.db.QueryContext(ctx, `
SELECT replica_id, project_id, version_id, node_id, generation, assigned_node_generation, state, valid_until
FROM runtime_replicas
WHERE valid_until IS NOT NULL AND valid_until < ?
ORDER BY valid_until`, nowStr)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []contract.RuntimeReplica
		for rows.Next() {
			var rep contract.RuntimeReplica
			var state string
			var assignedNodeGeneration sql.NullInt64
			var validUntil sql.NullString
			if err := rows.Scan(&rep.ReplicaID, &rep.ProjectID, &rep.VersionID, &rep.NodeID, &rep.Generation, &assignedNodeGeneration, &state, &validUntil); err != nil {
				return nil, err
			}
			if assignedNodeGeneration.Valid {
				rep.AssignedNodeGeneration = assignedNodeGeneration.Int64
			}
			rep.State = contract.ReplicaState(state)
			if validUntil.Valid {
				if t, err := time.Parse(time.RFC3339Nano, validUntil.String); err == nil {
					rep.ValidUntil = &t
				}
			}
			out = append(out, rep)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) ListElasticEnrolledVersions(ctx context.Context) ([]ElasticVersionRef, error) {
	return withRetry(func() ([]ElasticVersionRef, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT project_id, id FROM versions WHERE elastic_enrolled = 1 ORDER BY project_id, id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ElasticVersionRef
		for rows.Next() {
			var ref ElasticVersionRef
			if err := rows.Scan(&ref.ProjectID, &ref.VersionID); err != nil {
				return nil, err
			}
			out = append(out, ref)
		}
		return out, rows.Err()
	})
}

func scanRuntimeReplicaRows(rows *sql.Rows, nodeID string) ([]contract.RuntimeReplica, error) {
	var out []contract.RuntimeReplica
	for rows.Next() {
		var rep contract.RuntimeReplica
		rep.NodeID = nodeID
		var state string
		var assignedNodeGeneration sql.NullInt64
		var validUntil sql.NullString
		if err := rows.Scan(&rep.ReplicaID, &rep.ProjectID, &rep.VersionID, &rep.Generation, &assignedNodeGeneration, &state, &validUntil); err != nil {
			return nil, err
		}
		if assignedNodeGeneration.Valid {
			rep.AssignedNodeGeneration = assignedNodeGeneration.Int64
		}
		rep.State = contract.ReplicaState(state)
		if validUntil.Valid {
			if t, err := time.Parse(time.RFC3339Nano, validUntil.String); err == nil {
				rep.ValidUntil = &t
			}
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}
