package orch

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

// ErrDeployCompensationIncomplete is returned when elastic deploy compensation could not finish.
var ErrDeployCompensationIncomplete = errors.New("deploy_compensation_incomplete")

// ErrDeployCompensationSuperseded is returned when compensation no longer owns deploy side effects.
var ErrDeployCompensationSuperseded = errors.New("deploy_compensation_superseded")

const (
	compStepDesire  = "compensating:desire"
	compStepRoute  = "compensating:route"
	compStepBranch = "compensating:branch"
	compStepRelease = "compensating:release"
)

// branchDestroyForCompensation is overridden in tests to record Destroy calls.
var branchDestroyForCompensation = func(o *Orchestrator, ctx context.Context, projectID, versionID string) error {
	if o.branch == nil {
		return nil
	}
	return o.branch.Destroy(ctx, projectID, versionID)
}

func (o *Orchestrator) releaseCompensatingWorkerAttempt(ctx context.Context, workerID string, j *registry.Job) {
	if j == nil {
		return
	}
	if err := o.store.MarkJobCompensatingForAttempt(ctx, workerID, deployAttempt(j)); err != nil {
		log.Printf("orch: compensating job %s release to compensating failed: %v", j.ID, err)
	}
}

type deployCompensationResult struct {
	complete bool
	err      error
}

func normalizeCompStep(step string) string {
	step = strings.TrimSpace(step)
	if strings.HasPrefix(step, "compensating:") {
		return step
	}
	return compStepDesire
}

func (o *Orchestrator) holdsDeployOperation(ctx context.Context, j *registry.Job) bool {
	if j == nil {
		return false
	}
	return o.store.AssertVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j)) == nil
}

func (o *Orchestrator) deployOpHolderIndicatesNewOwner(ctx context.Context, holder registry.VersionDeployOperationHolder, selfJobID string) (bool, error) {
	if holder.JobID == "" || holder.JobID == selfJobID {
		return false, nil
	}
	if holder.LeaseActive {
		return true, nil
	}
	holderJob, err := o.store.GetJob(ctx, holder.JobID)
	if err != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if holderJob == nil {
		return false, nil
	}
	if holderJob.Status == "claimed" && holderJob.LeaseUntil != nil && holderJob.LeaseUntil.After(time.Now().UTC()) {
		return true, nil
	}
	return false, nil
}

// compensationResourceSuperseded is true when another deploy job/attempt owns the version deploy operation.
// Third-party serving_desires.reason (activator/autoscaler) does not transfer deploy resource ownership.
func (o *Orchestrator) compensationResourceSuperseded(ctx context.Context, j *registry.Job) (superseded bool, err error) {
	if j == nil {
		return true, nil
	}
	holder, herr := o.store.GetVersionDeployOperationHolder(ctx, j.ProjectID, j.VersionID)
	if herr != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if holder.JobID != "" && holder.JobID != j.ID {
		if holder.LeaseActive {
			return true, nil
		}
		newOwner, oerr := o.deployOpHolderIndicatesNewOwner(ctx, holder, j.ID)
		if oerr != nil {
			return false, oerr
		}
		if newOwner {
			return true, nil
		}
	}
	if o.holdsDeployOperation(ctx, j) {
		return false, nil
	}
	cur, derr := o.store.GetServingDesire(ctx, j.ProjectID, j.VersionID)
	if derr != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if cur != nil && deployQualificationReasonAmbiguous(cur.Reason) {
		return false, ErrDeployCompensationIncomplete
	}
	if cur != nil && deployQualificationReasonOwnedByJob(cur.Reason, j.ID) {
		return false, ErrDeployCompensationIncomplete
	}
	return false, ErrDeployCompensationIncomplete
}

// compensationSuperseded reports deploy resource ownership transfer (alias for resource supersession).
func (o *Orchestrator) compensationSuperseded(ctx context.Context, j *registry.Job) (superseded bool, err error) {
	return o.compensationResourceSuperseded(ctx, j)
}

func (o *Orchestrator) shouldRollbackQualificationDesireOnCompensation(ctx context.Context, j *registry.Job) (rollback bool, err error) {
	if j == nil {
		return false, ErrDeployCompensationIncomplete
	}
	cur, derr := o.store.GetServingDesire(ctx, j.ProjectID, j.VersionID)
	if derr != nil {
		return false, ErrDeployCompensationIncomplete
	}
	if cur == nil {
		return false, nil
	}
	if deployQualificationReasonAmbiguous(cur.Reason) {
		return false, ErrDeployCompensationIncomplete
	}
	if servingDesireIndicatesNewOwner(cur, j.ID) {
		return false, nil
	}
	return deployQualificationReasonOwnedByJob(cur.Reason, j.ID), nil
}

func (o *Orchestrator) teardownPreviewIngressForDeployCompensation(ctx context.Context, j *registry.Job, reason string) error {
	if j == nil {
		return ErrDeployCompensationIncomplete
	}
	if err := o.assertDeployOperation(ctx, j); err != nil {
		if deployOpSideEffectIncomplete(err) {
			return ErrDeployCompensationIncomplete
		}
		return err
	}
	o.teardownPreviewIngress(ctx, j.ProjectID, j.VersionID, reason)
	if err := o.assertDeployOperation(ctx, j); err != nil {
		if deployOpSideEffectIncomplete(err) {
			return ErrDeployCompensationIncomplete
		}
		return err
	}
	return nil
}

