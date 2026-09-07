package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
)

func (s *SQLiteStore) GetRouteRevision(ctx context.Context) (int64, error) {
	return withRetry(func() (int64, error) {
		var rev int64
		err := s.db.QueryRowContext(ctx, `SELECT route_revision FROM control_plane_meta WHERE id = 1`).Scan(&rev)
		return rev, err
	})
}

func (s *SQLiteStore) BumpRouteRevision(ctx context.Context) (int64, error) {
	return withRetry(func() (int64, error) {
		if _, err := s.db.ExecContext(ctx, `UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`); err != nil {
			return 0, err
		}
		var rev int64
		err := s.db.QueryRowContext(ctx, `SELECT route_revision FROM control_plane_meta WHERE id = 1`).Scan(&rev)
		return rev, err
	})
}

func (s *SQLiteStore) bumpRouteRevisionQuiet(ctx context.Context) {
	_, _ = s.BumpRouteRevision(ctx)
}

// CompareAndSetElasticVersionStatus transitions only an enrolled version in the expected state.
// The public snapshot revision changes in the same transaction whenever ready eligibility changes.
func (s *SQLiteStore) CompareAndSetElasticVersionStatus(ctx context.Context, projectID, versionID, expectedStatus, newStatus string, expectedDesireGeneration int64, expectedDesiredReplicas int) error {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(versionID) == "" || expectedDesireGeneration <= 0 || expectedDesiredReplicas < 0 ||
		!contract.ValidVersionStatus(expectedStatus) || !contract.ValidVersionStatus(newStatus) {
		return fmt.Errorf("invalid elastic version status transition")
	}
	allowed := expectedStatus == contract.StatusDeployReady && newStatus == contract.StatusReady ||
		expectedStatus == contract.StatusReady && newStatus == contract.StatusDeployReady
	if !allowed || newStatus == contract.StatusReady && expectedDesiredReplicas < 1 ||
		newStatus == contract.StatusDeployReady && expectedDesiredReplicas != 0 {
		return fmt.Errorf("unsupported elastic version status transition")
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var desireGeneration int64
		var desiredReplicas int
		var desireReason string
		if err := tx.QueryRowContext(ctx, `
SELECT generation, desired_replicas, reason FROM serving_desires
WHERE project_id = ? AND version_id = ?`, projectID, versionID).Scan(&desireGeneration, &desiredReplicas, &desireReason); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrElasticVersionStatusCASConflict
			}
			return err
		}
		if desireGeneration != expectedDesireGeneration || desiredReplicas != expectedDesiredReplicas {
			return ErrElasticVersionStatusCASConflict
		}
		var policyMin int
		var enrolled int
		if err := tx.QueryRowContext(ctx, `
SELECT min_replicas, elastic_enrolled FROM serving_policies
WHERE project_id = ? AND version_id = ?`, projectID, versionID).Scan(&policyMin, &enrolled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrElasticVersionStatusCASConflict
			}
			return err
		}
		if enrolled != 1 || newStatus == contract.StatusReady && desireReason != "activator_ensure" ||
			newStatus == contract.StatusDeployReady && policyMin != 0 {
			return ErrElasticVersionStatusCASConflict
		}
		allowProdCold := 0
		if config.LoadServingDefaults().ProdScaleToZero {
			allowProdCold = 1
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var readyAt, lastAccess interface{}
		if newStatus == contract.StatusReady {
			readyAt, lastAccess = now, now
		}
		res, err := tx.ExecContext(ctx, `
UPDATE versions SET status = ?, updated_at = ?,
  ready_at = COALESCE(?, ready_at),
  last_access_at = COALESCE(?, last_access_at, ready_at)
WHERE project_id = ? AND id = ? AND status = ? AND elastic_enrolled = 1
  AND (? <> ? OR ready_at IS NOT NULL)
  AND (? <> ? OR ? = 1 OR NOT EXISTS (
    SELECT 1 FROM projects p WHERE p.id = ? AND p.prod_version_id = ?
  ))`,
			newStatus, now, readyAt, lastAccess, projectID, versionID, expectedStatus,
			newStatus, contract.StatusReady,
			newStatus, contract.StatusDeployReady, allowProdCold, projectID, versionID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrElasticVersionStatusCASConflict
		}
		if contract.IsServingQualifiedReady(expectedStatus) != contract.IsServingQualifiedReady(newStatus) {
			if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (s *SQLiteStore) UpsertServingPolicy(ctx context.Context, row ServingPolicyRow) error {
	pol := contract.ServingPolicy{
		Revision:        row.Revision,
		MinReplicas:     row.MinReplicas,
		MaxReplicas:     row.MaxReplicas,
		Priority:        row.Priority,
		BackgroundMode:  row.BackgroundMode,
		ElasticEnrolled: row.ElasticEnrolled,
	}
	if err := contract.ValidateServingPolicy(pol); err != nil {
		return err
	}
	if row.ElasticEnrolled {
		if err := contract.ValidateServingPolicyBackground(pol, contract.BackgroundGuardOptions{}); err != nil {
			return err
		}
	}
	now := row.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return withRetryErr(func() error {
		return s.upsertServingPolicyTx(ctx, row, now)
	})
}

// upsertServingPolicyTestHook is set by tests to inject failures (step 1=policy, 2=version, 3=revision).
var upsertServingPolicyTestHook func(step int) error

func (s *SQLiteStore) upsertServingPolicyTx(ctx context.Context, row ServingPolicyRow, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	enrolled := 0
	if row.ElasticEnrolled {
		enrolled = 1
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO serving_policies (project_id, version_id, revision, min_replicas, max_replicas, priority, background_mode, elastic_enrolled, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, version_id) DO UPDATE SET
  revision = excluded.revision,
  min_replicas = excluded.min_replicas,
  max_replicas = excluded.max_replicas,
  priority = excluded.priority,
  background_mode = excluded.background_mode,
  elastic_enrolled = excluded.elastic_enrolled,
  updated_at = excluded.updated_at`,
		row.ProjectID, row.VersionID, row.Revision, row.MinReplicas, row.MaxReplicas, row.Priority,
		string(row.BackgroundMode), enrolled, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if upsertServingPolicyTestHook != nil {
		if err := upsertServingPolicyTestHook(1); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE versions SET elastic_enrolled = ? WHERE project_id = ? AND id = ?`,
		enrolled, row.ProjectID, row.VersionID)
	if err != nil {
		return err
	}
	if upsertServingPolicyTestHook != nil {
		if err := upsertServingPolicyTestHook(2); err != nil {
			return err
		}
	}
	if upsertServingPolicyTestHook != nil {
		if err := upsertServingPolicyTestHook(3); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE control_plane_meta SET policy_revision = policy_revision + 1 WHERE id = 1`)
	if err != nil {
		return err
	}
	if err := bumpRouteRevisionInTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetServingPolicy(ctx context.Context, projectID, versionID string) (*ServingPolicyRow, error) {
	return withRetry(func() (*ServingPolicyRow, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT revision, min_replicas, max_replicas, priority, background_mode, elastic_enrolled, updated_at
FROM serving_policies WHERE project_id = ? AND version_id = ?`, projectID, versionID)
		var r ServingPolicyRow
		r.ProjectID = projectID
		r.VersionID = versionID
		var enrolled int
		var updated string
		if err := row.Scan(&r.Revision, &r.MinReplicas, &r.MaxReplicas, &r.Priority, &r.BackgroundMode, &enrolled, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		r.ElasticEnrolled = enrolled == 1
		if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			r.UpdatedAt = t
		}
		return &r, nil
	})
}

