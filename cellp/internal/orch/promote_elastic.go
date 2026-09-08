package orch

import (
	"context"
	"fmt"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// validatePromoteTarget applies AD-15 promote gates when elastic runtime is enabled.
func (o *Orchestrator) validatePromoteTarget(ctx context.Context, projectID, versionID string, v *registry.Version) error {
	if !contract.PromoteEligible(v.Status) {
		return fmt.Errorf("version not ready: %s", v.Status)
	}
	ok, err := o.promoteRoutableInSnapshot(ctx, projectID, versionID)
	if err != nil {
		return fmt.Errorf("route snapshot: %w", err)
	}
	if ok {
		return nil
	}
	// Qualified ready versions may be cold (scale-to-zero) with no live replica in the public snapshot yet.
	if v.ReadyAt != nil && o.elasticQualificationEnabled(ctx, projectID, versionID) {
		if err := o.ensurePromotePreflightCapacity(ctx, projectID, versionID); err != nil {
			return fmt.Errorf("promote preflight: %w", err)
		}
		if err := o.waitPromoteRoutableSnapshot(ctx, projectID, versionID); err != nil {
			return fmt.Errorf("version not promote eligible: %w", err)
		}
		return nil
	}
	return fmt.Errorf("version not promote eligible: no routable endpoint in snapshot")
}

func (o *Orchestrator) promoteRoutableInSnapshot(ctx context.Context, projectID, versionID string) (bool, error) {
	snap, err := o.store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		return false, err
	}
	return snapshotHasRoutableEndpoint(snap, projectID, versionID), nil
}

func snapshotHasRoutableEndpoint(snap contract.RouteSnapshot, projectID, versionID string) bool {
	for _, es := range snap.EndpointSets {
		if es.ProjectID == projectID && es.VersionID == versionID && len(es.Endpoints) > 0 {
			return true
		}
	}
	return false
}

func (o *Orchestrator) ensurePromotePreflightCapacity(ctx context.Context, projectID, versionID string) error {
	proj, err := o.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	bundleDir, err := versionBundleDir(o.cfg, projectID, versionID)
	if err != nil {
		return err
	}
	armCron := CronShouldArm(proj, versionID)
	if err := o.ensureCronResidentDesire(ctx, projectID, versionID, armCron, bundleDir); err != nil {
		return err
	}
	return o.ensurePromotedProdActivation(ctx, projectID, versionID)
}

func (o *Orchestrator) waitPromoteRoutableSnapshot(ctx context.Context, projectID, versionID string) error {
	deadline := time.Now().Add(defaultQualificationWait)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if o.elasticSchedulerTick != nil {
			if err := o.elasticSchedulerTick(ctx); err != nil {
				return err
			}
		}
		ok, err := o.promoteRoutableInSnapshot(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no routable endpoint in snapshot after preflight for %s/%s", projectID, versionID)
		}
		if err := sleepUntil(ctx, 50*time.Millisecond); err != nil {
			return err
		}
	}
}

func (o *Orchestrator) commitProdPromote(ctx context.Context, projectID, oldProd, versionID string) error {
	_, err := o.store.CommitProdPromote(ctx, projectID, oldProd, versionID)
	return err
}
