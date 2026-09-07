package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/elastic/registrywire"
	"github.com/cellp/cellp/internal/registry"
)

// Handler executes transport-neutral Node Agent lifecycle commands.
type Handler struct {
	enabled      bool
	nodes        NodeStore
	reps         ReplicaStore
	store        LifecycleStore
	backend      LifecycleBackend
	now          func() time.Time
	retention    time.Duration
	commandLease time.Duration
	leaseTick    time.Duration
}

// NewHandler preserves the registry scaffold behavior for compatibility tests.
// Production callers must use NewLifecycleHandler.
func NewHandler(enabled bool, nodes NodeStore, reps ReplicaStore) *Handler {
	return &Handler{enabled: enabled, nodes: nodes, reps: reps, now: time.Now, retention: 24 * time.Hour, commandLease: 30 * time.Second, leaseTick: 10 * time.Second}
}

// NewLifecycleHandler builds the fail-closed production lifecycle handler.
func NewLifecycleHandler(enabled bool, nodes NodeStore, store LifecycleStore, backend LifecycleBackend) *Handler {
	return &Handler{
		enabled: enabled, nodes: nodes, store: store, backend: backend,
		now: time.Now, retention: 24 * time.Hour, commandLease: 30 * time.Second, leaseTick: 10 * time.Second,
	}
}

// Enabled reports whether elastic agent commands are active.
func (h *Handler) Enabled() bool { return h != nil && h.enabled }

// ProbeResult is the live observation for a single replica.
type ProbeResult struct {
	ReplicaID  string                `json:"replica_id"`
	State      contract.ReplicaState `json:"state"`
	Generation int64                 `json:"generation"`
}