func deployOpSideEffectIncomplete(err error) bool {
	return errors.Is(err, registry.ErrDeployOperationNotOwner)
}

func (o *Orchestrator) safeTerminalCompensationJob(ctx context.Context, j *registry.Job) {
	if j == nil {
		return
	}
	if err := o.store.FailJobForAttempt(ctx, deployAttempt(j)); err != nil {
		log.Printf("orch: superseded compensation job %s not terminalized: %v", j.ID, err)
	}
}

func (o *Orchestrator) advanceDeployCompensationStep(ctx context.Context, j *registry.Job, step string) (nextStep string, done bool, err error) {
	if j == nil {
		return "", false, ErrDeployCompensationIncomplete
	}
	step = normalizeCompStep(step)
	elasticQ := o.elasticQualificationEnabled(ctx, j.ProjectID, j.VersionID)

	switch step {
	case compStepDesire:
		if superseded, serr := o.compensationResourceSuperseded(ctx, j); serr != nil {
			return step, false, serr
		} else if superseded {
			return "", true, ErrDeployCompensationSuperseded
		}
		if err := o.assertDeployOperation(ctx, j); err != nil {
			if deployOpSideEffectIncomplete(err) {
				return step, false, ErrDeployCompensationIncomplete
			}
			return step, false, err
		}
		if err := o.teardownPreviewIngressForDeployCompensation(ctx, j, "deploy_failed"); err != nil {
			return step, false, err
		}
		if elasticQ {
			rollback, rerr := o.shouldRollbackQualificationDesireOnCompensation(ctx, j)
			if rerr != nil {
				return step, false, rerr
			}
			if rollback {
				if err := o.rollbackElasticQualificationDesire(ctx, j.ProjectID, j.VersionID, deployAttempt(j)); err != nil {
					if errors.Is(err, ErrQualificationDesireRollbackIncomplete) {
						return step, false, err
					}
					return step, false, err
				}
			}
		}
		return compStepRoute, false, nil

	case compStepRoute:
		if superseded, serr := o.compensationResourceSuperseded(ctx, j); serr != nil {
			return step, false, serr
		} else if superseded {
			return "", true, ErrDeployCompensationSuperseded
		}
		if err := o.store.SetRouteActiveForDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), false); err != nil {
			if deployOpSideEffectIncomplete(err) {
				return step, false, ErrDeployCompensationIncomplete
			}
			return step, false, err
		}
		return compStepBranch, false, nil

	case compStepBranch:
		if superseded, serr := o.compensationResourceSuperseded(ctx, j); serr != nil {
			return step, false, serr
		} else if superseded {
			return "", true, ErrDeployCompensationSuperseded
		}
		if err := o.assertDeployOperation(ctx, j); err != nil {
			if deployOpSideEffectIncomplete(err) {
				return step, false, ErrDeployCompensationIncomplete
			}
			return step, false, err
		}
		if _, verr := o.store.GetVersion(ctx, j.ProjectID, j.VersionID); verr != nil {
			return compStepBranch, false, ErrDeployCompensationIncomplete
		}
		if o.branch == nil {
			return compStepRelease, false, nil
		}
		if err := branchDestroyForCompensation(o, ctx, j.ProjectID, j.VersionID); err != nil {
			return compStepBranch, false, ErrDeployCompensationIncomplete
		}
		if err := o.assertDeployOperation(ctx, j); err != nil {
			if deployOpSideEffectIncomplete(err) {
				return compStepBranch, false, ErrDeployCompensationIncomplete
			}
			return compStepBranch, false, err
		}
		return compStepRelease, false, nil

	case compStepRelease:
		if superseded, serr := o.compensationResourceSuperseded(ctx, j); serr != nil {
			return step, false, serr
		} else if superseded {
			return "", true, ErrDeployCompensationSuperseded
		}
		if !o.holdsDeployOperation(ctx, j) {
			return step, false, ErrDeployCompensationIncomplete
		}
		if err := o.store.ReleaseVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j)); err != nil {
			if deployOpSideEffectIncomplete(err) {
				return step, false, ErrDeployCompensationIncomplete
			}
			return step, false, err
		}
		if err := o.store.FailJobForAttempt(ctx, deployAttempt(j)); err != nil {
			return compStepRelease, false, ErrDeployCompensationIncomplete
		}
		return "", true, nil

	default:
		return compStepDesire, false, nil
	}
}