func (s *SQLiteStore) ListElasticServingPolicies(ctx context.Context) ([]ServingPolicyRow, error) {
	return withRetry(func() ([]ServingPolicyRow, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT project_id, version_id, revision, min_replicas, max_replicas, priority, background_mode, elastic_enrolled, updated_at
FROM serving_policies WHERE elastic_enrolled = 1 ORDER BY project_id, version_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ServingPolicyRow
		for rows.Next() {
			var r ServingPolicyRow
			var enrolled int
			var updated string
			if err := rows.Scan(&r.ProjectID, &r.VersionID, &r.Revision, &r.MinReplicas, &r.MaxReplicas,
				&r.Priority, &r.BackgroundMode, &enrolled, &updated); err != nil {
				return nil, err
			}
			r.ElasticEnrolled = enrolled == 1
			if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
				r.UpdatedAt = t
			}
			out = append(out, r)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) CompareAndSetDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire ServingDesireRow) error {
	if expectGen == 0 {
		if desire.Generation != 1 {
			return ErrDesiredCASConflict
		}
	} else if desire.Generation != expectGen+1 {
		return ErrDesiredCASConflict
	}
	return withRetryErr(func() error {
		now := desire.UpdatedAt
		if now.IsZero() {
			now = time.Now().UTC()
		}
		res, err := s.db.ExecContext(ctx, `
UPDATE serving_desires SET desired_replicas = ?, generation = ?, reason = ?, updated_at = ?
WHERE project_id = ? AND version_id = ? AND generation = ?`,
			desire.DesiredReplicas, desire.Generation, desire.Reason, now.Format(time.RFC3339Nano),
			projectID, versionID, expectGen)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			return nil
		}
		// insert if missing and expectGen==0
		if expectGen != 0 {
			return ErrDesiredCASConflict
		}
		_, err = s.db.ExecContext(ctx, `
INSERT INTO serving_desires (project_id, version_id, desired_replicas, generation, reason, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			projectID, versionID, desire.DesiredReplicas, desire.Generation, desire.Reason, now.Format(time.RFC3339Nano))
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return ErrDesiredCASConflict
			}
			return err
		}
		return nil
	})
}

func (s *SQLiteStore) EnsureActivationDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire ServingDesireRow, minReplicas int) error {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(versionID) == "" || minReplicas < 1 || desire.DesiredReplicas < minReplicas ||
		(expectGen == 0 && desire.Generation != 1) || (expectGen > 0 && desire.Generation != expectGen+1) {
		return ErrDesiredCASConflict
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var status string
		var readyAt sql.NullString
		var versionEnrolled int
		if err := tx.QueryRowContext(ctx, `
SELECT status, ready_at, elastic_enrolled FROM versions
WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&status, &readyAt, &versionEnrolled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActivationNotEligible
			}
			return err
		}
		var maxReplicas, policyEnrolled int
		if err := tx.QueryRowContext(ctx, `
SELECT max_replicas, elastic_enrolled FROM serving_policies
WHERE project_id = ? AND version_id = ?`, projectID, versionID).Scan(&maxReplicas, &policyEnrolled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrActivationNotEligible
			}
			return err
		}
		if status != contract.StatusDeployReady || !readyAt.Valid || versionEnrolled != 1 || policyEnrolled != 1 {
			return ErrActivationNotEligible
		}
		if maxReplicas < minReplicas {
			return ErrServingCapacityUnavailable
		}
		var currentDesired int
		var currentGeneration int64
		err = tx.QueryRowContext(ctx, `
SELECT desired_replicas, generation FROM serving_desires
WHERE project_id = ? AND version_id = ?`, projectID, versionID).Scan(&currentDesired, &currentGeneration)
		if err == nil {
			if currentDesired >= minReplicas {
				return tx.Commit()
			}
			if currentGeneration != expectGen {
				return ErrDesiredCASConflict
			}
			available, err := activationCapacityAvailableTx(ctx, tx, time.Now().UTC())
			if err != nil {
				return err
			}
			if !available {
				return ErrServingCapacityUnavailable
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			res, err := tx.ExecContext(ctx, `
UPDATE serving_desires SET desired_replicas = ?, generation = ?, reason = ?, updated_at = ?
WHERE project_id = ? AND version_id = ? AND generation = ?`, desire.DesiredReplicas, desire.Generation, desire.Reason, now, projectID, versionID, expectGen)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return ErrDesiredCASConflict
			}
			return tx.Commit()
		}
		if !errors.Is(err, sql.ErrNoRows) || expectGen != 0 {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrDesiredCASConflict
			}
			return err
		}
		available, err := activationCapacityAvailableTx(ctx, tx, time.Now().UTC())
		if err != nil {
			return err
		}
		if !available {
			return ErrServingCapacityUnavailable
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO serving_desires (project_id, version_id, desired_replicas, generation, reason, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, projectID, versionID, desire.DesiredReplicas, desire.Generation, desire.Reason, now); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return ErrDesiredCASConflict
			}
			return err
		}
		return tx.Commit()
	})
}

