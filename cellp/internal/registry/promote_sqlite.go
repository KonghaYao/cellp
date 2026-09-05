package registry

import (
	"context"
	"fmt"
	"time"
)

// CommitProdPromote atomically CAS-updates prod_version_id, activates the new prod route,
// and bumps route_revision once (AD-15 E5). Used when CELLP_ELASTIC_RUNTIME=1.
func (s *SQLiteStore) CommitProdPromote(ctx context.Context, projectID, expectedProd, newProd string) (int64, error) {
	return withRetry(func() (int64, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()

		now := time.Now().UTC().Format(time.RFC3339Nano)
		var prevProd interface{}
		var prevAt interface{}
		if expectedProd != "" {
			prevProd = expectedProd
			prevAt = now
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE projects SET prod_version_id = ?,
				previous_prod_version_id = CASE WHEN ? = '' THEN previous_prod_version_id ELSE ? END,
				previous_prod_at = CASE WHEN ? = '' THEN previous_prod_at ELSE ? END
			WHERE id = ? AND (prod_version_id = ? OR (prod_version_id IS NULL AND ? = ''))`,
			newProd, expectedProd, prevProd, expectedProd, prevAt, projectID, nullIfEmpty(expectedProd), expectedProd)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, fmt.Errorf("CAS prod_version failed: expected %q", expectedProd)
		}

		res, err = tx.ExecContext(ctx,
			`UPDATE routes SET active = 1 WHERE project_id = ? AND version_id = ?`,
			projectID, newProd)
		if err != nil {
			return 0, err
		}
		if affected, _ := res.RowsAffected(); affected == 0 {
			return 0, fmt.Errorf("prod route not found for version %q", newProd)
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`); err != nil {
			return 0, err
		}
		var rev int64
		if err := tx.QueryRowContext(ctx, `SELECT route_revision FROM control_plane_meta WHERE id = 1`).Scan(&rev); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return rev, nil
	})
}