func (o *Orchestrator) persistCompensationStep(ctx context.Context, workerID string, j *registry.Job, step string) error {
	if j == nil || strings.TrimSpace(step) == "" {
		return nil
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || j.Status != "claimed" {
		return registry.ErrJobLeaseConflict
	}
	return o.store.UpdateJobStepForAttempt(ctx, workerID, deployAttempt(j), step)
}

func (o *Orchestrator) runDeployCompensationStateMachine(ctx context.Context, workerID string, j *registry.Job, step string, maxSteps int) (done bool, err error) {
	if j == nil {
		return false, ErrDeployCompensationIncomplete
	}
	cur := normalizeCompStep(step)
	for i := 0; i < maxSteps; i++ {
		next, finished, aerr := o.advanceDeployCompensationStep(ctx, j, cur)
		if finished {
			if errors.Is(aerr, ErrDeployCompensationSuperseded) {
				o.safeTerminalCompensationJob(ctx, j)
				return true, nil
			}
			return true, aerr
		}
		if aerr != nil {
			_ = o.persistCompensationStep(ctx, workerID, j, cur)
			return false, aerr
		}
		cur = next
		if err := o.persistCompensationStep(ctx, workerID, j, cur); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (o *Orchestrator) compensateDeploy(ctx context.Context, workerID string, j *registry.Job) deployCompensationResult {
	if j == nil {
		return deployCompensationResult{complete: false, err: ErrDeployCompensationIncomplete}
	}
	_ = o.persistCompensationStep(ctx, workerID, j, compStepDesire)
	done, err := o.runDeployCompensationStateMachine(ctx, workerID, j, compStepDesire, 16)
	if err != nil {
		if errors.Is(err, ErrDeployCompensationIncomplete) || errors.Is(err, ErrQualificationDesireRollbackIncomplete) {
			log.Printf("orch: deploy compensation incomplete for %s/%s job=%s", j.ProjectID, j.VersionID, j.ID)
			return deployCompensationResult{complete: false, err: err}
		}
		log.Printf("orch: deploy compensation error for %s/%s job=%s: %v", j.ProjectID, j.VersionID, j.ID, err)
		return deployCompensationResult{complete: false, err: err}
	}
	if !done {
		return deployCompensationResult{complete: false, err: ErrDeployCompensationIncomplete}
	}
	return deployCompensationResult{complete: true}
}

func (o *Orchestrator) processCompensationOne(ctx context.Context, workerID string) {
	j, err := o.store.ClaimCompensatingJob(ctx, workerID, jobLease)
	if err != nil || j == nil {
		o.reconcileDiscoveredDeployCompensation(ctx)
		return
	}
	if err := o.store.ClaimVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), jobLease); err != nil {
		log.Printf("orch: compensating job %s deploy op claim: %v", j.ID, err)
		o.releaseCompensatingWorkerAttempt(ctx, workerID, j)
		return
	}
	_ = o.withCompensatingJobLeases(ctx, j, workerID, jobLease, func(lctx context.Context) error {
		done, serr := o.runDeployCompensationStateMachine(lctx, workerID, j, j.Step, 8)
		if serr != nil {
			log.Printf("orch: compensating job %s retry: %v", j.ID, serr)
			o.releaseCompensatingWorkerAttempt(ctx, workerID, j)
			return nil
		}
		if !done {
			o.releaseCompensatingWorkerAttempt(ctx, workerID, j)
		}
		return nil
	})
}

func (o *Orchestrator) reconcileDiscoveredDeployCompensation(ctx context.Context) {
	legacy, err := o.store.ListLegacyDeployQualificationRecoveryCandidates(ctx)
	if err != nil {
		log.Printf("orch: list legacy deploy qualification recovery: %v", err)
	} else {
		for _, item := range legacy {
			recovered, rerr := o.store.RecoverLegacyDeployQualificationIfUnique(ctx, item.ProjectID, item.VersionID)
			if rerr != nil {
				log.Printf("orch: legacy deploy qualification recovery %s/%s: %v", item.ProjectID, item.VersionID, rerr)
				continue
			}
			if recovered {
				log.Printf("orch: legacy deploy qualification normalized %s/%s", item.ProjectID, item.VersionID)
			}
		}
	}
	work, err := o.store.ListDeployQualificationCompensationWork(ctx)
	if err != nil {
		log.Printf("orch: list deploy compensation work: %v", err)
		return
	}
	for _, item := range work {
		repaired, rerr := o.store.RepairClaimedJobLeaseInvariantForCompensation(ctx, item.JobID)
		if rerr != nil {
			if errors.Is(rerr, registry.ErrJobClaimedLeaseInvariant) {
				log.Printf("orch: compensation job=%s claimed lease invariant blocked (deploy op active)", item.JobID)
				continue
			}
			log.Printf("orch: repair claimed lease invariant job=%s: %v", item.JobID, rerr)
			continue
		}
		if repaired {
			log.Printf("orch: repaired claimed lease invariant job=%s %s/%s", item.JobID, item.ProjectID, item.VersionID)
		}
		prepared, perr := o.store.PrepareDiscoveredCompensationJob(ctx, item.JobID)
		if perr != nil {
			log.Printf("orch: prepare discovered compensation job=%s: %v", item.JobID, perr)
		}
		if !prepared {
			continue
		}
		log.Printf("orch: discovered compensation queued job=%s %s/%s", item.JobID, item.ProjectID, item.VersionID)
	}
}
