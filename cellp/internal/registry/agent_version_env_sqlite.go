package registry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
)

const agentVersionEnvAuthSQL = `
SELECT 1
FROM runtime_replicas r
JOIN runtime_nodes n ON n.node_id = r.node_id
WHERE r.node_id = ? AND r.project_id = ? AND r.version_id = ?
  AND r.state IN (?, ?, ?)
  AND r.valid_until > ?
  AND r.assigned_node_generation > 0
  AND r.assigned_node_generation = n.generation
  AND n.cordoned = 0
  AND n.lease_expiry > ?
LIMIT 1`

// GetAuthorizedAgentVersionEnv reads worker env only when the node still holds a live
// assignment for project/version at now. Authorization and env read share one read-only
// SQLite transaction so a concurrent assignment revocation cannot pass a separate
// authorize-then-read window (SERIALIZABLE snapshot for the transaction lifetime).
func (s *SQLiteStore) GetAuthorizedAgentVersionEnv(ctx context.Context, nodeID, projectID, versionID string, now time.Time) (map[string]string, error) {
	return withRetry(func() (map[string]string, error) {
		now = now.UTC()
		nowStr := now.Format(time.RFC3339Nano)
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()

		var marker int
		err = tx.QueryRowContext(ctx, agentVersionEnvAuthSQL,
			nodeID, projectID, versionID,
			string(contract.ReplicaPending), string(contract.ReplicaStarting), string(contract.ReplicaReady),
			nowStr, nowStr,
		).Scan(&marker)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentVersionEnvForbidden
		}
		if err != nil {
			return nil, err
		}

		var raw sql.NullString
		err = tx.QueryRowContext(ctx,
			`SELECT env_json FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID,
		).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("version not found")
		}
		if err != nil {
			return nil, err
		}
		env := unmarshalEnvJSON(raw.String)
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return env, nil
	})
}
