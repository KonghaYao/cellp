package registry

import "strings"

func (s *SQLiteStore) migrateElasticServing() error {
	if _, err := s.db.Exec(`
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
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_replicas (
  replica_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  version_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  state TEXT NOT NULL,
  valid_until TEXT,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (project_id, version_id) REFERENCES versions(project_id, id)
);

CREATE INDEX IF NOT EXISTS idx_runtime_replicas_version ON runtime_replicas(project_id, version_id);

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
	alters := []string{
		`ALTER TABLE versions ADD COLUMN elastic_enrolled INTEGER NOT NULL DEFAULT 0`,
	}
	for _, q := range alters {
		if _, err := s.db.Exec(q); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}
