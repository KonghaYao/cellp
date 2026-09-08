package registry

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const deployQualificationReasonPrefix = "deploy_qualification:"
const deployQualificationReasonLegacyExact = "deploy_qualification"

func deployHolderJobBlocksTakeover(ctx context.Context, tx *sql.Tx, holderJobID string) (bool, error) {
	var status, step string
	err := tx.QueryRowContext(ctx, `SELECT status, step FROM jobs WHERE id = ?`, holderJobID).Scan(&status, &step)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status == "compensating" {
		return true, nil
	}
	if status == "claimed" && strings.HasPrefix(step, "compensating:") {
		return true, nil
	}
	return false, nil
}

func jobHasActiveWorkerClaim(ctx context.Context, tx *sql.Tx, jobID string, now time.Time) (bool, error) {
	var status, step string
	var leaseUntil sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT status, step, lease_until FROM jobs WHERE id = ?`, jobID).Scan(&status, &step, &leaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != "claimed" {
		return false, nil
	}
	return jobWorkerLeaseIsLive(leaseUntil, now), nil
}

func versionDeployOperationActiveForJob(ctx context.Context, tx *sql.Tx, projectID, versionID, jobID string, now time.Time) (bool, error) {
	var holder sql.NullString
	var holderUntil sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT deploy_operation_job_id, deploy_operation_lease_until
FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&holder, &holderUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !holder.Valid || strings.TrimSpace(holder.String) != jobID {
		return false, nil
	}
	if holderUntil.Valid && holderUntil.String != "" {
		t, parseErr := time.Parse(time.RFC3339Nano, holderUntil.String)
		if parseErr == nil && t.After(now) {
			return true, nil
		}
	}
	return false, nil
}

func (s *SQLiteStore) ClaimVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, lease time.Duration) error {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if projectID == "" || versionID == "" || attempt.JobID == "" || lease <= 0 {
		return errors.New("invalid deploy operation claim")
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := time.Now().UTC()
		nowStr := now.Format(time.RFC3339Nano)
		until := now.Add(lease).Format(time.RFC3339Nano)

		var jobEpoch int64
		var jobStatus string
		err = tx.QueryRowContext(ctx, `SELECT claim_epoch, status FROM jobs WHERE id = ?`, attempt.JobID).Scan(&jobEpoch, &jobStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("job not found")
		}
		if err != nil {
			return err
		}
		if jobEpoch != attempt.ClaimEpoch {
			return ErrDeployOperationNotOwner
		}
		if jobStatus != "claimed" && jobStatus != "compensating" {
			return ErrDeployOperationConflict
		}

		var holder sql.NullString
		var holderUntil sql.NullString
		err = tx.QueryRowContext(ctx, `
SELECT deploy_operation_job_id, deploy_operation_lease_until
FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&holder, &holderUntil)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("version not found")
			}
			return err
		}
		var otherComp int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM jobs
WHERE project_id = ? AND version_id = ? AND id <> ?
  AND (status = 'compensating' OR (status = 'claimed' AND step LIKE 'compensating:%'))`,
			projectID, versionID, attempt.JobID).Scan(&otherComp); err != nil {
			return err
		}
		if otherComp > 0 {
			return ErrDeployOperationConflict
		}
		if holder.Valid && strings.TrimSpace(holder.String) != "" {
			current := strings.TrimSpace(holder.String)
			if current != attempt.JobID {
				active := false
				if holderUntil.Valid && holderUntil.String != "" {
					t, parseErr := time.Parse(time.RFC3339Nano, holderUntil.String)
					if parseErr == nil && t.After(now) {
						active = true
					}
				}
				blocks, berr := deployHolderJobBlocksTakeover(ctx, tx, current)
				if berr != nil {
					return berr
				}
				if blocks {
					return ErrDeployOperationConflict
				}
				var jobStatus string
				jerr := tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?`, current).Scan(&jobStatus)
				takeover := jerr == nil && (jobStatus == "failed" || jobStatus == "completed")
				if active && !takeover {
					return ErrDeployOperationConflict
				}
				if !active && !takeover {
					return ErrDeployOperationConflict
				}
			}
		}
		res, err := tx.ExecContext(ctx, `
UPDATE versions SET deploy_operation_job_id = ?, deploy_operation_claim_epoch = ?,
  deploy_operation_lease_until = ?, updated_at = ?
WHERE project_id = ? AND id = ?`, attempt.JobID, attempt.ClaimEpoch, until, nowStr, projectID, versionID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return errors.New("version not found")
		}
		return tx.Commit()
	})
}

func (s *SQLiteStore) RenewVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, lease time.Duration) error {
	return s.claimOrRenewDeployOperation(ctx, projectID, versionID, attempt, lease, true)
}

