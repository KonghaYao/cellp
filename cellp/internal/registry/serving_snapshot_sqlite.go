package registry

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

type routeSnapshotResult struct {
	snap contract.RouteSnapshot
	ok   bool
}

func (s *SQLiteStore) buildRouteSnapshot(ctx context.Context, afterRevision int64) (contract.RouteSnapshot, bool, error) {
	res, err := withRetry(func() (routeSnapshotResult, error) {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return routeSnapshotResult{}, err
		}
		defer tx.Rollback()

		var rev int64
		if err := tx.QueryRowContext(ctx, `SELECT route_revision FROM control_plane_meta WHERE id = 1`).Scan(&rev); err != nil {
			return routeSnapshotResult{}, err
		}
		if afterRevision >= 0 && rev <= afterRevision {
			return routeSnapshotResult{}, nil
		}
		var policyRev int64
		if err := tx.QueryRowContext(ctx, `SELECT policy_revision FROM control_plane_meta WHERE id = 1`).Scan(&policyRev); err != nil {
			return routeSnapshotResult{}, err
		}

		bindings, err := listActiveIngressBindingsTx(ctx, tx)
		if err != nil {
			return routeSnapshotResult{}, err
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

		now := time.Now().UTC()
		endpointSets := make(map[string]contract.EndpointSet)
		elasticKeys, err := elasticPublicEndpointSetsTx(ctx, tx, now, endpointSets)
		if err != nil {
			return routeSnapshotResult{}, err
		}
		if err := legacyRouteEndpointSetsTx(ctx, tx, elasticKeys, endpointSets); err != nil {
			return routeSnapshotResult{}, err
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
		if err := contract.ValidateRouteSnapshot(afterRevision, snap, now); err != nil && rev > 0 {
			return routeSnapshotResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return routeSnapshotResult{}, err
		}
		return routeSnapshotResult{snap: snap, ok: true}, nil
	})
	return res.snap, res.ok, err
}

func (s *SQLiteStore) BuildQualificationViewAfter(ctx context.Context, afterRevision int64) (QualificationView, bool, error) {
	var view QualificationView
	var ok bool
	err := withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var rev int64
		if err := tx.QueryRowContext(ctx, `SELECT route_revision FROM control_plane_meta WHERE id = 1`).Scan(&rev); err != nil {
			return err
		}
		if afterRevision >= 0 && rev <= afterRevision {
			ok = false
			return tx.Commit()
		}
		now := time.Now().UTC()
		endpointSets := make(map[string]contract.EndpointSet)
		if err := qualificationEndpointSetsTx(ctx, tx, now, endpointSets); err != nil {
			return err
		}
		var sets []contract.EndpointSet
		for _, es := range endpointSets {
			if len(es.Endpoints) > 0 {
				sets = append(sets, es)
			}
		}
		view = QualificationView{RouteRevision: rev, EndpointSets: sets}
		ok = true
		return tx.Commit()
	})
	return view, ok, err
}