func activationCapacityAvailableTx(ctx context.Context, tx *sql.Tx, now time.Time) (bool, error) {
	var available int
	err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM runtime_nodes n
WHERE n.cordoned = 0
  AND n.lease_expiry > ?
  AND n.capacity_units > 0
  AND n.agent_base_url <> ''
  AND n.identity_uri <> ''
  AND n.zone <> ''
  AND n.capacity_units > (
    SELECT COUNT(*) FROM runtime_replicas r
    WHERE r.node_id = n.node_id
      AND r.state NOT IN (?, ?)
      AND r.assigned_node_generation > 0
      AND r.valid_until > ?
  )`, now.Format(time.RFC3339Nano), string(contract.ReplicaStopped), string(contract.ReplicaFailed), now.Format(time.RFC3339Nano)).Scan(&available)
	return available > 0, err
}

func (s *SQLiteStore) GetServingDesire(ctx context.Context, projectID, versionID string) (*ServingDesireRow, error) {
	return withRetry(func() (*ServingDesireRow, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT desired_replicas, generation, reason, updated_at FROM serving_desires
WHERE project_id = ? AND version_id = ?`, projectID, versionID)
		var r ServingDesireRow
		r.ProjectID = projectID
		r.VersionID = versionID
		var updated string
		if err := row.Scan(&r.DesiredReplicas, &r.Generation, &r.Reason, &updated); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			r.UpdatedAt = t
		}
		return &r, nil
	})
}

