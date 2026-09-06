package orch

import (
	"context"
	"fmt"
	"log"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

// CronShouldArm reports whether celld should receive triggers.crons on deploy for this version.
// When the project has no prod yet, the deploying version may arm (first ready / pre-CAS deploy).
func CronShouldArm(proj *registry.Project, versionID string) bool {
	if proj == nil {
		return true
	}
	if proj.ProdVersionID == nil || *proj.ProdVersionID == "" {
		return true
	}
	return *proj.ProdVersionID == versionID
}

// shouldSkipInactiveDisarmCronReconcile reports whether cron disarm redeploy can be
// skipped after promote drain: inactive route and unreachable upstream (AD-11).
func shouldSkipInactiveDisarmCronReconcile(arm, routeActive, upstreamHealthy bool) bool {
	return !arm && !routeActive && !upstreamHealthy
}

// ReconcileCronAfterProdChange redeploys fleet manifests so only the new prod arms cron triggers.
func (o *Orchestrator) ReconcileCronAfterProdChange(ctx context.Context, projectID, oldProd, newProd string) error {
	proj, err := o.store.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return fmt.Errorf("project not found")
	}
	// Arm new prod before disarming old prod so a drained predecessor cannot block cutover.
	reconcileOrder := make([]string, 0, 2)
	if newProd != "" {
		reconcileOrder = append(reconcileOrder, newProd)
	}
	if oldProd != "" && oldProd != newProd {
		reconcileOrder = append(reconcileOrder, oldProd)
	}
	for _, vid := range reconcileOrder {
		v, err := o.store.GetVersion(ctx, projectID, vid)
		if err != nil {
			return fmt.Errorf("version %s: %w", vid, err)
		}
		if v == nil {
			return fmt.Errorf("version %s not found", vid)
		}
		if v.Status != registry.StatusReady {
			log.Printf("orch: cron reconcile skip %s/%s status=%s", projectID, vid, v.Status)
			continue
		}
		bundleDir, err := versionBundleDir(o.cfg, projectID, vid)
		if err != nil {
			return err
		}
		arm := CronShouldArm(proj, vid)
		route, err := o.store.GetRoute(ctx, projectID, vid)
		if err != nil {
			return fmt.Errorf("cron reconcile route %s: %w", vid, err)
		}
		if route == nil {
			return fmt.Errorf("cron reconcile route %s: not configured", vid)
		}
		if shouldSkipInactiveDisarmCronReconcile(arm, route.Active, o.runtime.Health(ctx, route.UpstreamHost, route.UpstreamPort)) {
			log.Printf("orch: cron reconcile skip %s/%s disarm: upstream unavailable with inactive route", projectID, vid)
			continue
		}
		if err := o.runtime.DeployForCronReconcile(ctx, projectID, vid, bundleDir, arm, route.UpstreamHost, route.UpstreamPort); err != nil {
			return fmt.Errorf("cron reconcile deploy %s: %w", vid, err)
		}
		if err := o.runtime.Restart(ctx, projectID, vid); err != nil {
			return fmt.Errorf("cron reconcile restart %s: %w", vid, err)
		}
		log.Printf("orch: cron reconcile %s/%s arm=%v", projectID, vid, arm)
	}
	return nil
}

func versionBundleDir(cfg config.Config, projectID, versionID string) (string, error) {
	return runtime.ResolveVersionBundleDir(cfg.ArtifactsDir, projectID, versionID)
}
