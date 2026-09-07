package orch

// Elastic enrolled deploy qualification uses Scheduler→Agent desired state (AD-15 / SURGE §16.1).
// docs/decisions.md §20 does not approve orchestrator-local runtime.Start for enrolled paths.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

const (
	desireReasonDeployQualificationPrefix = "deploy_qualification:"
	desireReasonDeployQualificationLegacy = "deploy_qualification"
	desireReasonDeployFailed              = "deploy_failed"
	desireReasonIdle                      = "idle"
	desireReasonCronResident              = "cron_resident"
	desireReasonPromoteActivate           = "promote_activate"
	defaultQualificationWait              = 120 * time.Second
	qualificationDesireCASAttempts        = 8
)

// ErrQualificationDesireRollbackIncomplete is returned when deploy compensation cannot
// clear an owned qualification desire after bounded CAS retries.
var ErrQualificationDesireRollbackIncomplete = errors.New("qualification desire rollback incomplete")

// ErrQualificationDesireCASExhausted is returned when qualification desire CAS retries are exhausted.
var ErrQualificationDesireCASExhausted = errors.New("qualification_desire_cas_exhausted")

type servingDesireCAS interface {
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
	CompareAndSetDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire registry.ServingDesireRow) error
}

type ElasticSchedulerTick func(ctx context.Context) error

// SetElasticSchedulerTick wires scheduler ticks for elastic deploy qualification (serve.Run).
func (o *Orchestrator) SetElasticSchedulerTick(tick ElasticSchedulerTick) {
	o.elasticSchedulerTick = tick
}

func deployQualificationReasonForJob(jobID string) string {
	return desireReasonDeployQualificationPrefix + strings.TrimSpace(jobID)
}

func deployQualificationReasonOwnedByJob(reason, jobID string) bool {
	want := deployQualificationReasonForJob(jobID)
	return reason == want
}

// deployQualificationReasonAmbiguous is true for legacy or malformed qualification reasons
// that do not identify a single owning deploy job.
func deployQualificationReasonAmbiguous(reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" || reason == desireReasonDeployQualificationLegacy {
		return true
	}
	if strings.HasPrefix(reason, desireReasonDeployQualificationPrefix) {
		owner := strings.TrimSpace(strings.TrimPrefix(reason, desireReasonDeployQualificationPrefix))
		return owner == ""
	}
	return false
}

func servingDesireIndicatesNewOwner(cur *registry.ServingDesireRow, jobID string) bool {
	if cur == nil {
		return false
	}
	reason := strings.TrimSpace(cur.Reason)
	if reason == "" || reason == desireReasonDeployFailed {
		return false
	}
	if deployQualificationReasonAmbiguous(reason) {
		return false
	}
	if deployQualificationReasonOwnedByJob(reason, jobID) {
		return false
	}
	if strings.HasPrefix(reason, desireReasonDeployQualificationPrefix) {
		owner := strings.TrimSpace(strings.TrimPrefix(reason, desireReasonDeployQualificationPrefix))
		return owner != "" && owner != jobID
	}
	return true
}

func clampDeployQualificationDesired(policy *registry.ServingPolicyRow, cur *registry.ServingDesireRow) (int, error) {
	if policy == nil {
		return 0, fmt.Errorf("qualification desire: serving policy unavailable")
	}
	if policy.MaxReplicas < 1 {
		return 0, fmt.Errorf("qualification desire: max replicas unavailable")
	}
	desired := 1
	if cur != nil && cur.DesiredReplicas > desired {
		desired = cur.DesiredReplicas
	}
	if policy.MinReplicas > desired {
		desired = policy.MinReplicas
	}
	if desired < 1 {
		desired = 1
	}
	if desired > policy.MaxReplicas {
		desired = policy.MaxReplicas
	}
	if desired < 1 {
		return 0, fmt.Errorf("qualification desire: policy cannot satisfy qualification")
	}
	return desired, nil
}

