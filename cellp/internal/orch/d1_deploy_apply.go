package orch

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cellp/cellp/internal/registry"
)

// applyD1DeployPlan runs celld d1 import or branch against a version bucket.
// Requires a running celld fleet (node leases in the bucket); call after elastic
// qualification health when deploy is elastic-enrolled.
func (o *Orchestrator) applyD1DeployPlan(ctx context.Context, j *registry.Job, d1Plan D1DeployPlan, bundleDir, seedPath string) error {
	if err := o.assertDeployOperation(ctx, j); err != nil {
		return err
	}
	if d1Plan.UseBranch {
		t0 := time.Now()
		if err := o.runtime.D1Branch(ctx, j.ProjectID, j.VersionID, d1Plan.ParentID, bundleDir); err != nil {
			if deployFailClosed() {
				return fmt.Errorf("d1 branch: %w", err)
			}
			log.Printf("orch: d1 branch warn after %s: %v", time.Since(t0), err)
		} else {
			log.Printf("orch: d1 branch took %s", time.Since(t0))
		}
		return nil
	}
	if _, err := os.Stat(seedPath); err != nil {
		return nil
	}
	t0 := time.Now()
	if err := o.runtime.D1Execute(ctx, j.ProjectID, j.VersionID, bundleDir, seedPath); err != nil {
		if deployFailClosed() {
			return fmt.Errorf("d1 seed: %w", err)
		}
		log.Printf("orch: d1 seed warn after %s: %v", time.Since(t0), err)
	} else {
		log.Printf("orch: d1 seed took %s", time.Since(t0))
	}
	return nil
}
