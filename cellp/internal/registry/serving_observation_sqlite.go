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

type storedEndpoint struct {
	host      string
	port      int
	epState   contract.EndpointState
	validStr  string
	validTime *time.Time
	present   bool
}

func (s *SQLiteStore) RecordObservation(ctx context.Context, obs ReplicaObservation) error {
	if err := validateObservation(obs); err != nil {
		return err
	}
	now := time.Now().UTC()
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var curProj, curVer, curState, curNode string
		var curGen int64
		var curValid sql.NullString
		var curAssigned sql.NullInt64
		err = tx.QueryRowContext(ctx, `
SELECT project_id, version_id, state, generation, node_id, valid_until, assigned_node_generation FROM runtime_replicas WHERE replica_id = ?`,
			obs.ReplicaID).Scan(&curProj, &curVer, &curState, &curGen, &curNode, &curValid, &curAssigned)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrObservationStale
		}
		if err != nil {
			return err
		}
		if curGen != obs.Generation || curNode != obs.NodeID {
			return ErrObservationStale
		}
		if curProj != obs.ProjectID || curVer != obs.VersionID {
			return ErrObservationStale
		}
		if !curAssigned.Valid {
			return ErrObservationStale
		}

		if !curValid.Valid {
			return ErrLeaseExpired
		}
		assignUntil, err := parseRFC3339NanoRequired(curValid.String)
		if err != nil {
			return fmt.Errorf("timestamp corrupt")
		}
		if !assignUntil.After(now) {
			return ErrLeaseExpired
		}
		if obs.AssignmentValidUntil != nil && !assignmentLeaseEqual(*obs.AssignmentValidUntil, assignUntil) {
			return ErrObservationStale
		}
		assignStr := assignUntil.Format(time.RFC3339Nano)

		var cordoned int
		var nodeLease string
		var nodeGen int64
		err = tx.QueryRowContext(ctx, `
SELECT cordoned, lease_expiry, generation FROM runtime_nodes WHERE node_id = ?`, obs.NodeID).Scan(&cordoned, &nodeLease, &nodeGen)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseExpired
		}
		if err != nil {
			return err
		}
		// Cordoned nodes reject new work (pending/starting/ready). Draining/stopped/failed remain
		// admissible when leases match so in-flight replicas can tear down without widening routing.
		// ErrLeaseExpired here is fail-closed admission, not a claim that the node lease expired.
		if cordoned != 0 {
			switch obs.State {
			case contract.ReplicaDraining, contract.ReplicaStopped, contract.ReplicaFailed:
			default:
				return ErrLeaseExpired
			}
		}
		if nodeGen != curAssigned.Int64 {
			return ErrObservationStale
		}
		nodeExpiry, err := parseRFC3339NanoRequired(nodeLease)
		if err != nil || !nodeExpiry.After(now) {
			return ErrLeaseExpired
		}

		prior, err := loadStoredEndpointTx(ctx, tx, obs.ReplicaID)
		if err != nil {
			return err
		}

		if contract.ReplicaState(curState) == obs.State {
			if observationReplayEqual(curState, prior, obs) {
				return tx.Commit()
			}
		} else if !replicaTransitionAllowed[contract.ReplicaState(curState)][obs.State] {
			return ErrReplicaTransitionInvalid
		}

		nowStr := now.Format(time.RFC3339Nano)
		res, err := tx.ExecContext(ctx, `
UPDATE runtime_replicas SET state = ?, valid_until = ?, updated_at = ?
WHERE replica_id = ? AND generation = ? AND node_id = ?`,
			string(obs.State), assignStr, nowStr, obs.ReplicaID, obs.Generation, obs.NodeID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrObservationStale
		}

		bumpRevision := observationRoutingBumpNeeded(contract.ReplicaState(curState), prior, obs)

		if obs.State == contract.ReplicaReady || obs.State == contract.ReplicaDraining {
			if strings.TrimSpace(obs.ListenHost) == "" || obs.ListenPort <= 0 {
				return fmt.Errorf("listen_host and listen_port required for routable replica")
			}
			epState := obs.EndpointState
			if epState == "" {
				if obs.State == contract.ReplicaDraining {
					epState = contract.EndpointDraining
				} else {
					epState = contract.EndpointReady
				}
			}
			if epState != contract.EndpointReady && epState != contract.EndpointDraining {
				return fmt.Errorf("invalid endpoint_state")
			}
			if epState == contract.EndpointReady {
				if obs.EndpointValidUntil == nil || obs.EndpointValidUntil.IsZero() {
					return ErrLeaseExpired
				}
				if !obs.EndpointValidUntil.After(now) {
					return ErrLeaseExpired
				}
			}
			var epValid *string
			if obs.EndpointValidUntil != nil && !obs.EndpointValidUntil.IsZero() {
				v := obs.EndpointValidUntil.UTC().Format(time.RFC3339Nano)
				epValid = &v
			}
			_, err = tx.ExecContext(ctx, `
INSERT INTO runtime_replica_endpoints (replica_id, listen_host, listen_port, endpoint_state, valid_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(replica_id) DO UPDATE SET
  listen_host = excluded.listen_host,
  listen_port = excluded.listen_port,
  endpoint_state = excluded.endpoint_state,
  valid_until = excluded.valid_until,
  updated_at = excluded.updated_at`,
				obs.ReplicaID, obs.ListenHost, obs.ListenPort, string(epState), epValid, nowStr)
			if err != nil {
				return err
			}
		}
		if obs.State == contract.ReplicaStopped || obs.State == contract.ReplicaFailed {
			if prior.present {
				_, err = tx.ExecContext(ctx, `DELETE FROM runtime_replica_endpoints WHERE replica_id = ?`, obs.ReplicaID)
				if err != nil {
					return err
				}
			} else {
				bumpRevision = false
			}
		}
		if bumpRevision {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func loadStoredEndpointTx(ctx context.Context, tx *sql.Tx, replicaID string) (storedEndpoint, error) {
	var host string
	var port int
	var epState string
	var epValid sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT listen_host, listen_port, endpoint_state, valid_until FROM runtime_replica_endpoints WHERE replica_id = ?`,
		replicaID).Scan(&host, &port, &epState, &epValid)
	if errors.Is(err, sql.ErrNoRows) {
		return storedEndpoint{}, nil
	}
	if err != nil {
		return storedEndpoint{}, err
	}
	se := storedEndpoint{
		host:    host,
		port:    port,
		epState: contract.EndpointState(epState),
		present: true,
	}
	if epState != string(contract.EndpointReady) && epState != string(contract.EndpointDraining) {
		return storedEndpoint{}, fmt.Errorf("endpoint state corrupt")
	}
	if epValid.Valid {
		se.validStr = epValid.String
		t, perr := parseRFC3339NanoRequired(epValid.String)
		if perr != nil {
			return storedEndpoint{}, perr
		}
		se.validTime = &t
	}
	return se, nil
}

func observationReplayEqual(curState string, prior storedEndpoint, obs ReplicaObservation) bool {
	if contract.ReplicaState(curState) != obs.State {
		return false
	}
	if obs.State == contract.ReplicaReady || obs.State == contract.ReplicaDraining {
		epState := obs.EndpointState
		if epState == "" {
			if obs.State == contract.ReplicaDraining {
				epState = contract.EndpointDraining
			} else {
				epState = contract.EndpointReady
			}
		}
		if !prior.present {
			return false
		}
		if prior.host != obs.ListenHost || prior.port != obs.ListenPort || prior.epState != epState {
			return false
		}
		var obsValid string
		if obs.EndpointValidUntil != nil && !obs.EndpointValidUntil.IsZero() {
			obsValid = obs.EndpointValidUntil.UTC().Format(time.RFC3339Nano)
		}
		return prior.validStr == obsValid
	}
	return true
}

func observationRoutingBumpNeeded(from contract.ReplicaState, prior storedEndpoint, obs ReplicaObservation) bool {
	if obs.State == contract.ReplicaStopped || obs.State == contract.ReplicaFailed {
		return prior.present
	}
	if obs.State != contract.ReplicaReady && obs.State != contract.ReplicaDraining {
		return false
	}
	epState := obs.EndpointState
	if epState == "" {
		if obs.State == contract.ReplicaDraining {
			epState = contract.EndpointDraining
		} else {
			epState = contract.EndpointReady
		}
	}
	if !prior.present {
		return true
	}
	var obsValid string
	if obs.EndpointValidUntil != nil && !obs.EndpointValidUntil.IsZero() {
		obsValid = obs.EndpointValidUntil.UTC().Format(time.RFC3339Nano)
	}
	if from != obs.State || prior.host != obs.ListenHost || prior.port != obs.ListenPort ||
		prior.epState != epState || prior.validStr != obsValid {
		return true
	}
	return false
}

// WithdrawReplica atomically removes ready routing eligibility and records draining,
// fenced by the complete durable replica identity. It deliberately ignores lease age.
func (s *SQLiteStore) WithdrawReplica(ctx context.Context, replicaID, projectID, versionID, nodeID string, generation int64) error {
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var current string
		err = tx.QueryRowContext(ctx, `SELECT state FROM runtime_replicas
WHERE replica_id = ? AND project_id = ? AND version_id = ? AND node_id = ? AND generation = ?`,
			replicaID, projectID, versionID, nodeID, generation).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrObservationStale
		}
		if err != nil {
			return err
		}
		state := contract.ReplicaState(current)
		if state == contract.ReplicaReady {
			if _, err := tx.ExecContext(ctx, `UPDATE runtime_replicas SET state = ?, updated_at = ?
WHERE replica_id = ? AND project_id = ? AND version_id = ? AND node_id = ? AND generation = ?`,
				string(contract.ReplicaDraining), time.Now().UTC().Format(time.RFC3339Nano),
				replicaID, projectID, versionID, nodeID, generation); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM runtime_replica_endpoints WHERE replica_id = ?`, replicaID)
		if err != nil {
			return err
		}
		if removed, _ := res.RowsAffected(); removed > 0 {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// TerminalizeReplica is the reconciliation-only fail-closed path. It fences on
// replica generation and node ownership but deliberately does not require a live lease.
func (s *SQLiteStore) TerminalizeReplica(ctx context.Context, replicaID, nodeID string, generation int64, state contract.ReplicaState) error {
	if state != contract.ReplicaStopped && state != contract.ReplicaFailed {
		return fmt.Errorf("terminal replica state required")
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var current string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM runtime_replicas WHERE replica_id = ? AND node_id = ? AND generation = ?`,
			replicaID, nodeID, generation).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrObservationStale
			}
			return err
		}
		if contract.ReplicaState(current) != state && !replicaTransitionAllowed[contract.ReplicaState(current)][state] {
			return ErrReplicaTransitionInvalid
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM runtime_replica_endpoints WHERE replica_id = ?`, replicaID)
		if err != nil {
			return err
		}
		removed, _ := res.RowsAffected()
		if _, err := tx.ExecContext(ctx, `UPDATE runtime_replicas SET state = ?, updated_at = ? WHERE replica_id = ? AND node_id = ? AND generation = ?`,
			string(state), time.Now().UTC().Format(time.RFC3339Nano), replicaID, nodeID, generation); err != nil {
			return err
		}
		if removed > 0 {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// RecordObservationAndCompleteAgentCommand atomically publishes a ready endpoint
// and the terminal command result, closing the ready/command-complete crash window.
func (s *SQLiteStore) RecordObservationAndCompleteAgentCommand(ctx context.Context, obs ReplicaObservation, command AgentCommand) error {
	if obs.State != contract.ReplicaReady || command.Status != "succeeded" || command.ResultState != contract.ReplicaReady {
		return fmt.Errorf("ready success required")
	}
	if err := validateObservation(obs); err != nil {
		return err
	}
	if err := validateAgentCommand(command); err != nil {
		return err
	}
	now := time.Now().UTC()
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var projectID, versionID, nodeID, current, validUntil string
		var generation, assignedNodeGeneration, nodeGeneration int64
		var nodeLease string
		var cordoned int
		err = tx.QueryRowContext(ctx, `
SELECT r.project_id, r.version_id, r.node_id, r.generation, r.state, r.valid_until,
       r.assigned_node_generation, n.generation, n.lease_expiry, n.cordoned
FROM runtime_replicas r JOIN runtime_nodes n ON n.node_id = r.node_id WHERE r.replica_id = ?`, obs.ReplicaID).
			Scan(&projectID, &versionID, &nodeID, &generation, &current, &validUntil,
				&assignedNodeGeneration, &nodeGeneration, &nodeLease, &cordoned)
		if err != nil {
			return err
		}
		assignmentExpiry, err := time.Parse(time.RFC3339Nano, validUntil)
		if err != nil {
			return fmt.Errorf("timestamp corrupt")
		}
		nodeExpiry, err := time.Parse(time.RFC3339Nano, nodeLease)
		if err != nil {
			return fmt.Errorf("timestamp corrupt")
		}
		if projectID != obs.ProjectID || versionID != obs.VersionID || nodeID != obs.NodeID || generation != obs.Generation ||
			assignedNodeGeneration != nodeGeneration || cordoned != 0 || !assignmentExpiry.After(now) || !nodeExpiry.After(now) ||
			obs.AssignmentValidUntil == nil || !assignmentLeaseEqual(*obs.AssignmentValidUntil, assignmentExpiry) {
			return ErrObservationStale
		}
		if contract.ReplicaState(current) != contract.ReplicaReady && !replicaTransitionAllowed[contract.ReplicaState(current)][contract.ReplicaReady] {
			return ErrReplicaTransitionInvalid
		}
		if strings.TrimSpace(obs.ListenHost) == "" || obs.ListenPort <= 0 || obs.EndpointValidUntil == nil || !obs.EndpointValidUntil.After(now) {
			return ErrLeaseExpired
		}
		prior, err := loadStoredEndpointTx(ctx, tx, obs.ReplicaID)
		if err != nil {
			return err
		}
		nowStr := now.Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE runtime_replicas SET state = ?, updated_at = ? WHERE replica_id = ? AND node_id = ? AND generation = ?`,
			string(contract.ReplicaReady), nowStr, obs.ReplicaID, obs.NodeID, obs.Generation); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO runtime_replica_endpoints (replica_id, listen_host, listen_port, endpoint_state, valid_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(replica_id) DO UPDATE SET listen_host=excluded.listen_host,
listen_port=excluded.listen_port, endpoint_state=excluded.endpoint_state, valid_until=excluded.valid_until, updated_at=excluded.updated_at`,
			obs.ReplicaID, obs.ListenHost, obs.ListenPort, string(contract.EndpointReady), obs.EndpointValidUntil.UTC().Format(time.RFC3339Nano), nowStr); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE runtime_agent_commands SET status = ?, result_state = ?, reason = NULL, expires_at = ?, updated_at = ?
WHERE idempotency_key = ? AND action = ? AND node_id = ? AND project_id = ? AND version_id = ? AND replica_id = ? AND generation = ? AND status = 'running' AND attempt_token = ?`,
			command.Status, string(command.ResultState), command.ExpiresAt.UTC().Format(time.RFC3339Nano), nowStr,
			command.IdempotencyKey, string(command.Action), command.NodeID, command.ProjectID, command.VersionID, command.ReplicaID, command.Generation, command.AttemptToken)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrAgentCommandConflict
		}
		if !observationReplayEqual(current, prior, obs) {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}
