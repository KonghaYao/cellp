package activator

import (
	"context"
	"errors"

	"github.com/cellp/cellp/internal/registry"
)

// EnsureCapacityClient is the control-plane hook for idempotent desired>=min bumps.
type EnsureCapacityClient interface {
	EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error
}

// RegistryEnsureClient bumps serving_desires via CAS (stub until WP-API server exists).
type RegistryEnsureClient struct {
	Store registry.ServingStore
}

// EnsureCapacity sets desired_replicas to at least minReplicas using generation CAS.
func (c *RegistryEnsureClient) EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error {
	if c == nil || c.Store == nil {
		return errors.New("activator: nil registry client")
	}
	if minReplicas < 1 {
		minReplicas = 1
	}
	const maxAttempts = 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		cur, err := c.Store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if cur != nil && cur.DesiredReplicas >= minReplicas {
			return nil
		}
		expectGen := int64(0)
		nextGen := int64(1)
		if cur != nil {
			expectGen = cur.Generation
			nextGen = cur.Generation + 1
		}
		row := registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: minReplicas,
			Generation:      nextGen,
			Reason:          "activator_ensure",
		}
		err = c.Store.CompareAndSetDesired(ctx, projectID, versionID, expectGen, row)
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return registry.ErrDesiredCASConflict
}
