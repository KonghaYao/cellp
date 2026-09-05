package orch

import (
	"context"
	"fmt"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

// validatePromoteTarget applies AD-15 promote gates when elastic runtime is enabled.
func (o *Orchestrator) validatePromoteTarget(ctx context.Context, projectID, versionID string, v *registry.Version) error {
	if !contract.ElasticRuntimeEnabled() {
		return nil
	}
	if !contract.PromoteEligible(v.Status) {
		return fmt.Errorf("version not ready: %s", v.Status)
	}
	snap, err := o.store.BuildLegacyRouteSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("route snapshot: %w", err)
	}
	for _, es := range snap.EndpointSets {
		if es.ProjectID == projectID && es.VersionID == versionID && len(es.Endpoints) > 0 {
			return nil
		}
	}
	return fmt.Errorf("version not promote eligible: no routable endpoint in snapshot")
}

func (o *Orchestrator) commitProdPromote(ctx context.Context, projectID, oldProd, versionID string) error {
	if contract.ElasticRuntimeEnabled() {
		_, err := o.store.CommitProdPromote(ctx, projectID, oldProd, versionID)
		return err
	}
	if err := o.store.SetProdVersionCAS(ctx, projectID, oldProd, versionID); err != nil {
		return err
	}
	return o.store.SetRouteActive(ctx, projectID, versionID, true)
}
