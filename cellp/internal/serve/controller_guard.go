package serve

import (
	"context"
	"fmt"

	"github.com/cellp/cellp/internal/registry"
)

// controllerGuardRequired is true when the singleton cellpd writer guard must be acquired.
func controllerGuardRequired() bool {
	return true
}

func requireElasticControllerGuard(enabled, acquired bool) error {
	if enabled && !acquired {
		return fmt.Errorf("elastic runtime requires singleton controller guard")
	}
	return nil
}

// releaseControllerGuardIfQuiesced releases the singleton writer guard only after a fully quiesced shutdown.
func releaseControllerGuardIfQuiesced(ctx context.Context, store registry.Store, guardID string, acquired, quiesced bool) error {
	if !acquired || !quiesced || store == nil || guardID == "" {
		return nil
	}
	return store.ReleaseControllerGuard(ctx, guardID)
}
