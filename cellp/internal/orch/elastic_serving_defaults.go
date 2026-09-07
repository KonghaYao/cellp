package orch

import (
	"context"

	"github.com/cellp/cellp/internal/config"
	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
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
func (o *Orchestrator) ensureDefaultElasticServingPolicy(ctx context.Context, projectID, versionID string) error {
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