func (o *Orchestrator) ensureDeployQualificationDesire(ctx context.Context, j *registry.Job) error {
	if j == nil || strings.TrimSpace(j.ID) == "" {
		return fmt.Errorf("qualification desire: missing deploy job")
	}
	if err := o.assertDeployOperation(ctx, j); err != nil {
		return err
	}
	reasonWant := deployQualificationReasonForJob(j.ID)
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		policy, err := o.store.GetServingPolicy(ctx, j.ProjectID, j.VersionID)
		if err != nil {
			return err
		}
		if policy == nil {
			return fmt.Errorf("qualification desire: serving policy unavailable")
		}
		cur, err := o.store.GetServingDesire(ctx, j.ProjectID, j.VersionID)
		if err != nil {
			return err
		}
		desired, err := clampDeployQualificationDesired(policy, cur)
		if err != nil {
			return err
		}
		if cur != nil && deployQualificationReasonOwnedByJob(cur.Reason, j.ID) && cur.DesiredReplicas == desired {
			return nil
		}
		if cur != nil && deployQualificationReasonAmbiguous(cur.Reason) {
			return ErrDeployCompensationIncomplete
		}
		expectGen := int64(0)
		nextGen := int64(1)
		if cur != nil {
			expectGen = cur.Generation
			nextGen = cur.Generation + 1
		}
		err = o.store.CompareAndSetDesiredForDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), expectGen, registry.ServingDesireRow{
			ProjectID:       j.ProjectID,
			VersionID:       j.VersionID,
			DesiredReplicas: desired,
			Generation:      nextGen,
			Reason:          reasonWant,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDeployOperationNotOwner) {
			return err
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return ErrQualificationDesireCASExhausted
}

func (o *Orchestrator) waitElasticQualificationEndpoint(ctx context.Context, projectID, versionID string) (string, int, error) {
	if o.elasticSchedulerTick == nil {
		return "", 0, fmt.Errorf("elastic scheduler tick not configured")
	}
	deadline := time.Now().Add(defaultQualificationWait)
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		if err := o.elasticSchedulerTick(ctx); err != nil {
			return "", 0, err
		}
		host, port, ok, err := qualificationEndpointFromStore(ctx, o.store, projectID, versionID)
		if err != nil {
			return "", 0, err
		}
		if ok {
			return host, port, nil
		}
		if time.Now().After(deadline) {
			return "", 0, fmt.Errorf("qualification endpoint timeout for %s/%s", projectID, versionID)
		}
		if err := sleepUntil(ctx, 10*time.Millisecond); err != nil {
			return "", 0, err
		}
	}
}

func qualificationEndpointFromStore(ctx context.Context, store registry.Store, projectID, versionID string) (host string, port int, ok bool, err error) {
	view, found, err := store.BuildQualificationViewAfter(ctx, -1)
	if err != nil || !found {
		return "", 0, false, err
	}
	now := time.Now().UTC()
	for _, set := range view.EndpointSets {
		if set.ProjectID != projectID || set.VersionID != versionID {
			continue
		}
		for _, endpoint := range set.Endpoints {
			if endpoint.State != contract.EndpointReady || endpoint.ValidUntil == nil || !endpoint.ValidUntil.After(now) {
				continue
			}
			host, portRaw, splitErr := net.SplitHostPort(endpoint.Address)
			if splitErr != nil {
				return "", 0, false, fmt.Errorf("qualification endpoint address corrupt: %w", splitErr)
			}
			p, convErr := strconv.Atoi(portRaw)
			if convErr != nil || host == "" || p <= 0 {
				return "", 0, false, fmt.Errorf("qualification endpoint address corrupt")
			}
			return host, p, true, nil
		}
	}
	return "", 0, false, nil
}

func (o *Orchestrator) waitQualificationHealth(ctx context.Context, host string, port int) error {
	deadline := time.Now().Add(defaultQualificationWait)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if o.runtime.Health(ctx, host, port) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("qualification health check failed")
		}
		if err := sleepUntil(ctx, 50*time.Millisecond); err != nil {
			return err
		}
	}
}

