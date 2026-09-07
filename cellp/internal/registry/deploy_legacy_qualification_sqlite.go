package registry

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) ListLegacyDeployQualificationRecoveryCandidates(ctx context.Context) ([]DeployCompensationWork, error) {
	return withRetry(func() ([]DeployCompensationWork, error) {
		rows, err := s.db.QueryContext(ctx, `
SELECT d.project_id, d.version_id
FROM serving_desires d
INNER JOIN versions v ON v.project_id = d.project_id AND v.id = d.version_id
WHERE d.reason = ? AND v.status = ?`,
			deployQualificationReasonLegacyExact, StatusFailed)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []DeployCompensationWork
		for rows.Next() {
			var projectID, versionID string
			if err := rows.Scan(&projectID, &versionID); err != nil {
				return nil, err
			}
			out = append(out, DeployCompensationWork{ProjectID: projectID, VersionID: versionID})
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) RecoverLegacyDeployQualificationIfUnique(ctx context.Context, projectID, versionID string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	if projectID == "" || versionID == "" {
		return false, nil
	}
	return withRetry(func() (bool, error) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)

		var versionStatus string
		err = tx.QueryRowContext(ctx, `
SELECT status FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&versionStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if versionStatus != StatusFailed {
			return false, nil
		}

		var desireReason string
		var desireGen int64
		err = tx.QueryRowContext(ctx, `
SELECT reason, generation FROM serving_desires WHERE project_id = ? AND version_id = ?`,
			projectID, versionID).Scan(&desireReason, &desireGen)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(desireReason) != deployQualificationReasonLegacyExact {
			return false, nil
		}

		rows, err := tx.QueryContext(ctx, `
SELECT id FROM jobs
WHERE project_id = ? AND version_id = ?
  AND (
    status IN ('compensating', 'failed')
    OR (status = 'claimed' AND step LIKE ?)
  )`,
			projectID, versionID, compensatingStepPrefix+"%")
		if err != nil {
			return false, err
		}
		defer rows.Close()
		var candidates []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return false, err
			}
			candidates = append(candidates, strings.TrimSpace(id))
		}
		if err := rows.Err(); err != nil {
			return false, err
		}
		if len(candidates) != 1 {
			return false, nil
		}
		jobID := candidates[0]
		if jobID == "" {
			return false, nil
		}

		var holderJob sql.NullString
		var holderUntil sql.NullString
		err = tx.QueryRowContext(ctx, `
SELECT deploy_operation_job_id, deploy_operation_lease_until
FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&holderJob, &holderUntil)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if holderJob.Valid {
			holderID := strings.TrimSpace(holderJob.String)
			if holderID != "" && holderID != jobID {
				active := false
				if holderUntil.Valid && holderUntil.String != "" {
					t, parseErr := time.Parse(time.RFC3339Nano, holderUntil.String)
					if parseErr == nil && t.After(now) {
						active = true
					}
				}
				if active {
					return false, nil
				}
			}
		}

		var otherActive int
		otherRows, err := tx.QueryContext(ctx, `
SELECT lease_until FROM jobs
WHERE project_id = ? AND version_id = ? AND id <> ?
  AND status = 'claimed'
  AND step NOT LIKE ?`, projectID, versionID, jobID, compensatingStepPrefix+"%")
		if err != nil {
			return false, err
		}
		for otherRows.Next() {
			var lease sql.NullString
			if err := otherRows.Scan(&lease); err != nil {
				otherRows.Close()
				return false, err
			}
			if jobWorkerLeaseIsLive(lease, now) {
				otherActive++
			}
		}
		otherRows.Close()
		if err := otherRows.Err(); err != nil {
			return false, err
		}
		if otherActive > 0 {
			return false, nil
		}

		activeClaim, err := jobHasActiveWorkerClaim(ctx, tx, jobID, now)
		if err != nil {
			return false, err
		}
		if activeClaim {
			return false, nil
		}

		newReason := deployQualificationReasonPrefix + jobID
		res, err := tx.ExecContext(ctx, `
UPDATE serving_desires SET reason = ?, generation = generation + 1, updated_at = ?
WHERE project_id = ? AND version_id = ? AND reason = ? AND generation = ?`,
			newReason, nowStr, projectID, versionID, deployQualificationReasonLegacyExact, desireGen)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return false, nil
		}

		var jobStatus string
		var jobLease sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT status, lease_until FROM jobs WHERE id = ?`, jobID).Scan(&jobStatus, &jobLease)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if jobStatus == "claimed" && jobWorkerLeaseIsLive(jobLease, now) {
			return false, nil
		}
		leaseWhere, leaseArg := jobLeaseExactMatchSQL(jobLease)
		jobUpdate := `
UPDATE jobs SET status = 'compensating', lease_until = NULL, claimed_worker_id = NULL,
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  updated_at = ?
WHERE id = ? AND status IN ('failed', 'claimed', 'compensating') AND ` + leaseWhere
		jobArgs := []any{compensatingStepPrefix + "%", compensatingStepPrefix + "desire", nowStr, jobID}
		if leaseArg != nil {
			jobArgs = append(jobArgs, leaseArg)
		}
		_, err = tx.ExecContext(ctx, jobUpdate, jobArgs...)
		if err != nil {
			return false, err
		}

		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	})
}
