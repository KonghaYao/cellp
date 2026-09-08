package orch

import (
	"context"
	"errors"
	"log"

	"github.com/cellp/cellp/internal/registry"
)

func (o *Orchestrator) finalizeDeployJobFailure(ctx context.Context, workerID string, j *registry.Job, runErr error) {
	if j == nil {
		return
	}
	attempt := deployAttempt(j)
	holdsOp := o.holdsDeployOperation(ctx, j)
	if !holdsOp {
		o.abortDeployJobWithoutOp(ctx, workerID, j, runErr)
		return
	}
	ctx, stopLease := o.startDeployWorkerLeaseKeeper(ctx, j, workerID, jobLease)
	defer stopLease()
	msg := runErr.Error()
	if uerr := o.store.UpdateVersionStatusForDeployOperation(ctx, j.ProjectID, j.VersionID, attempt, registry.StatusFailed, &msg); uerr != nil && !errors.Is(uerr, registry.ErrDeployOperationNotOwner) {
		log.Printf("orch: job %s failed status update: %v", j.ID, uerr)
	}
	if assertErr := o.assertDeployOperation(ctx, j); assertErr != nil {
		o.abortDeployJobWithoutOp(ctx, workerID, j, runErr)
		return
	}
	comp := o.compensateDeploy(ctx, workerID, j)
	if comp.complete {
		if err := o.store.FailJobForAttempt(ctx, attempt); err != nil {
			rec, _ := o.store.GetJob(ctx, j.ID)
			if rec != nil && rec.Status == "failed" {
				return
			}
			if merr := o.store.MarkJobCompensatingForAttempt(ctx, workerID, attempt); merr != nil {
				log.Printf("orch: job %s compensating mark after fail-for-attempt: %v; keeping claimed until lease expiry for discovery", j.ID, merr)
			}
		}
		return
	}
	if err := o.store.MarkJobCompensatingForAttempt(ctx, workerID, attempt); err != nil {
		log.Printf("orch: job %s compensating handoff (exact attempt) failed: %v; keeping claimed until lease expiry for discovery", j.ID, err)
	}
}

func (o *Orchestrator) abortDeployJobWithoutOp(ctx context.Context, workerID string, j *registry.Job, runErr error) {
	attempt := deployAttempt(j)
	log.Printf("orch: job %s failure without deploy op lease: %v", j.ID, runErr)
	needs, nerr := o.deployFailureNeedsCompensation(ctx, j)
	if nerr != nil {
		log.Printf("orch: job %s compensation need uncertain: %v", j.ID, nerr)
		o.handoffDeployJobToCompensating(ctx, workerID, j, attempt, runErr)
		return
	}
	if !needs {
		if err := o.store.FailJobForAttempt(ctx, attempt); err != nil {
			_ = o.store.ReleaseClaimedJobToPending(ctx, workerID, attempt)
		}
		return
	}
	o.handoffDeployJobToCompensating(ctx, workerID, j, attempt, runErr)
}

func (o *Orchestrator) handoffDeployJobToCompensating(ctx context.Context, workerID string, j *registry.Job, attempt registry.DeployAttempt, runErr error) {
	if j == nil {
		return
	}
	msg := runErr.Error()
	statusErr := o.store.UpdateVersionStatusForDeployOperation(ctx, j.ProjectID, j.VersionID, attempt, registry.StatusFailed, &msg)
	if statusErr != nil && !errors.Is(statusErr, registry.ErrDeployOperationNotOwner) {
		log.Printf("orch: job %s failed status update (no op): %v", j.ID, statusErr)
	}
	if err := o.store.MarkJobCompensatingForAttempt(ctx, workerID, attempt); err != nil {
		log.Printf("orch: job %s compensating handoff (exact attempt) failed: %v; keeping claimed until lease expiry for discovery", j.ID, err)
	}
}

func (o *Orchestrator) deployFailureNeedsCompensation(ctx context.Context, j *registry.Job) (bool, error) {
	if j == nil {
		return false, nil
	}
	v, verr := o.store.GetVersion(ctx, j.ProjectID, j.VersionID)
	if verr != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if v == nil {
		return false, nil
	}
	if v.Status == registry.StatusFailed {
		d, derr := o.store.GetServingDesire(ctx, j.ProjectID, j.VersionID)
		if derr != nil {
			return false, ErrDeployCompensationIncomplete
		}
		if d != nil && deployQualificationReasonOwnedByJob(d.Reason, j.ID) {
			return true, nil
		}
	}
	holder, herr := o.store.GetVersionDeployOperationHolder(ctx, j.ProjectID, j.VersionID)
	if herr != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if holder.JobID == j.ID {
		return true, nil
	}
	return false, nil
}
