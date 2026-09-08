package orch

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
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

// ReconcileCronAfterProdChange redeploys fleet manifests so only the new prod arms cron triggers.
func (o *Orchestrator) ReconcileCronAfterProdChange(ctx context.Context, projectID, oldProd, newProd string) error {
	proj, err := o.store.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return fmt.Errorf("project not found")
	}
	for _, vid := range []string{oldProd, newProd} {
		if vid == "" {
			continue
		}
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
		if err := o.ensureCronServingPolicy(ctx, projectID, vid, arm, bundleDir); err != nil {
			return fmt.Errorf("cron reconcile policy %s: %w", vid, err)
		}
		if err := o.ensureCronResidentDesire(ctx, projectID, vid, arm, bundleDir); err != nil {
			return fmt.Errorf("cron reconcile desire %s: %w", vid, err)
		}
		if err := o.runtime.Deploy(ctx, projectID, vid, bundleDir, arm); err != nil {
			return fmt.Errorf("cron reconcile deploy %s: %w", vid, err)
		}
		if err := o.runtime.Restart(ctx, projectID, vid); err != nil {
			return fmt.Errorf("cron reconcile restart %s: %w", vid, err)
		}
		log.Printf("orch: cron reconcile %s/%s arm=%v", projectID, vid, arm)
	}
	return nil
}

func (o *Orchestrator) enqueueCronReconcileAfterProdChange(projectID, oldProd, newProd string) {
	if o == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := o.ReconcileCronAfterProdChange(ctx, projectID, oldProd, newProd); err != nil {
			log.Printf("orch: cron reconcile after promote warn: %v", err)
		}
	}()
}

func (o *Orchestrator) ensureCronServingPolicy(ctx context.Context, projectID, versionID string, armCron bool, bundleDir string) error {
	if o == nil || o.store == nil {
		return nil
	}
	if !armCron || !bundleHasCrons(bundleDir) {
		return nil
	}
	pol, err := o.store.GetServingPolicy(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	if pol == nil {
		return o.ensureDefaultElasticServingPolicy(ctx, projectID, versionID, bundleDir, true)
	}
	if pol.BackgroundMode == contract.BackgroundModeResidentRequired && pol.MinReplicas >= 1 {
		return nil
	}
	row := *pol
	row.Revision++
	row.BackgroundMode = contract.BackgroundModeResidentRequired
	if row.MinReplicas < 1 {
		row.MinReplicas = 1
	}
	row.MaxReplicas = 1
	return o.store.UpsertServingPolicy(ctx, row)
}

func (o *Orchestrator) ensureCronResidentDesire(ctx context.Context, projectID, versionID string, armCron bool, bundleDir string) error {
	if o == nil || o.store == nil || !armCron || !bundleHasCrons(bundleDir) {
		return nil
	}
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		cur, err := o.store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		expectGen := int64(0)
		nextGen := int64(1)
		if cur != nil {
			if cur.DesiredReplicas >= 1 {
				return nil
			}
			expectGen = cur.Generation
			nextGen = cur.Generation + 1
		}
		err = o.store.CompareAndSetDesired(ctx, projectID, versionID, expectGen, registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: 1,
			Generation:      nextGen,
			Reason:          desireReasonCronResident,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return ErrQualificationDesireCASExhausted
}

func versionBundleDir(cfg config.Config, projectID, versionID string) (string, error) {
	destDir := filepath.Join(cfg.ArtifactsDir, projectID, versionID)
	bundleDir := filepath.Join("dev", "examples", "counter")
	if _, err := os.Stat(filepath.Join(destDir, "wrangler.jsonc")); err == nil {
		bundleDir = destDir
	} else if _, err := os.Stat(filepath.Join(destDir, "wrangler.json")); err == nil {
		bundleDir = destDir
	} else if alt := filepath.Join(cfg.ArtifactsDir, "..", "examples", "counter"); alt != bundleDir {
		if _, err := os.Stat(filepath.Join(alt, "wrangler.jsonc")); err == nil {
			bundleDir = alt
		}
	}
	abs, err := filepath.Abs(bundleDir)
	if err != nil {
		return "", err
	}
	return abs, nil
}
