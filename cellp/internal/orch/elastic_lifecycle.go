package orch

import (
	"context"

	"github.com/cellp/cellp/internal/registry"
)

// elasticQualificationEnabled is true when deploy should enter deploy_ready before qualification (AD-15).
func (o *Orchestrator) elasticQualificationEnabled(ctx context.Context, projectID, versionID string) bool {
	pol, err := o.store.GetServingPolicy(ctx, projectID, versionID)
	if err != nil || pol == nil {
		return false
	}
	return pol.ElasticEnrolled
}

// maybeEnterDeployReady transitions to deploy_ready when elastic qualification applies.
func (o *Orchestrator) maybeEnterDeployReady(ctx context.Context, j *registry.Job) error {
	if !o.elasticQualificationEnabled(ctx, j.ProjectID, j.VersionID) {
		return nil
	}
	if err := o.assertDeployOperation(ctx, j); err != nil {
		return err
	}
	return o.setStatus(ctx, j, registry.StatusDeployReady)
}
