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

// deployOperationAssertInTx verifies the version deploy operation is held by attempt with a live lease.
func deployOperationAssertInTx(ctx context.Context, tx *sql.Tx, projectID, versionID string, attempt DeployAttempt, now time.Time) error {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if projectID == "" || versionID == "" || attempt.JobID == "" {
		return ErrDeployOperationNotOwner
	}
	var holder sql.NullString
	var holderUntil sql.NullString
	var holderEpoch int64
	err := tx.QueryRowContext(ctx, `
SELECT deploy_operation_job_id, deploy_operation_lease_until, deploy_operation_claim_epoch
FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&holder, &holderUntil, &holderEpoch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeployOperationNotOwner
		}
		return err
	}
	if !holder.Valid || strings.TrimSpace(holder.String) != attempt.JobID {
		return ErrDeployOperationNotOwner
	}
	if holderEpoch != attempt.ClaimEpoch {
		return ErrDeployOperationNotOwner
	}
	if holderUntil.Valid && holderUntil.String != "" {
		t, parseErr := time.Parse(time.RFC3339Nano, holderUntil.String)
		if parseErr == nil && !t.After(now) {
			return ErrDeployOperationNotOwner
		}
	}
	var jobEpoch int64
	jerr := tx.QueryRowContext(ctx, `SELECT claim_epoch FROM jobs WHERE id = ?`, attempt.JobID).Scan(&jobEpoch)
	if jerr != nil {
		if errors.Is(jerr, sql.ErrNoRows) {
			return ErrDeployOperationNotOwner
		}
		return jerr
	}
	if jobEpoch != attempt.ClaimEpoch {
		return ErrDeployOperationNotOwner
	}
	return nil
}

func compareAndSetDesiredInTx(ctx context.Context, tx *sql.Tx, projectID, versionID string, expectGen int64, desire ServingDesireRow) error {
	if expectGen == 0 {
		if desire.Generation != 1 {
			return ErrDesiredCASConflict
		}
	} else if desire.Generation != expectGen+1 {
		return ErrDesiredCASConflict
	}
	now := desire.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowStr := now.Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `
UPDATE serving_desires SET desired_replicas = ?, generation = ?, reason = ?, updated_at = ?
WHERE project_id = ? AND version_id = ? AND generation = ?`,
		desire.DesiredReplicas, desire.Generation, desire.Reason, nowStr,
		projectID, versionID, expectGen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		return nil
	}
	if expectGen != 0 {
		return ErrDesiredCASConflict
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO serving_desires (project_id, version_id, desired_replicas, generation, reason, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, versionID, desire.DesiredReplicas, desire.Generation, desire.Reason, nowStr)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrDesiredCASConflict
		}
		return err
	}
	return nil
}

// UpdateVersionStatusForDeployOperation atomically checks deploy operation ownership then updates version status.
func (s *SQLiteStore) UpdateVersionStatusForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, status string, errMsg *string) error {
	return withRetryErr(func() error {
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if err := deployOperationAssertInTx(ctx, tx, projectID, versionID, attempt, now); err != nil {
			return err
		}

		var oldStatus string
		err = tx.QueryRowContext(ctx,
			`SELECT status FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&oldStatus)
		if err == sql.ErrNoRows {
			return ErrDeployOperationNotOwner
		}
		if err != nil {
			return err
		}

		var readyAt interface{}
		var lastAccess interface{}
		if status == StatusReady {
			readyAt = nowStr
			lastAccess = nowStr
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE versions SET status = ?, error = ?, updated_at = ?,
				ready_at = COALESCE(?, ready_at),
				last_access_at = COALESCE(?, last_access_at, ready_at),
				data_branch = COALESCE(data_branch, ?)
			WHERE project_id = ? AND id = ?`,
			status, nullStr(errMsg), nowStr, readyAt, lastAccess,
			fmt.Sprintf("%s/%s", projectID, versionID),
			projectID, versionID)
		if err != nil {
			return err
		}
		if contract.IsServingQualifiedReady(oldStatus) != contract.IsServingQualifiedReady(status) {
			if _, err = tx.ExecContext(ctx,
				`UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// SetRouteForDeployOperation atomically checks deploy operation ownership then upserts the route.
func (s *SQLiteStore) SetRouteForDeployOperation(ctx context.Context, attempt DeployAttempt, route Route) error {
	return withRetryErr(func() error {
		now := time.Now().UTC()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := deployOperationAssertInTx(ctx, tx, route.ProjectID, route.VersionID, attempt, now); err != nil {
			return err
		}
		if err := s.setRouteInTx(ctx, tx, route); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// SetRouteActiveForDeployOperation atomically checks deploy operation ownership then toggles route active.
func (s *SQLiteStore) SetRouteActiveForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, active bool) error {
	return withRetryErr(func() error {
		now := time.Now().UTC()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := deployOperationAssertInTx(ctx, tx, projectID, versionID, attempt, now); err != nil {
			return err
		}
		activeInt := 0
		if active {
			activeInt = 1
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE routes SET active = ? WHERE project_id = ? AND version_id = ?`,
			activeInt, projectID, versionID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE control_plane_meta SET route_revision = route_revision + 1 WHERE id = 1`); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// CompareAndSetDesiredForDeployOperation atomically checks deploy operation ownership then CAS-desires.
func (s *SQLiteStore) CompareAndSetDesiredForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, expectGen int64, desire ServingDesireRow) error {
	return withRetryErr(func() error {
		now := time.Now().UTC()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := deployOperationAssertInTx(ctx, tx, projectID, versionID, attempt, now); err != nil {
			return err
		}
		if err := compareAndSetDesiredInTx(ctx, tx, projectID, versionID, expectGen, desire); err != nil {
			return err
		}
		return tx.Commit()
	})
}