func (s *SQLiteStore) ActivateRuntimeNode(ctx context.Context, node contract.RuntimeNode, expectedGeneration int64) error {
	if expectedGeneration < 0 || node.Generation != expectedGeneration+1 || strings.TrimSpace(node.NodeID) == "" || node.CapacityUnits <= 0 || node.LeaseExpiry.IsZero() {
		return fmt.Errorf("invalid runtime node activation")
	}
	if err := contract.ValidateRuntimeNodeMetadataIfPresent(node); err != nil {
		return fmt.Errorf("runtime node metadata: %w", err)
	}
	cordoned := 0
	if node.Cordoned {
		cordoned = 1
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var res sql.Result
		var err error
		if expectedGeneration == 0 {
			res, err = s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO runtime_nodes (node_id, capacity_units, cordoned, lease_expiry, generation, agent_base_url, identity_uri, zone, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, node.NodeID, node.CapacityUnits, cordoned, node.LeaseExpiry.UTC().Format(time.RFC3339Nano), node.Generation, node.AgentBaseURL, node.IdentityURI, node.Zone, now)
		} else {
			res, err = s.db.ExecContext(ctx, `
UPDATE runtime_nodes SET capacity_units = ?, cordoned = ?, lease_expiry = ?, generation = ?, agent_base_url = ?, identity_uri = ?, zone = ?, updated_at = ?
WHERE node_id = ? AND generation = ?`, node.CapacityUnits, cordoned, node.LeaseExpiry.UTC().Format(time.RFC3339Nano), node.Generation, node.AgentBaseURL, node.IdentityURI, node.Zone, now, node.NodeID, expectedGeneration)
		}
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows != 1 {
			return ErrNodeLeaseCASConflict
		}
		return nil
	})
}