var safeStoragePathID = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]{0,126}[A-Za-z0-9])?$`)

func validateStorageScope(scope contract.CommandScope) error {
	for name, value := range map[string]string{"project_id": scope.ProjectID, "version_id": scope.VersionID} {
		if !safeStoragePathID.MatchString(value) {
			return fmt.Errorf("%s is not a safe storage identifier", name)
		}
	}
	return nil
}

// StartReplica claims durable idempotency before touching the runtime.
func (h *Handler) StartReplica(ctx context.Context, spec contract.StartReplicaSpec, idempotencyKey string) (contract.RuntimeReplica, error) {
	if err := validateStorageScope(spec.Scope); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if err := h.validateScope(ctx, spec.Scope, contract.ActionStartReplica, true); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if strings.TrimSpace(spec.Bucket) == "" {
		return contract.RuntimeReplica{}, fmt.Errorf("bucket required")
	}
	if h.store == nil || h.backend == nil {
		return h.startCompatibility(ctx, spec)
	}
	expectedBucket, err := h.backend.ExpectedReplicaBucket(spec.Scope)
	if err != nil || spec.Bucket != expectedBucket {
		return contract.RuntimeReplica{}, fmt.Errorf("bucket does not match assignment")
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return contract.RuntimeReplica{}, fmt.Errorf("idempotency key required")
	}
	rep, err := h.assignment(ctx, spec.Scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	claim, err := h.store.ClaimAgentCommand(ctx, commandFor(spec.Scope, idempotencyKey, h.now().UTC().Add(h.retention)))
	if err != nil {
		return contract.RuntimeReplica{}, h.commandStoreError(err)
	}
	if !claim.Claimed {
		return replayCommand(rep, claim.Command)
	}
	command := claim.Command
	leaseCommand := command
	const startReplicaWorkTimeout = 120 * time.Second
	workCtx, cancelWork := context.WithTimeout(context.WithoutCancel(ctx), startReplicaWorkTimeout)
	defer cancelWork()
	renewCtx := context.WithoutCancel(ctx)
	ownershipLost := make(chan struct{})
	var ownershipOnce sync.Once
	loseOwnership := func() { ownershipOnce.Do(func() { close(ownershipLost); cancelWork() }) }
	checkOwnership := func() error {
		select {
		case <-ownershipLost:
			return registry.ErrAgentCommandConflict
		default:
		}
		expiry := h.now().UTC().Add(h.commandLease)
		if err := h.store.RenewAgentCommandLease(ctx, leaseCommand, expiry); err != nil {
			loseOwnership()
			return err
		}
		return nil
	}
	if err := checkOwnership(); err != nil {
		return contract.RuntimeReplica{}, h.commandStoreError(err)
	}
	go func() {
		ticker := time.NewTicker(h.leaseTick)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				if err := h.store.RenewAgentCommandLease(renewCtx, leaseCommand, h.now().UTC().Add(h.commandLease)); err != nil {
					loseOwnership()
					return
				}
			}
		}
	}()

	// A prior attempt may have committed readiness before losing its command lease.
	// Never regress that healthy assignment: verify exact backend identity and finish atomically.
	if rep.State == contract.ReplicaReady {
		live, probeErr := h.backend.Probe(workCtx, spec.Scope)
		if probeErr != nil || !backendMatches(*rep, live) {
			return contract.RuntimeReplica{}, &CommandError{Reason: contract.ReasonColdActivating, Message: "ready replica verification unavailable"}
		}
		if err := checkOwnership(); err != nil {
			return contract.RuntimeReplica{}, h.commandStoreError(err)
		}
		command.Status, command.ResultState = "succeeded", contract.ReplicaReady
		if err := h.store.RecordObservationAndCompleteAgentCommand(ctx, readyObservation(*rep, live.Host, live.Port), command); err != nil {
			return contract.RuntimeReplica{}, h.commandStoreError(err)
		}
		return *rep, nil
	}

	fail := func(reason contract.ReasonCode, cause error, cleanup bool) (contract.RuntimeReplica, error) {
		if err := checkOwnership(); err != nil {
			return contract.RuntimeReplica{}, errors.Join(&CommandError{Reason: contract.ReasonGenerationStale, Message: "command ownership lost"}, err)
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var persistErrs []error
		if cleanup {
			if stopErr := h.backend.Stop(cleanupCtx, spec.Scope); stopErr != nil {
				persistErrs = append(persistErrs, fmt.Errorf("runtime cleanup: %w", stopErr))
			}
		}
		if recordErr := h.record(cleanupCtx, *rep, contract.ReplicaFailed, "", 0); recordErr != nil {
			persistErrs = append(persistErrs, fmt.Errorf("record failed: %w", recordErr))
		}
		command.Status, command.ResultState, command.Reason = "failed", contract.ReplicaFailed, reason
		if completeErr := h.store.CompleteAgentCommand(cleanupCtx, command); completeErr != nil {
			persistErrs = append(persistErrs, fmt.Errorf("complete command: %w", completeErr))
		}
		base := error(&CommandError{Reason: reason, Message: safeLifecycleMessage(cause)})
		joined := []error{base}
		if cause != nil {
			joined = append(joined, cause)
		}
		return contract.RuntimeReplica{}, errors.Join(append(joined, persistErrs...)...)
	}
	if err := h.backend.Diagnose(workCtx, spec); err != nil {
		return fail(contract.ReasonSnapshotUnavailable, err, false)
	}
	if err := checkOwnership(); err != nil {
		return contract.RuntimeReplica{}, h.commandStoreError(err)
	}
	if rep.State == contract.ReplicaPending || rep.State == contract.ReplicaFailed || rep.State == contract.ReplicaStopped {
		if err := h.record(workCtx, *rep, contract.ReplicaStarting, "", 0); err != nil {
			return fail(contract.ReasonGenerationStale, err, false)
		}
	}
	host, port, err := h.backend.Start(workCtx, spec)
	if err != nil {
		return fail(contract.ReasonSnapshotUnavailable, err, true)
	}
	if err := checkOwnership(); err != nil {
		return contract.RuntimeReplica{}, h.commandStoreError(err)
	}
	live, err := h.backend.Probe(workCtx, spec.Scope)
	if err != nil || !live.Healthy || live.Host != host || live.Port != port || !backendIdentityMatches(*rep, live) {
		return fail(contract.ReasonSnapshotUnavailable, err, true)
	}
	if ownershipErr := checkOwnership(); ownershipErr != nil {
		return contract.RuntimeReplica{}, h.commandStoreError(ownershipErr)
	}
	command.Status, command.ResultState = "succeeded", contract.ReplicaReady
	if err := h.store.RecordObservationAndCompleteAgentCommand(ctx, readyObservation(*rep, host, port), command); err != nil {
		return fail(contract.ReasonGenerationStale, err, true)
	}
	rep.State = contract.ReplicaReady
	return *rep, nil
}

func (h *Handler) startCompatibility(ctx context.Context, spec contract.StartReplicaSpec) (contract.RuntimeReplica, error) {
	rep := contract.RuntimeReplica{
		ReplicaID: spec.Scope.ReplicaID, ProjectID: spec.Scope.ProjectID, VersionID: spec.Scope.VersionID,
		NodeID: spec.Scope.NodeID, Generation: spec.Scope.Generation, State: contract.ReplicaStarting,
	}
	if h.reps != nil {
		if err := h.reps.UpsertRuntimeReplica(ctx, rep); err != nil {
			return contract.RuntimeReplica{}, err
		}
	}
	return rep, nil
}

// ProbeReplica verifies assignment fencing and live backend health.
func (h *Handler) ProbeReplica(ctx context.Context, scope contract.CommandScope) (ProbeResult, error) {
	if err := h.validateScope(ctx, scope, contract.ActionProbeReplica, true); err != nil {
		return ProbeResult{}, err
	}
	if h.store == nil || h.backend == nil {
		rep, err := h.compatibilityReplica(ctx, scope)
		if err != nil {
			return ProbeResult{}, err
		}
		return ProbeResult{ReplicaID: rep.ReplicaID, State: rep.State, Generation: rep.Generation}, nil
	}
	rep, err := h.assignment(ctx, scope)
	if err != nil {
		return ProbeResult{}, err
	}
	live, probeErr := h.backend.Probe(ctx, scope)
	if probeErr != nil || !live.Healthy {
		if rep.State == contract.ReplicaStarting || rep.State == contract.ReplicaReady {
			if err := h.record(ctx, *rep, contract.ReplicaFailed, "", 0); err != nil {
				return ProbeResult{}, errors.Join(probeErr, err)
			}
		}
		if probeErr != nil {
			return ProbeResult{}, probeErr
		}
		return ProbeResult{ReplicaID: rep.ReplicaID, State: contract.ReplicaFailed, Generation: rep.Generation}, nil
	}
	return ProbeResult{ReplicaID: rep.ReplicaID, State: rep.State, Generation: rep.Generation}, nil
}

// StopReplica stops the real process and records a terminal observation.
func (h *Handler) StopReplica(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	if err := h.validateScope(ctx, scope, contract.ActionStopReplica, true); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if h.store == nil || h.backend == nil {
		return h.stopCompatibility(ctx, scope)
	}
	rep, err := h.cleanupAssignment(ctx, scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	if rep.State == contract.ReplicaStopped {
		return *rep, nil
	}
	// Probe is advisory and only supplies a fresh endpoint. Durable withdrawal
	// is the authority and must succeed before touching the local process.
	live, probeErr := h.backend.Probe(ctx, scope)
	if probeErr == nil && backendMatches(*rep, live) && rep.State == contract.ReplicaReady {
		if err := h.record(ctx, *rep, contract.ReplicaDraining, live.Host, live.Port); err != nil &&
			!errors.Is(err, registry.ErrLeaseExpired) {
			return contract.RuntimeReplica{}, err
		}
	}
	if err := h.store.WithdrawReplica(ctx, rep.ReplicaID, rep.ProjectID, rep.VersionID, rep.NodeID, rep.Generation); err != nil {
		return contract.RuntimeReplica{}, errors.Join(probeErr, err)
	}
	rep.State = contract.ReplicaDraining
	if err := h.backend.Stop(ctx, scope); err != nil {
		return contract.RuntimeReplica{}, errors.Join(probeErr, err)
	}
	if err := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaStopped
	return *rep, nil
}

// DrainReplica withdraws the endpoint, then stops because celld has no drain RPC.
func (h *Handler) DrainReplica(ctx context.Context, scope contract.CommandScope, deadline time.Time) (contract.RuntimeReplica, error) {
	if err := h.validateScope(ctx, scope, contract.ActionDrainReplica, true); err != nil {
		return contract.RuntimeReplica{}, err
	}
	if h.store == nil || h.backend == nil {
		return h.drainCompatibility(ctx, scope)
	}
	rep, err := h.cleanupAssignment(ctx, scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	if rep.State == contract.ReplicaStopped {
		return *rep, nil
	}
	live, probeErr := h.backend.Probe(ctx, scope)
	if probeErr == nil && backendMatches(*rep, live) && rep.State == contract.ReplicaReady {
		if err := h.record(ctx, *rep, contract.ReplicaDraining, live.Host, live.Port); err != nil &&
			!errors.Is(err, registry.ErrLeaseExpired) {
			return contract.RuntimeReplica{}, err
		}
	}
	if err := h.store.WithdrawReplica(ctx, rep.ReplicaID, rep.ProjectID, rep.VersionID, rep.NodeID, rep.Generation); err != nil {
		return contract.RuntimeReplica{}, errors.Join(probeErr, err)
	}
	rep.State = contract.ReplicaDraining
	if err := h.backend.Drain(ctx, scope, deadline); err != nil {
		return contract.RuntimeReplica{}, errors.Join(probeErr, err)
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.store.TerminalizeReplica(cleanupCtx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaStopped
	return *rep, nil
}

// ListReplicas returns this node's durable assignments reconciled with local inventory.
func (h *Handler) ListReplicas(ctx context.Context, scope contract.CommandScope) ([]contract.RuntimeReplica, error) {
	if err := h.validateScope(ctx, scope, contract.ActionListReplicas, false); err != nil {
		return nil, err
	}
	if h.store == nil || h.backend == nil {
		return h.listCompatibility(ctx, scope)
	}
	facts, err := h.store.ListRuntimeReplicasByNode(ctx, scope.NodeID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.RuntimeReplica, 0, len(facts))
	for _, rep := range facts {
		liveState := rep.State == contract.ReplicaPending || rep.State == contract.ReplicaStarting || rep.State == contract.ReplicaReady || rep.State == contract.ReplicaDraining
		if liveState && rep.ValidUntil != nil && rep.ValidUntil.After(h.now().UTC()) &&
			(scope.ProjectID == "" || rep.ProjectID == scope.ProjectID) && (scope.VersionID == "" || rep.VersionID == scope.VersionID) {
			out = append(out, rep)
		}
	}
	return out, nil
}

// ReconcileNode restores/verifies valid assignments and stops orphan or expired local processes.
func (h *Handler) ReconcileNode(ctx context.Context, nodeID string) error {
	if h.store == nil || h.backend == nil {
		return fmt.Errorf("lifecycle backend and store required")
	}
	node, nodeErr := h.nodes.GetRuntimeNode(ctx, nodeID)
	if nodeErr != nil {
		return fmt.Errorf("runtime node read: %w", nodeErr)
	}
	facts, factsErr := h.store.ListRuntimeReplicasByNode(ctx, nodeID)
	inventory, inventoryErr := h.backend.List(ctx)
	if factsErr != nil || inventoryErr != nil {
		return errors.Join(factsErr, inventoryErr)
	}
	now := h.now().UTC()
	nodeActive := node != nil && !node.Cordoned && node.LeaseExpiry.After(now)
	if !nodeActive {
		var cleanupErrs []error
		for _, rep := range facts {
			if rep.State != contract.ReplicaStopped && rep.State != contract.ReplicaFailed {
				if err := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); err != nil {
					cleanupErrs = append(cleanupErrs, fmt.Errorf("terminalize unavailable-node replica %s: %w", rep.ReplicaID, err))
				}
			}
		}
		for _, item := range inventory {
			if err := h.backend.Stop(ctx, contract.CommandScope{NodeID: nodeID, ProjectID: item.ProjectID, VersionID: item.VersionID, ReplicaID: item.ReplicaID, Generation: 1, Action: contract.ActionStopReplica}); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("stop unavailable-node replica %s: %w", item.ReplicaID, err))
			}
		}
		cleanupErrs = append(cleanupErrs, &CommandError{Reason: contract.ReasonGenerationStale, Message: "node unavailable"})
		return errors.Join(cleanupErrs...)
	}
	local := make(map[string]BackendReplica, len(inventory))
	for _, item := range inventory {
		local[item.ReplicaID] = item
	}
	valid := make(map[string]contract.RuntimeReplica)
	uncertain := make(map[string]struct{})
	var allErrs []error
	for _, rep := range facts {
		stopScope := scopeFor(rep, rep.Generation, contract.ActionStopReplica)
		validated, assignmentErr := h.store.ValidateAgentAssignment(ctx, stopScope, h.now().UTC())
		authoritativelyInvalid := validated == nil && assignmentErr == nil ||
			registrywire.IsAuthoritativeGenerationStale(assignmentErr) || registrywire.IsAuthoritativeLeaseExpired(assignmentErr)
		if assignmentErr != nil && !authoritativelyInvalid {
			uncertain[rep.ReplicaID] = struct{}{}
			allErrs = append(allErrs, fmt.Errorf("validate assignment for replica %s: %w", rep.ReplicaID, assignmentErr))
			continue
		}
		if authoritativelyInvalid {
			if rep.State != contract.ReplicaStopped && rep.State != contract.ReplicaFailed {
				if terminalErr := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); terminalErr != nil {
					allErrs = append(allErrs, fmt.Errorf("terminalize expired replica %s: %w", rep.ReplicaID, terminalErr))
					continue
				}
			}
			if _, ok := local[rep.ReplicaID]; ok {
				if stopErr := h.backend.Stop(ctx, stopScope); stopErr != nil {
					allErrs = append(allErrs, fmt.Errorf("stop expired replica %s: %w", rep.ReplicaID, stopErr))
				}
			}
			continue
		}
		valid[rep.ReplicaID] = rep
		item, running := local[rep.ReplicaID]
		identityHealthy := running && backendMatches(rep, item)
		if identityHealthy && rep.State != contract.ReplicaDraining {
			if err := h.recordReadyIfRunning(ctx, &rep, item.Host, item.Port); err != nil {
				allErrs = append(allErrs, fmt.Errorf("record ready replica %s: %w", rep.ReplicaID, err))
			}
			continue
		}
		if rep.State == contract.ReplicaDraining && !running {
			if err := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped); err != nil {
				allErrs = append(allErrs, fmt.Errorf("terminalize drained replica %s: %w", rep.ReplicaID, err))
			}
			continue
		}
		if (rep.State == contract.ReplicaStopped || rep.State == contract.ReplicaFailed || rep.State == contract.ReplicaDraining) && running {
			if stopErr := h.backend.Stop(ctx, stopScope); stopErr != nil {
				allErrs = append(allErrs, fmt.Errorf("stop terminal replica %s: %w", rep.ReplicaID, stopErr))
				continue
			}
			if rep.State != contract.ReplicaStopped {
				if err := h.record(ctx, rep, contract.ReplicaStopped, "", 0); err != nil {
					allErrs = append(allErrs, fmt.Errorf("terminalize cleaned replica %s: %w", rep.ReplicaID, err))
				}
			}
			continue
		}
		if rep.State == contract.ReplicaReady {
			if err := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); err != nil {
				allErrs = append(allErrs, fmt.Errorf("withdraw stale endpoint %s: %w", rep.ReplicaID, err))
				continue
			}
			rep.State = contract.ReplicaFailed
		}
		if running {
			if stopErr := h.backend.Stop(ctx, stopScope); stopErr != nil {
				allErrs = append(allErrs, fmt.Errorf("stop mismatched replica %s: %w", rep.ReplicaID, stopErr))
				continue
			}
		}
		if rep.State != contract.ReplicaPending && rep.State != contract.ReplicaStarting && rep.State != contract.ReplicaFailed {
			continue
		}
		startScope := scopeFor(rep, rep.Generation, contract.ActionStartReplica)
		spec := contract.StartReplicaSpec{Scope: startScope, Bucket: fmt.Sprintf("s3://cellp-celld/%s/%s", rep.ProjectID, rep.VersionID)}
		if err := h.backend.Diagnose(ctx, spec); err != nil {
			allErrs = append(allErrs, fmt.Errorf("diagnose replica %s: %w", rep.ReplicaID, err))
			continue
		}
		if err := h.recordStartingIfNeeded(ctx, &rep); err != nil {
			allErrs = append(allErrs, fmt.Errorf("record starting replica %s: %w", rep.ReplicaID, err))
			continue
		}
		host, port, startErr := h.backend.Start(ctx, spec)
		if startErr != nil {
			stopErr := h.backend.Stop(ctx, startScope)
			recordErr := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed)
			allErrs = append(allErrs, errors.Join(fmt.Errorf("start replica %s: %w", rep.ReplicaID, startErr), stopErr, recordErr))
			continue
		}
		probe, probeErr := h.backend.Probe(ctx, startScope)
		if probeErr != nil || !probe.Healthy || probe.Host != host || probe.Port != port || !backendIdentityMatches(rep, probe) {
			stopErr := h.backend.Stop(ctx, startScope)
			recordErr := h.store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed)
			probeFail := probeErr
			if probeFail == nil {
				probeFail = errors.New("probe verification failed")
			}
			allErrs = append(allErrs, errors.Join(fmt.Errorf("probe replica %s: %w", rep.ReplicaID, probeFail), stopErr, recordErr))
			continue
		}
		if err := h.recordReadyIfRunning(ctx, &rep, host, port); err != nil {
			allErrs = append(allErrs, fmt.Errorf("record ready replica %s: %w", rep.ReplicaID, err))
		}
	}
	for _, item := range inventory {
		if _, ok := valid[item.ReplicaID]; ok {
			continue
		}
		if _, ok := uncertain[item.ReplicaID]; ok {
			continue
		}
		if err := h.backend.Stop(ctx, contract.CommandScope{
			NodeID: nodeID, ProjectID: item.ProjectID, VersionID: item.VersionID, ReplicaID: item.ReplicaID,
			Generation: node.Generation, LeaseExpiry: node.LeaseExpiry, Action: contract.ActionStopReplica,
		}); err != nil {
			allErrs = append(allErrs, fmt.Errorf("stop orphan replica %s: %w", item.ReplicaID, err))
		}
	}
	return errors.Join(allErrs...)
}

func (h *Handler) validateScope(ctx context.Context, scope contract.CommandScope, action contract.LifecycleAction, requireReplica bool) error {
	if err := h.guard(); err != nil {
		return err
	}
	if err := contract.ValidateCommandScope(scope); err != nil {
		return fmt.Errorf("scope: %w", err)
	}
	if h.store != nil && scope.Action != action {
		return fmt.Errorf("invalid action")
	}
	if requireReplica && strings.TrimSpace(scope.ReplicaID) == "" {
		return fmt.Errorf("replica_id required")
	}
	if action == contract.ActionStopReplica || action == contract.ActionDrainReplica {
		return nil
	}
	return h.checkNode(ctx, scope)
}

func (h *Handler) guard() error {
	if h == nil || !h.enabled {
		return elasticDisabled()
	}
	if h.nodes == nil {
		return fmt.Errorf("node store required")
	}
	return nil
}

func (h *Handler) checkNode(ctx context.Context, scope contract.CommandScope) error {
	now := h.now().UTC()
	if !scope.LeaseExpiry.After(now) {
		return &CommandError{Reason: contract.ReasonGenerationStale, Message: "assignment lease expired"}
	}
	node, err := h.nodes.GetRuntimeNode(ctx, scope.NodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return errNodeNotFound
	}
	if node.Cordoned {
		return errNodeCordoned
	}
	if !node.LeaseExpiry.After(now) {
		return &CommandError{Reason: contract.ReasonGenerationStale, Message: "node lease expired"}
	}
	return nil
}

func (h *Handler) cleanupAssignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	rep, err := h.store.ValidateAgentCleanupAssignment(ctx, scope)
	if err != nil {
		if errors.Is(err, registry.ErrObservationStale) || errors.Is(err, registry.ErrLeaseExpired) {
			return nil, &CommandError{Reason: contract.ReasonGenerationStale, Message: "assignment mismatch"}
		}
		return nil, err
	}
	if rep == nil {
		return nil, errReplicaNotFound
	}
	return rep, nil
}

func (h *Handler) assignment(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	rep, err := h.store.ValidateAgentAssignment(ctx, scope, h.now().UTC())
	if err != nil {
		if errors.Is(err, registry.ErrObservationStale) || errors.Is(err, registry.ErrLeaseExpired) {
			return nil, &CommandError{Reason: contract.ReasonGenerationStale, Message: "assignment mismatch"}
		}
		return nil, err
	}
	if rep == nil {
		return nil, errReplicaNotFound
	}
	return rep, nil
}

func (h *Handler) recordStartingIfNeeded(ctx context.Context, rep *contract.RuntimeReplica) error {
	switch rep.State {
	case contract.ReplicaStarting, contract.ReplicaReady:
		return nil
	case contract.ReplicaPending, contract.ReplicaFailed, contract.ReplicaStopped:
		if err := h.record(ctx, *rep, contract.ReplicaStarting, "", 0); err != nil {
			if errors.Is(err, registry.ErrReplicaTransitionInvalid) {
				return nil
			}
			return err
		}
		rep.State = contract.ReplicaStarting
		return nil
	default:
		return nil
	}
}

func (h *Handler) recordReadyIfRunning(ctx context.Context, rep *contract.RuntimeReplica, host string, port int) error {
	if rep.State == contract.ReplicaReady {
		return h.record(ctx, *rep, contract.ReplicaReady, host, port)
	}
	if err := h.recordStartingIfNeeded(ctx, rep); err != nil {
		return err
	}
	if err := h.record(ctx, *rep, contract.ReplicaReady, host, port); err != nil {
		return err
	}
	rep.State = contract.ReplicaReady
	return nil
}

func (h *Handler) record(ctx context.Context, rep contract.RuntimeReplica, state contract.ReplicaState, host string, port int) error {
	obs := registry.ReplicaObservation{
		ReplicaID: rep.ReplicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
		Generation: rep.Generation, State: state, AssignmentValidUntil: rep.ValidUntil,
	}
	if state == contract.ReplicaReady || state == contract.ReplicaDraining {
		obs.ListenHost, obs.ListenPort = host, port
		until := *rep.ValidUntil
		obs.EndpointValidUntil = &until
		if state == contract.ReplicaDraining {
			obs.EndpointState = contract.EndpointDraining
		} else {
			obs.EndpointState = contract.EndpointReady
		}
	}
	return h.store.RecordObservation(ctx, obs)
}

func backendIdentityMatches(rep contract.RuntimeReplica, live BackendReplica) bool {
	return live.ReplicaID == rep.ReplicaID && live.ProjectID == rep.ProjectID && live.VersionID == rep.VersionID
}

func backendMatches(rep contract.RuntimeReplica, live BackendReplica) bool {
	return live.Healthy && strings.TrimSpace(live.Host) != "" && live.Port > 0 && backendIdentityMatches(rep, live)
}

func readyObservation(rep contract.RuntimeReplica, host string, port int) registry.ReplicaObservation {
	obs := registry.ReplicaObservation{
		ReplicaID: rep.ReplicaID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, NodeID: rep.NodeID,
		Generation: rep.Generation, State: contract.ReplicaReady, ListenHost: host, ListenPort: port,
		EndpointState: contract.EndpointReady, AssignmentValidUntil: rep.ValidUntil,
	}
	if rep.ValidUntil != nil {
		until := *rep.ValidUntil
		obs.EndpointValidUntil = &until
	}
	return obs
}

func commandFor(scope contract.CommandScope, key string, expires time.Time) registry.AgentCommand {
	return registry.AgentCommand{
		IdempotencyKey: key, Action: scope.Action, NodeID: scope.NodeID, ProjectID: scope.ProjectID,
		VersionID: scope.VersionID, ReplicaID: scope.ReplicaID, Generation: scope.Generation, ExpiresAt: expires,
	}
}

func replayCommand(rep *contract.RuntimeReplica, command registry.AgentCommand) (contract.RuntimeReplica, error) {
	if command.Status == "succeeded" {
		out := *rep
		out.State = command.ResultState
		return out, nil
	}
	return contract.RuntimeReplica{}, &CommandError{Reason: command.Reason, Message: "lifecycle command failed"}
}

func (h *Handler) commandStoreError(err error) error {
	switch {
	case errors.Is(err, registry.ErrAgentCommandConflict):
		return &CommandError{Reason: contract.ReasonReplayRejected, Message: "idempotency scope conflict"}
	case errors.Is(err, registry.ErrAgentCommandInProgress):
		return &CommandError{Reason: contract.ReasonColdActivating, Message: "lifecycle command in progress"}
	default:
		return err
	}
}

func safeLifecycleMessage(err error) string {
	if err == nil {
		return "runtime health failed"
	}
	return "runtime lifecycle failed"
}

func scopeFor(rep contract.RuntimeReplica, nodeGeneration int64, action contract.LifecycleAction) contract.CommandScope {
	lease := time.Time{}
	if rep.ValidUntil != nil {
		lease = *rep.ValidUntil
	}
	return contract.CommandScope{
		NodeID: rep.NodeID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, ReplicaID: rep.ReplicaID,
		Generation: nodeGeneration, LeaseExpiry: lease, Nonce: "reconcile:" + rep.ReplicaID, Action: action,
	}
}

func (h *Handler) compatibilityReplica(ctx context.Context, scope contract.CommandScope) (*contract.RuntimeReplica, error) {
	if h.reps == nil {
		return nil, errReplicaNotFound
	}
	reps, err := h.reps.ListRuntimeReplicas(ctx, scope.ProjectID, scope.VersionID)
	if err != nil {
		return nil, err
	}
	for _, rep := range reps {
		if rep.ReplicaID == scope.ReplicaID {
			if rep.Generation != scope.Generation {
				return nil, &CommandError{Reason: contract.ReasonGenerationStale, Message: "generation mismatch"}
			}
			return &rep, nil
		}
	}
	return nil, errReplicaNotFound
}

func (h *Handler) stopCompatibility(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	rep, err := h.compatibilityReplica(ctx, scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaStopped
	if err := h.reps.UpsertRuntimeReplica(ctx, *rep); err != nil {
		return contract.RuntimeReplica{}, err
	}
	return *rep, nil
}

func (h *Handler) drainCompatibility(ctx context.Context, scope contract.CommandScope) (contract.RuntimeReplica, error) {
	rep, err := h.compatibilityReplica(ctx, scope)
	if err != nil {
		return contract.RuntimeReplica{}, err
	}
	rep.State = contract.ReplicaDraining
	if err := h.reps.UpsertRuntimeReplica(ctx, *rep); err != nil {
		return contract.RuntimeReplica{}, err
	}
	return *rep, nil
}

func (h *Handler) listCompatibility(ctx context.Context, scope contract.CommandScope) ([]contract.RuntimeReplica, error) {
	if h.reps == nil {
		return nil, nil
	}
	reps, err := h.reps.ListRuntimeReplicas(ctx, scope.ProjectID, scope.VersionID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.RuntimeReplica, 0, len(reps))
	for _, rep := range reps {
		if rep.NodeID == scope.NodeID {
			out = append(out, rep)
		}
	}
	return out, nil
}