func sleepUntil(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func rollbackElasticQualificationDesireOnStore(ctx context.Context, store servingDesireCAS, projectID, versionID, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("qualification rollback: missing deploy job")
	}
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		cur, err := store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if cur == nil || !deployQualificationReasonOwnedByJob(cur.Reason, jobID) {
			return nil
		}
		nextGen := cur.Generation + 1
		err = store.CompareAndSetDesired(ctx, projectID, versionID, cur.Generation, registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: 0,
			Generation:      nextGen,
			Reason:          desireReasonDeployFailed,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return ErrQualificationDesireRollbackIncomplete
}

func commitQualificationIdleDesireOnStore(ctx context.Context, store servingDesireCAS, projectID, versionID, jobID string, minReplicas int, background contract.BackgroundMode) error {
	if minReplicas > 0 || background == contract.BackgroundModeResidentRequired {
		return nil
	}
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("qualification idle: missing deploy job")
	}
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		cur, err := store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if cur == nil || !deployQualificationReasonOwnedByJob(cur.Reason, jobID) {
			return nil
		}
		nextGen := cur.Generation + 1
		err = store.CompareAndSetDesired(ctx, projectID, versionID, cur.Generation, registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: 0,
			Generation:      nextGen,
			Reason:          desireReasonIdle,
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

// finalizeQualificationIdleDesire drops desired replicas to zero after successful deploy when policy allows scale-to-zero.
func (o *Orchestrator) finalizeQualificationIdleDesire(ctx context.Context, j *registry.Job) error {
	if j == nil {
		return nil
	}
	if !o.effectiveServingDefaults().IdleAfterDeploy {
		return nil
	}
	if err := o.assertDeployOperation(ctx, j); err != nil {
		return err
	}
	policy, err := o.store.GetServingPolicy(ctx, j.ProjectID, j.VersionID)
	if err != nil {
		return err
	}
	if policy == nil {
		return nil
	}
	if policy.MinReplicas > 0 || policy.BackgroundMode == contract.BackgroundModeResidentRequired {
		return nil
	}
	if !o.effectiveServingDefaults().ScaleToZeroEnabled {
		return nil
	}
	for attempt := 0; attempt < qualificationDesireCASAttempts; attempt++ {
		cur, err := o.store.GetServingDesire(ctx, j.ProjectID, j.VersionID)
		if err != nil {
			return err
		}
		if cur == nil || !deployQualificationReasonOwnedByJob(cur.Reason, j.ID) {
			return nil
		}
		nextGen := cur.Generation + 1
		err = o.store.CompareAndSetDesiredForDeployOperation(ctx, j.ProjectID, j.VersionID, deployAttempt(j), cur.Generation, registry.ServingDesireRow{
			ProjectID:       j.ProjectID,
			VersionID:       j.VersionID,
			DesiredReplicas: 0,
			Generation:      nextGen,
			Reason:          desireReasonIdle,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDeployOperationNotOwner) {
			return nil
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return ErrQualificationDesireCASExhausted
}

func (o *Orchestrator) rollbackElasticQualificationDesire(ctx context.Context, projectID, versionID string, attempt registry.DeployAttempt) error {
	if strings.TrimSpace(attempt.JobID) == "" {
		return fmt.Errorf("qualification rollback: missing deploy job")
	}
	for attemptNum := 0; attemptNum < qualificationDesireCASAttempts; attemptNum++ {
		cur, err := o.store.GetServingDesire(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if cur == nil || !deployQualificationReasonOwnedByJob(cur.Reason, attempt.JobID) {
			return nil
		}
		nextGen := cur.Generation + 1
		err = o.store.CompareAndSetDesiredForDeployOperation(ctx, projectID, versionID, attempt, cur.Generation, registry.ServingDesireRow{
			ProjectID:       projectID,
			VersionID:       versionID,
			DesiredReplicas: 0,
			Generation:      nextGen,
			Reason:          desireReasonDeployFailed,
		})
		if err == nil {
			return nil
		}
		if errors.Is(err, registry.ErrDeployOperationNotOwner) {
			return ErrQualificationDesireRollbackIncomplete
		}
		if errors.Is(err, registry.ErrDesiredCASConflict) {
			continue
		}
		return err
	}
	return ErrQualificationDesireRollbackIncomplete
}
