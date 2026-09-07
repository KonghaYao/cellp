package registry

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *SQLiteStore) migrateElasticServing(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS control_plane_meta (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  route_revision INTEGER NOT NULL DEFAULT 0,
  policy_revision INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO control_plane_meta (id, route_revision, policy_revision) VALUES (1, 0, 0);

CREATE TABLE IF NOT EXISTS serving_policies (
  project_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  min_replicas INTEGER NOT NULL DEFAULT 0,
  max_replicas INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 0,
  background_mode TEXT NOT NULL DEFAULT 'none',
  elastic_enrolled INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (project_id, version_id),
  FOREIGN KEY (project_id, version_id) REFERENCES versions(project_id, id)
);

CREATE TABLE IF NOT EXISTS serving_desires (
  project_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  desired_replicas INTEGER NOT NULL,
  generation INTEGER NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (project_id, version_id),
  FOREIGN KEY (project_id, version_id) REFERENCES versions(project_id, id)
);

CREATE TABLE IF NOT EXISTS runtime_nodes (
  node_id TEXT PRIMARY KEY,
  capacity_units INTEGER NOT NULL DEFAULT 0,
  cordoned INTEGER NOT NULL DEFAULT 0,
  lease_expiry TEXT NOT NULL,
  generation INTEGER NOT NULL,
  agent_base_url TEXT NOT NULL DEFAULT '',
  identity_uri TEXT NOT NULL DEFAULT '',
  zone TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_replicas (
  replica_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  assigned_node_generation INTEGER,
  state TEXT NOT NULL,
  valid_until TEXT,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id, version_id) REFERENCES versions(project_id, id)
);

CREATE INDEX IF NOT EXISTS idx_runtime_replicas_version ON runtime_replicas(project_id, version_id);
CREATE INDEX IF NOT EXISTS idx_runtime_replicas_node ON runtime_replicas(node_id);
CREATE INDEX IF NOT EXISTS idx_runtime_replicas_state ON runtime_replicas(project_id, version_id, state);

CREATE TABLE IF NOT EXISTS runtime_replica_endpoints (
  replica_id TEXT PRIMARY KEY,
  listen_host TEXT NOT NULL,
  listen_port INTEGER NOT NULL,
  endpoint_state TEXT NOT NULL DEFAULT 'ready',
  valid_until TEXT,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (replica_id) REFERENCES runtime_replicas(replica_id)
);
CREATE INDEX IF NOT EXISTS idx_runtime_replica_endpoints_state ON runtime_replica_endpoints(endpoint_state);

CREATE TABLE IF NOT EXISTS runtime_agent_commands (
  idempotency_key TEXT PRIMARY KEY,
  action TEXT NOT NULL,
  node_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  replica_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  status TEXT NOT NULL,
  result_state TEXT,
  reason TEXT,
  expires_at TEXT NOT NULL,
  lease_expires_at TEXT,
  attempt_token TEXT,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runtime_agent_commands_expiry ON runtime_agent_commands(expires_at);

CREATE TABLE IF NOT EXISTS controller_guard (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  holder_id TEXT,
  acquired_at TEXT,
  holder_pid INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO controller_guard (id, holder_id, acquired_at, holder_pid) VALUES (1, NULL, NULL, 0);
`); err != nil {
		return err
	}
	alters := []elasticAlter{
		{table: "versions", column: "elastic_enrolled", query: `ALTER TABLE versions ADD COLUMN elastic_enrolled INTEGER NOT NULL DEFAULT 0`},
		{table: "runtime_replicas", column: "assigned_node_generation", query: `ALTER TABLE runtime_replicas ADD COLUMN assigned_node_generation INTEGER`},
		{table: "runtime_nodes", column: "agent_base_url", query: `ALTER TABLE runtime_nodes ADD COLUMN agent_base_url TEXT NOT NULL DEFAULT ''`},
		{table: "runtime_nodes", column: "identity_uri", query: `ALTER TABLE runtime_nodes ADD COLUMN identity_uri TEXT NOT NULL DEFAULT ''`},
		{table: "runtime_nodes", column: "zone", query: `ALTER TABLE runtime_nodes ADD COLUMN zone TEXT NOT NULL DEFAULT ''`},
		{table: "runtime_agent_commands", column: "lease_expires_at", query: `ALTER TABLE runtime_agent_commands ADD COLUMN lease_expires_at TEXT`},
		{table: "runtime_agent_commands", column: "attempt_token", query: `ALTER TABLE runtime_agent_commands ADD COLUMN attempt_token TEXT`},
		{table: "versions", column: "deploy_operation_job_id", query: `ALTER TABLE versions ADD COLUMN deploy_operation_job_id TEXT`},
		{table: "versions", column: "deploy_operation_lease_until", query: `ALTER TABLE versions ADD COLUMN deploy_operation_lease_until TEXT`},
		{table: "jobs", column: "claimed_worker_id", query: `ALTER TABLE jobs ADD COLUMN claimed_worker_id TEXT`},
		{table: "jobs", column: "claim_epoch", query: `ALTER TABLE jobs ADD COLUMN claim_epoch INTEGER NOT NULL DEFAULT 0`},
		{table: "versions", column: "deploy_operation_claim_epoch", query: `ALTER TABLE versions ADD COLUMN deploy_operation_claim_epoch INTEGER NOT NULL DEFAULT 0`},
	}
	for _, alter := range alters {
		has, err := sqliteTableHasColumn(ctx, tx, alter.table, alter.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := tx.ExecContext(ctx, alter.query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type elasticAlter struct {
	table  string
	column string
	query  string
}

func sqliteTableHasColumn(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
