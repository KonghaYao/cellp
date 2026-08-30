package orch

import (
	"context"
	"fmt"

	"github.com/cellp/cellp/internal/registry"
)

// ApplyWorkerEnv persists Worker vars and restarts a ready version so celld reloads CELLD_VARS_FILE.
func (o *Orchestrator) ApplyWorkerEnv(ctx context.Context, projectID, versionID string, env map[string]string) error {
	v, err := o.store.GetVersion(ctx, projectID, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if v.Status == registry.StatusDestroyed {
		return fmt.Errorf("version destroyed")
	}
	if err := o.store.SetVersionEnv(ctx, projectID, versionID, env); err != nil {
		return err
	}
	if v.Status != registry.StatusReady {
		return nil
	}
	return o.runtime.Restart(ctx, projectID, versionID)
}