func (s *SQLiteStore) UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error {
	if strings.TrimSpace(node.NodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if node.CapacityUnits < 0 {
		return fmt.Errorf("capacity_units must be non-negative")
	}
	if node.Generation <= 0 {
		return fmt.Errorf("generation must be positive")
	}
	if err := contract.ValidateRuntimeNodeMetadataIfPresent(node); err != nil {
		return fmt.Errorf("runtime node metadata: %w", err)
	}
	return withRetryErr(func() error {
		cordoned := 0
		if node.Cordoned {
			cordoned = 1
		}
		expiry := node.LeaseExpiry.UTC()
		if expiry.IsZero() {
			expiry = time.Now().UTC().Add(24 * time.Hour)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_nodes (node_id, capacity_units, cordoned, lease_expiry, generation, agent_base_url, identity_uri, zone, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET
  capacity_units = excluded.capacity_units,
  cordoned = excluded.cordoned,
  lease_expiry = excluded.lease_expiry,
  generation = excluded.generation,
  agent_base_url = excluded.agent_base_url,
  identity_uri = excluded.identity_uri,
  zone = excluded.zone,
  updated_at = excluded.updated_at
WHERE runtime_nodes.generation <= excluded.generation
  AND (runtime_nodes.agent_base_url = '' OR runtime_nodes.agent_base_url = excluded.agent_base_url)
  AND (runtime_nodes.identity_uri = '' OR runtime_nodes.identity_uri = excluded.identity_uri)
  AND (runtime_nodes.zone = '' OR runtime_nodes.zone = excluded.zone)`,
			node.NodeID, node.CapacityUnits, cordoned, expiry.Format(time.RFC3339Nano), node.Generation,
			node.AgentBaseURL, node.IdentityURI, node.Zone, now)
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

func (s *SQLiteStore) GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error) {
	return withRetry(func() (*contract.RuntimeNode, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT capacity_units, cordoned, lease_expiry, generation, agent_base_url, identity_uri, zone FROM runtime_nodes WHERE node_id = ?`, nodeID)
		var n contract.RuntimeNode
		n.NodeID = nodeID
		var cordoned int
		var expiry string
		if err := row.Scan(&n.CapacityUnits, &cordoned, &expiry, &n.Generation, &n.AgentBaseURL, &n.IdentityURI, &n.Zone); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		n.Cordoned = cordoned == 1
		if t, err := time.Parse(time.RFC3339Nano, expiry); err == nil {
			n.LeaseExpiry = t
		}
		return &n, nil
	})
}

func (s *SQLiteStore) ListRuntimeNodes(ctx context.Context) ([]contract.RuntimeNode, error) {
	return withRetry(func() ([]contract.RuntimeNode, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT node_id, capacity_units, cordoned, lease_expiry, generation, agent_base_url, identity_uri, zone FROM runtime_nodes ORDER BY node_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []contract.RuntimeNode
		for rows.Next() {
			var n contract.RuntimeNode
			var cordoned int
			var expiry string
			if err := rows.Scan(&n.NodeID, &n.CapacityUnits, &cordoned, &expiry, &n.Generation, &n.AgentBaseURL, &n.IdentityURI, &n.Zone); err != nil {
				return nil, err
			}
			n.Cordoned = cordoned == 1
			if t, err := time.Parse(time.RFC3339Nano, expiry); err == nil {
				n.LeaseExpiry = t
			}
			out = append(out, n)
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) UpsertRuntimeReplica(ctx context.Context, rep contract.RuntimeReplica) error {
	return withRetryErr(func() error {
		var validUntil *string
		if rep.ValidUntil != nil {
			v := rep.ValidUntil.UTC().Format(time.RFC3339Nano)
			validUntil = &v
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_replicas (replica_id, project_id, version_id, node_id, generation, state, valid_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(replica_id) DO UPDATE SET
  node_id = excluded.node_id,
  generation = excluded.generation,
  state = excluded.state,
  valid_until = excluded.valid_until,
  updated_at = excluded.updated_at`,
			rep.ReplicaID, rep.ProjectID, rep.VersionID, rep.NodeID, rep.Generation, string(rep.State), validUntil, now)
		return err
	})
}

func (s *SQLiteStore) ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error) {
	return withRetry(func() ([]contract.RuntimeReplica, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT replica_id, node_id, generation, assigned_node_generation, state, valid_until FROM runtime_replicas
WHERE project_id = ? AND version_id = ? ORDER BY replica_id`, projectID, versionID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []contract.RuntimeReplica
		for rows.Next() {
			var rep contract.RuntimeReplica
			rep.ProjectID = projectID
			rep.VersionID = versionID
			var state string
			var assignedNodeGeneration sql.NullInt64
			var validUntil sql.NullString
			if err := rows.Scan(&rep.ReplicaID, &rep.NodeID, &rep.Generation, &assignedNodeGeneration, &state, &validUntil); err != nil {
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

func (s *SQLiteStore) TryAcquireControllerGuard(ctx context.Context, holderID string, pid int) error {
	return withRetryErr(func() error {
		const maxAttempts = 5
		for attempt := 0; attempt < maxAttempts; attempt++ {
			cur, err := s.GetControllerGuard(ctx)
			if err != nil {
				return err
			}
			if cur != nil && cur.HolderID != "" && cur.HolderID != holderID {
				if pidAlive(cur.HolderPID) {
					return ErrControllerGuardHeld
				}
				res, err := s.db.ExecContext(ctx, `
UPDATE controller_guard SET holder_id = NULL, acquired_at = NULL, holder_pid = 0
WHERE id = 1 AND holder_id = ? AND holder_pid = ?`, cur.HolderID, cur.HolderPID)
				if err != nil {
					return err
				}
				n, _ := res.RowsAffected()
				if n == 0 {
					continue
				}
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			res, err := s.db.ExecContext(ctx, `
	UPDATE controller_guard SET holder_id = ?, acquired_at = ?, holder_pid = ?
	WHERE id = 1 AND (holder_id IS NULL OR holder_id = '' OR holder_id = ?)`, holderID, now, pid, holderID)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				continue
			}
			return nil
		}
		return ErrControllerGuardHeld
	})
}

func (s *SQLiteStore) ReleaseControllerGuard(ctx context.Context, holderID string) error {
	return withRetryErr(func() error {
		res, err := s.db.ExecContext(ctx, `
UPDATE controller_guard SET holder_id = NULL, acquired_at = NULL, holder_pid = 0
WHERE id = 1 AND holder_id = ?`, holderID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("controller guard not held by %q", holderID)
		}
		return nil
	})
}

func (s *SQLiteStore) GetControllerGuard(ctx context.Context) (*ControllerGuardState, error) {
	return withRetry(func() (*ControllerGuardState, error) {
		var holder sql.NullString
		var acquired sql.NullString
		var pid int
		err := s.db.QueryRowContext(ctx, `SELECT holder_id, acquired_at, holder_pid FROM controller_guard WHERE id = 1`).
			Scan(&holder, &acquired, &pid)
		if err != nil {
			return nil, err
		}
		st := &ControllerGuardState{HolderPID: pid}
		if holder.Valid {
			st.HolderID = holder.String
		}
		if acquired.Valid && acquired.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, acquired.String); err == nil {
				st.AcquiredAt = &t
			}
		}
		return st, nil
	})
}

func (s *SQLiteStore) BuildLegacyRouteSnapshot(ctx context.Context) (contract.RouteSnapshot, error) {
	snap, _, err := s.buildRouteSnapshot(ctx, -1)
	return snap, err
}

func (s *SQLiteStore) BuildSnapshotAfter(ctx context.Context, afterRevision int64) (contract.RouteSnapshot, bool, error) {
	return s.buildRouteSnapshot(ctx, afterRevision)
}
