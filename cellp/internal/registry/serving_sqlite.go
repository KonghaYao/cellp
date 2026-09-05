package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
		enrolled := 0
		if row.ElasticEnrolled {
			enrolled = 1
		}
		_, err := s.db.ExecContext(ctx, `
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
		_, err = s.db.ExecContext(ctx,
			`UPDATE versions SET elastic_enrolled = ? WHERE project_id = ? AND id = ?`,
			enrolled, row.ProjectID, row.VersionID)
		return err
	})
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

func (s *SQLiteStore) UpsertRuntimeNode(ctx context.Context, node contract.RuntimeNode) error {
	if strings.TrimSpace(node.NodeID) == "" {
		return fmt.Errorf("node_id required")
	}
	if node.CapacityUnits < 0 {
		return fmt.Errorf("capacity_units must be non-negative")
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
		_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_nodes (node_id, capacity_units, cordoned, lease_expiry, generation, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET
  capacity_units = excluded.capacity_units,
  cordoned = excluded.cordoned,
  lease_expiry = excluded.lease_expiry,
  generation = excluded.generation,
  updated_at = excluded.updated_at`,
			node.NodeID, node.CapacityUnits, cordoned, expiry.Format(time.RFC3339Nano), node.Generation, now)
		return err
	})
}

func (s *SQLiteStore) GetRuntimeNode(ctx context.Context, nodeID string) (*contract.RuntimeNode, error) {
	return withRetry(func() (*contract.RuntimeNode, error) {
		row := s.db.QueryRowContext(ctx, `
SELECT capacity_units, cordoned, lease_expiry, generation FROM runtime_nodes WHERE node_id = ?`, nodeID)
		var n contract.RuntimeNode
		n.NodeID = nodeID
		var cordoned int
		var expiry string
		if err := row.Scan(&n.CapacityUnits, &cordoned, &expiry, &n.Generation); err != nil {
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
SELECT node_id, capacity_units, cordoned, lease_expiry, generation FROM runtime_nodes ORDER BY node_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []contract.RuntimeNode
		for rows.Next() {
			var n contract.RuntimeNode
			var cordoned int
			var expiry string
			if err := rows.Scan(&n.NodeID, &n.CapacityUnits, &cordoned, &expiry, &n.Generation); err != nil {
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
SELECT replica_id, node_id, generation, state, valid_until FROM runtime_replicas
WHERE project_id = ? AND version_id = ?`, projectID, versionID)
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
			var validUntil sql.NullString
			if err := rows.Scan(&rep.ReplicaID, &rep.NodeID, &rep.Generation, &state, &validUntil); err != nil {
				return nil, err
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
		cur, err := s.GetControllerGuard(ctx)
		if err != nil {
			return err
		}
		if cur != nil && cur.HolderID != "" && cur.HolderID != holderID {
			if pidAlive(cur.HolderPID) {
				return ErrControllerGuardHeld
			}
			_, _ = s.db.ExecContext(ctx, `
UPDATE controller_guard SET holder_id = NULL, acquired_at = NULL, holder_pid = 0 WHERE id = 1`)
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
			return ErrControllerGuardHeld
		}
		return nil
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
	return withRetry(func() (contract.RouteSnapshot, error) {
		rev, err := s.GetRouteRevision(ctx)
		if err != nil {
			return contract.RouteSnapshot{}, err
		}
		var policyRev int64
		_ = s.db.QueryRowContext(ctx, `SELECT policy_revision FROM control_plane_meta WHERE id = 1`).Scan(&policyRev)

		bindings, err := s.ListActiveIngressBindings(ctx)
		if err != nil {
			return contract.RouteSnapshot{}, err
		}
		var snapBindings []contract.IngressBinding
		for _, b := range bindings {
			if b.VersionID == nil {
				continue
			}
			host := b.SyntheticHost
			if b.Host != nil && *b.Host != "" {
				host = *b.Host
			}
			port := 0
			if b.ListenPort != nil {
				port = *b.ListenPort
			}
			snapBindings = append(snapBindings, contract.IngressBinding{
				Role:       b.Role,
				Host:       host,
				ListenPort: port,
				ProjectID:  b.ProjectID,
				VersionID:  *b.VersionID,
			})
		}

		routes, err := s.ListAllActiveRoutes(ctx)
		if err != nil {
			return contract.RouteSnapshot{}, err
		}
		endpointSets := make(map[string]contract.EndpointSet)
		for _, r := range routes {
			v, err := s.GetVersion(ctx, r.ProjectID, r.VersionID)
			if err != nil || v == nil {
				continue
			}
			if v.Status != StatusReady {
				continue
			}
			key := r.ProjectID + "/" + r.VersionID
			es := endpointSets[key]
			es.ProjectID = r.ProjectID
			es.VersionID = r.VersionID
			addr := r.UpstreamHost + ":" + strconv.Itoa(r.UpstreamPort)
			es.Endpoints = append(es.Endpoints, contract.Endpoint{
				ReplicaID: "legacy-" + r.VersionID,
				Address:   addr,
				State:     contract.EndpointReady,
			})
			endpointSets[key] = es
		}
		var sets []contract.EndpointSet
		for _, es := range endpointSets {
			if len(es.Endpoints) > 0 {
				sets = append(sets, es)
			}
		}
		snap := contract.RouteSnapshot{
			Revision:       rev,
			PolicyRevision: policyRev,
			Bindings:       snapBindings,
			EndpointSets:   sets,
		}
		if err := contract.ValidateRouteSnapshot(0, snap, time.Now().UTC()); err != nil && rev > 0 {
			return contract.RouteSnapshot{}, err
		}
		return snap, nil
	})
}
