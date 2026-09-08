package activator

import (
	"context"
	"errors"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

var (
	// ErrActivationNotEligible means the version is not an enrolled cold-activation target.
	ErrActivationNotEligible = errors.New("activator: version not eligible")
	// ErrActivationNotQualified means the version never completed deploy qualification.
	ErrActivationNotQualified = errors.New("activator: version not qualified")
	// ErrActivationCapacityUnavailable means policy forbids even one replica.
	ErrActivationCapacityUnavailable = errors.New("activator: capacity unavailable")
	// ErrActivationGuardLost means this process may no longer write desired state.
	ErrActivationGuardLost = errors.New("activator: controller guard lost")
)

// EnsureCapacityClient is the control-plane hook for idempotent desired>=min bumps.
type EnsureCapacityClient interface {
	EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error
}

// WriteGuard verifies singleton controller ownership before a desired-state write.
type WriteGuard interface {
	HoldsWriteLock(ctx context.Context) error
}

// EnsureStore is the registry surface used by cold activation.
type EnsureStore interface {
	GetVersion(ctx context.Context, projectID, versionID string) (*registry.Version, error)
	GetServingPolicy(ctx context.Context, projectID, versionID string) (*registry.ServingPolicyRow, error)
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
	EnsureActivationDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire registry.ServingDesireRow, minReplicas int) error
}

// RegistryEnsureClient bumps serving_desires through generation CAS after
// revalidating status, enrollment, policy bounds, and singleton ownership.
type RegistryEnsureClient struct {
	Store EnsureStore
	Guard WriteGuard
}

// EnsureCapacity sets desired_replicas to at least minReplicas using generation CAS.
func (c *RegistryEnsureClient) EnsureCapacity(ctx context.Context, projectID, versionID string, minReplicas int) error {
	if c == nil || c.Store == nil || c.Guard == nil {
		return ErrActivationGuardLost
	}
	if minReplicas < 1 {
		minReplicas = 1
	}
	const maxAttempts = 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := c.guard(ctx); err != nil {
			return err
		}
		version, err := c.Store.GetVersion(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if version == nil || !activationCapacityStatusEligible(version.Status) {
			return ErrActivationNotEligible
		}
		if version.ReadyAt == nil {
			return ErrActivationNotQualified
		}
		policy, err := c.Store.GetServingPolicy(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if policy == nil || !policy.ElasticEnrolled {
			return ErrActivationNotEligible
		}
		if err := contract.ValidateServingPolicy(contract.ServingPolicy{
			Revision: policy.Revision, MinReplicas: policy.MinReplicas, MaxReplicas: policy.MaxReplicas,
			Priority: policy.Priority, BackgroundMode: policy.BackgroundMode, ElasticEnrolled: policy.ElasticEnrolled,
		}); err != nil {
			return ErrActivationNotEligible
		}
		if policy.MaxReplicas < minReplicas {
			return ErrActivationCapacityUnavailable
		}
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
		if err := c.guard(ctx); err != nil {
			return err
		}
		err = c.Store.EnsureActivationDesired(ctx, projectID, versionID, expectGen, row, minReplicas)
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		if errors.Is(err, registry.ErrActivationNotEligible) {
			return ErrActivationNotEligible
		}
		if errors.Is(err, registry.ErrServingCapacityUnavailable) {
			return ErrActivationCapacityUnavailable
		}
		return err
	}
	return registry.ErrDesiredCASConflict
}

func activationCapacityStatusEligible(status string) bool {
	return status == contract.StatusDeployReady || status == contract.StatusReady
}

func (c *RegistryEnsureClient) guard(ctx context.Context) error {
	if err := c.Guard.HoldsWriteLock(ctx); err != nil {
		return errors.Join(ErrActivationGuardLost, err)
	}
	return nil
}
