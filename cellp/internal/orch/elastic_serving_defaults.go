package orch

import (
	"context"
	"errors"
	"strings"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
	"github.com/cellp/cellp/internal/runtime"
)

func (o *Orchestrator) effectiveServingDefaults() config.ServingDefaults {
	if o == nil {
		return config.LoadServingDefaults()
	}
	d := o.cfg.Serving
	if d.DefaultMaxReplicas < 1 || d.DefaultBackground == "" {
		return config.LoadServingDefaults()
	}
	return d
}

// ensureDefaultElasticServingPolicy enrolls a version with platform serving defaults when no policy exists.
func (o *Orchestrator) ensureDefaultElasticServingPolicy(ctx context.Context, projectID, versionID, bundleDir string, armCron bool) error {
	if o == nil || o.store == nil {
		return nil
	}
	pol, err := o.store.GetServingPolicy(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	if pol != nil {
		return nil
	}
	def := o.effectiveServingDefaults()
	bg := def.DefaultBackground
	if bg == contract.BackgroundModeUnknown {
		bg = contract.BackgroundModeNone
	}
	min := def.EffectiveDefaultMinReplicas()
	max := def.DefaultMaxReplicas
	if max < min {
		max = min
	}
	if armCron && bundleHasCrons(bundleDir) {
		bg = contract.BackgroundModeResidentRequired
		min = 1
		max = 1
	}
	row := registry.ServingPolicyRow{
		ProjectID:       projectID,
		VersionID:       versionID,
		Revision:        1,
		MinReplicas:     min,
		MaxReplicas:     max,
		Priority:        0,
		BackgroundMode:  bg,
		ElasticEnrolled: true,
	}
	return o.store.UpsertServingPolicy(ctx, row)
}

func bundleHasCrons(bundleDir string) bool {
	bundleDir = strings.TrimSpace(bundleDir)
	if bundleDir == "" {
		return false
	}
	b, err := runtime.ParseBindings(bundleDir)
	if err != nil {
		return false
	}
	return len(b.Crons) > 0
}

// ensurePromotedProdActivation raises desired replicas for the new prod version so cutover can serve immediately.
func (o *Orchestrator) ensurePromotedProdActivation(ctx context.Context, projectID, versionID string) error {
	if o == nil || o.store == nil {
		return nil
	}
	pol, err := o.store.GetServingPolicy(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	if pol == nil || !pol.ElasticEnrolled {
		return nil
	}
	target := pol.MinReplicas
	if target < 1 {
		target = 1
	}
	if pol.MaxReplicas > 0 && pol.MaxReplicas < target {
		target = pol.MaxReplicas
	}
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		cur, err := o.store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if cur != nil && cur.DesiredReplicas >= target {
			break
		}
		expectGen := int64(0)
		nextGen := int64(1)
		if cur != nil {
			expectGen = cur.Generation
			nextGen = cur.Generation + 1
		}
		err = o.store.CompareAndSetDesired(ctx, projectID, versionID, expectGen, registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: target,
			Generation:      nextGen,
			Reason:          desireReasonPromoteActivate,
		})
		if err == nil {
			break
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	if o.elasticSchedulerTick != nil {
		if err := o.elasticSchedulerTick(ctx); err != nil {
			return err
		}
	}
	return nil
}