func (s *SQLiteStore) AssertVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt) error {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if projectID == "" || versionID == "" || attempt.JobID == "" {
		return ErrDeployOperationNotOwner
	}
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := deployOperationAssertInTx(ctx, tx, projectID, versionID, attempt, time.Now().UTC()); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func (s *SQLiteStore) ReleaseVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt) error {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if projectID == "" || versionID == "" || attempt.JobID == "" {
		return nil
	}
	return withRetryErr(func() error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx, `
UPDATE versions SET deploy_operation_job_id = NULL, deploy_operation_claim_epoch = 0,
  deploy_operation_lease_until = NULL, updated_at = ?
WHERE project_id = ? AND id = ? AND deploy_operation_job_id = ? AND deploy_operation_claim_epoch = ?`,
			now, projectID, versionID, attempt.JobID, attempt.ClaimEpoch)
		return err
	})
}

func (s *SQLiteStore) ListDeployQualificationCompensationWork(ctx context.Context) ([]DeployCompensationWork, error) {
	return withRetry(func() ([]DeployCompensationWork, error) {
		now := time.Now().UTC()
		qualReason := deployQualificationReasonPrefix + "%"
		rows, err := s.db.QueryContext(ctx, `
SELECT project_id, version_id, job_id, status, lease_until FROM (
  SELECT j.project_id, j.version_id, j.id AS job_id, j.status, j.lease_until
  FROM jobs j
  INNER JOIN serving_desires d ON d.project_id = j.project_id AND d.version_id = j.version_id
    AND d.reason = ? || j.id
  WHERE j.status = 'compensating'
      OR (j.status = 'claimed' AND j.step LIKE ?)
  UNION
  SELECT d.project_id, d.version_id, SUBSTR(d.reason, ?) AS job_id, j.status, j.lease_until
  FROM serving_desires d
  INNER JOIN versions v ON v.project_id = d.project_id AND v.id = d.version_id
  INNER JOIN jobs j ON j.id = SUBSTR(d.reason, ?)
    AND j.project_id = d.project_id AND j.version_id = d.version_id
  WHERE v.status = ? AND d.reason LIKE ?
)`,
			deployQualificationReasonPrefix, compensatingStepPrefix+"%",
			len(deployQualificationReasonPrefix)+1, len(deployQualificationReasonPrefix)+1,
			StatusFailed, qualReason)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		seen := make(map[string]struct{})
		var out []DeployCompensationWork
		for rows.Next() {
			var projectID, versionID, jobID, status string
			var leaseUntil sql.NullString
			if err := rows.Scan(&projectID, &versionID, &jobID, &status, &leaseUntil); err != nil {
				return nil, err
			}
			jobID = strings.TrimSpace(jobID)
			if jobID == "" {
				continue
			}
			if status == "claimed" && !jobWorkerLeaseNeedsCompensationRepair(leaseUntil, now) {
				continue
			}
			if _, ok := seen[jobID]; ok {
				continue
			}
			seen[jobID] = struct{}{}
			out = append(out, DeployCompensationWork{ProjectID: projectID, VersionID: versionID, JobID: jobID})
		}
		return out, rows.Err()
	})
}

func (s *SQLiteStore) PrepareDiscoveredCompensationJob(ctx context.Context, jobID string) (bool, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
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
		active, err := jobHasActiveWorkerClaim(ctx, tx, jobID, now)
		if err != nil {
			return false, err
		}
		if active {
			return false, nil
		}
		var status string
		err = tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?`, jobID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if status == "completed" {
			return false, nil
		}
		res, err := tx.ExecContext(ctx, `
UPDATE jobs SET status = 'compensating', lease_until = NULL, claimed_worker_id = NULL,
  step = CASE WHEN step LIKE ? THEN step ELSE ? END,
  updated_at = ?
	WHERE id = ? AND status IN ('failed', 'compensating')`,
			compensatingStepPrefix+"%", compensatingStepPrefix+"desire", nowStr, jobID)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return false, nil
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *SQLiteStore) claimOrRenewDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, lease time.Duration, renewOnly bool) error {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	attempt.JobID = strings.TrimSpace(attempt.JobID)
	if projectID == "" || versionID == "" || attempt.JobID == "" || lease <= 0 {
		return errors.New("invalid deploy operation renew")
	}
	_ = renewOnly
	return withRetryErr(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := time.Now().UTC()
		if err := deployOperationAssertInTx(ctx, tx, projectID, versionID, attempt, now); err != nil {
			return err
		}
		until := now.Add(lease).Format(time.RFC3339Nano)
		nowStr := now.Format(time.RFC3339Nano)
		res, err := tx.ExecContext(ctx, `
UPDATE versions SET deploy_operation_lease_until = ?, updated_at = ?
WHERE project_id = ? AND id = ? AND deploy_operation_job_id = ? AND deploy_operation_claim_epoch = ?`,
			until, nowStr, projectID, versionID, attempt.JobID, attempt.ClaimEpoch)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrDeployOperationNotOwner
		}
		return tx.Commit()
	})
}
