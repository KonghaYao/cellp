package orch

import (
	"context"
	"errors"
	"fmt"

	"github.com/cellp/cellp/internal/registry"
)

func deployAttempt(j *registry.Job) registry.DeployAttempt {
	if j == nil {
		return registry.DeployAttempt{}
	}
	return registry.DeployAttempt{JobID: j.ID, ClaimEpoch: j.ClaimEpoch}
}

// ErrDeployVersionBusy is returned when the version deploy operation is held by another job.
var ErrDeployVersionBusy = errors.New("deploy_version_busy")

func (o *Orchestrator) claimDeployOperation(ctx context.Context, j *registry.Job) error {
	if j == nil || j.ID == "" {
		return fmt.Errorf("deploy operation: missing job")
	}
	if err := o.store.ClaimVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), jobLease); err != nil {
		if errors.Is(err, registry.ErrDeployOperationConflict) {
			return ErrDeployVersionBusy
		}
		return err
	}
	return nil
}

func (o *Orchestrator) assertDeployOperation(ctx context.Context, j *registry.Job) error {
	if j == nil || j.ID == "" {
		return registry.ErrDeployOperationNotOwner
	}
	if err := o.store.AssertVersionDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j)); err != nil {
		return err
	}
	return nil
}

func (o *Orchestrator) setRouteForDeploy(ctx context.Context, j *registry.Job, route registry.Route) error {
	if j == nil || j.ID == "" {
		return registry.ErrDeployOperationNotOwner
	}
	return o.store.SetRouteForDeployOperation(ctx, deployAttempt(j), route)
}

func (o *Orchestrator) setRouteActiveForDeploy(ctx context.Context, j *registry.Job, active bool) error {
	if j == nil || j.ID == "" {
		return registry.ErrDeployOperationNotOwner
	}
	return o.store.SetRouteActiveForDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), active)
}

func (o *Orchestrator) setStatusForDeploy(ctx context.Context, j *registry.Job, status string) error {
	if j == nil || j.ID == "" {
		return registry.ErrDeployOperationNotOwner
	}
	if err := o.store.UpdateVersionStatusForDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), status, nil); err != nil {
		return err
	}
	return o.store.UpdateJobStep(ctx, j.ID, status)
}
