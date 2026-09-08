package registry

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// VersionDeployOperationHolder is the current deploy-operation lease on a version (read-only).
type VersionDeployOperationHolder struct {
	JobID       string
	ClaimEpoch  int64
	LeaseActive bool
}

// GetVersionDeployOperationHolder returns the deploy operation holder for a version.
func (s *SQLiteStore) GetVersionDeployOperationHolder(ctx context.Context, projectID, versionID string) (VersionDeployOperationHolder, error) {
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	if projectID == "" || versionID == "" {
		return VersionDeployOperationHolder{}, errors.New("invalid version")
	}
	var out VersionDeployOperationHolder
	err := withRetryErr(func() error {
		var holder sql.NullString
		var holderUntil sql.NullString
		var holderEpoch int64
		err := s.db.QueryRowContext(ctx, `
SELECT deploy_operation_job_id, deploy_operation_lease_until, deploy_operation_claim_epoch
FROM versions WHERE project_id = ? AND id = ?`, projectID, versionID).Scan(&holder, &holderUntil, &holderEpoch)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("version not found")
			}
			return err
		}
		if holder.Valid {
			out.JobID = strings.TrimSpace(holder.String)
		}
		out.ClaimEpoch = holderEpoch
		out.LeaseActive = false
		if holderUntil.Valid && holderUntil.String != "" {
			t, parseErr := time.Parse(time.RFC3339Nano, holderUntil.String)
			if parseErr == nil && t.After(time.Now().UTC()) {
				out.LeaseActive = true
			}
		}
		return nil
	})
	if err != nil {
		return VersionDeployOperationHolder{}, err
	}
	return out, nil
}