func listActiveIngressBindingsTx(ctx context.Context, tx *sql.Tx) ([]IngressBinding, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+ingressSelectCols+` FROM ingress_bindings WHERE active = 1 ORDER BY project_id, binding_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIngressBindings(rows)
}

func elasticPublicEndpointSetsTx(ctx context.Context, tx *sql.Tx, now time.Time, out map[string]contract.EndpointSet) (map[string]bool, error) {
	elasticKeys := make(map[string]bool)
	rows, err := tx.QueryContext(ctx, `
SELECT r.replica_id, r.project_id, r.version_id,
       e.listen_host, e.listen_port, e.valid_until, e.endpoint_state,
       v.status, v.elastic_enrolled,
       n.lease_expiry, n.generation, n.cordoned,
       r.valid_until, r.assigned_node_generation
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
LEFT JOIN runtime_replica_endpoints e ON e.replica_id = r.replica_id
JOIN versions v ON v.project_id = r.project_id AND v.id = r.version_id
WHERE r.state = ? AND v.elastic_enrolled = 1`,
		string(contract.ReplicaReady))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		ep, key, err := scanElasticSnapshotRow(rows, now, "snapshot", func(versionStatus string) bool {
			return contract.IsServingQualifiedReady(versionStatus)
		})
		if err != nil {
			return nil, err
		}
		if ep == nil {
			continue
		}
		elasticKeys[key] = true
		es := out[key]
		es.ProjectID = ep.projectID
		es.VersionID = ep.versionID
		es.Endpoints = append(es.Endpoints, ep.endpoint)
		out[key] = es
	}
	return elasticKeys, rows.Err()
}

func qualificationEndpointSetsTx(ctx context.Context, tx *sql.Tx, now time.Time, out map[string]contract.EndpointSet) error {
	rows, err := tx.QueryContext(ctx, `
SELECT r.replica_id, r.project_id, r.version_id,
       e.listen_host, e.listen_port, e.valid_until, e.endpoint_state,
       v.status, v.elastic_enrolled,
       n.lease_expiry, n.generation, n.cordoned,
       r.valid_until, r.assigned_node_generation
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
LEFT JOIN runtime_replica_endpoints e ON e.replica_id = r.replica_id
JOIN versions v ON v.project_id = r.project_id AND v.id = r.version_id
WHERE r.state = ? AND v.status = ? AND v.elastic_enrolled = 1`,
		string(contract.ReplicaReady), contract.StatusDeployReady)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		ep, key, err := scanElasticSnapshotRow(rows, now, "qualification", func(string) bool { return true })
		if err != nil {
			return err
		}
		if ep == nil {
			continue
		}
		es := out[key]
		es.ProjectID = ep.projectID
		es.VersionID = ep.versionID
		es.Endpoints = append(es.Endpoints, ep.endpoint)
		out[key] = es
	}
	return rows.Err()
}

type elasticSnapshotEndpoint struct {
	projectID string
	versionID string
	endpoint  contract.Endpoint
}

func scanElasticSnapshotRow(rows *sql.Rows, now time.Time, scope string, versionOK func(string) bool) (*elasticSnapshotEndpoint, string, error) {
	var replicaID, projectID, versionID string
	var host, epValid, epState sql.NullString
	var port sql.NullInt64
	var versionStatus string
	var enrolled int
	var nodeLease, assignValid string
	var nodeGen int64
	var cordoned int
	var assigned sql.NullInt64
	if err := rows.Scan(&replicaID, &projectID, &versionID, &host, &port, &epValid, &epState,
		&versionStatus, &enrolled, &nodeLease, &nodeGen, &cordoned, &assignValid, &assigned); err != nil {
		return nil, "", err
	}
	if enrolled != 1 {
		return nil, "", nil
	}
	if !versionOK(versionStatus) {
		return nil, "", nil
	}
	if cordoned != 0 {
		return nil, "", nil
	}
	if !assigned.Valid || assigned.Int64 != nodeGen {
		return nil, "", nil
	}
	if !epState.Valid || strings.TrimSpace(epState.String) == "" {
		return nil, "", fmt.Errorf("%s endpoint state corrupt", scope)
	}
	switch contract.EndpointState(epState.String) {
	case contract.EndpointDraining:
		return nil, "", nil
	case contract.EndpointReady:
	default:
		return nil, "", fmt.Errorf("%s endpoint state corrupt", scope)
	}
	if !host.Valid || strings.TrimSpace(host.String) == "" || !port.Valid || port.Int64 <= 0 {
		return nil, "", fmt.Errorf("%s endpoint listen address corrupt", scope)
	}
	if !epValid.Valid || strings.TrimSpace(epValid.String) == "" {
		return nil, "", fmt.Errorf("%s endpoint timestamp corrupt", scope)
	}
	nodeExpiry, err := parseRFC3339NanoRequired(nodeLease)
	if err != nil {
		return nil, "", fmt.Errorf("%s node lease timestamp corrupt", scope)
	}
	if !nodeExpiry.After(now) {
		return nil, "", nil
	}
	assignUntil, err := parseRFC3339NanoRequired(assignValid)
	if err != nil {
		return nil, "", fmt.Errorf("%s assignment timestamp corrupt", scope)
	}
	if !assignUntil.After(now) {
		return nil, "", nil
	}
	endpointUntil, err := parseRFC3339NanoRequired(epValid.String)
	if err != nil {
		return nil, "", fmt.Errorf("%s endpoint timestamp corrupt", scope)
	}
	if !endpointUntil.After(now) {
		return nil, "", nil
	}
	lkg := routingLKGValidUntil(nodeExpiry, assignUntil, endpointUntil)
	key := projectID + "/" + versionID
	return &elasticSnapshotEndpoint{
		projectID: projectID,
		versionID: versionID,
		endpoint: contract.Endpoint{
			ReplicaID:  replicaID,
			Address:    net.JoinHostPort(host.String, strconv.FormatInt(port.Int64, 10)),
			State:      contract.EndpointReady,
			ValidUntil: &lkg,
		},
	}, key, nil
}

func legacyRouteEndpointSetsTx(ctx context.Context, tx *sql.Tx, elasticKeys map[string]bool, endpointSets map[string]contract.EndpointSet) error {
	rows, err := tx.QueryContext(ctx, `
SELECT r.project_id, r.version_id, r.upstream_host, r.upstream_port, v.status, v.elastic_enrolled
FROM routes r
JOIN versions v ON v.project_id = r.project_id AND v.id = r.version_id
WHERE r.active = 1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, versionID, host string
		var port int
		var status string
		var enrolled int
		if err := rows.Scan(&projectID, &versionID, &host, &port, &status, &enrolled); err != nil {
			return err
		}
		key := projectID + "/" + versionID
		if elasticKeys[key] || enrolled == 1 {
			continue
		}
		if !contract.IsServingQualifiedReady(status) {
			continue
		}
		es := endpointSets[key]
		es.ProjectID = projectID
		es.VersionID = versionID
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		es.Endpoints = append(es.Endpoints, contract.Endpoint{
			ReplicaID: "legacy-" + versionID,
			Address:   addr,
			State:     contract.EndpointReady,
		})
		endpointSets[key] = es
	}
	return rows.Err()
}
