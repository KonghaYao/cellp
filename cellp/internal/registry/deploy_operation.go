package registry

import (
	"context"
	"errors"
	"time"
)

// ErrDeployOperationConflict is returned when another deploy job holds the version lease.
var ErrDeployOperationConflict = errors.New("deploy_operation_conflict")

// ErrDeployOperationNotOwner is returned when a deploy write is not owned by the active job.
var ErrDeployOperationNotOwner = errors.New("deploy_operation_not_owner")

// DeployCompensationWork is a failed elastic deploy that may still own qualification desire.
type DeployCompensationWork struct {
	ProjectID string
	VersionID string
	JobID     string
}

// DeployAttempt identifies one worker claim generation for a job (job id + monotonic epoch).
type DeployAttempt struct {
	JobID      string
	ClaimEpoch int64
}

// JobDeployAttempt returns the deploy fence for a claimed job row.
func JobDeployAttempt(j *Job) DeployAttempt {
	if j == nil {
		return DeployAttempt{}
	}
	return DeployAttempt{JobID: j.ID, ClaimEpoch: j.ClaimEpoch}
}

// DeployOperationStore fences concurrent deploy jobs per version.
type DeployOperationStore interface {
	ClaimVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, lease time.Duration) error
	RenewVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, lease time.Duration) error
	AssertVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt) error
	ReleaseVersionDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt) error
	ListDeployQualificationCompensationWork(ctx context.Context) ([]DeployCompensationWork, error)
	ListLegacyDeployQualificationRecoveryCandidates(ctx context.Context) ([]DeployCompensationWork, error)
	RecoverLegacyDeployQualificationIfUnique(ctx context.Context, projectID, versionID string) (recovered bool, err error)
	PrepareDiscoveredCompensationJob(ctx context.Context, jobID string) (prepared bool, err error)

	UpdateVersionStatusForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, status string, errMsg *string) error
	SetRouteForDeployOperation(ctx context.Context, attempt DeployAttempt, route Route) error
	SetRouteActiveForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, active bool) error
	CompareAndSetDesiredForDeployOperation(ctx context.Context, projectID, versionID string, attempt DeployAttempt, expectGen int64, desire ServingDesireRow) error
}
