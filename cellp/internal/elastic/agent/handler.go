package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
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

// readyCommitAttempts bounds the authoritative re-read of a ready commit. The lease
// moves on every node heartbeat, so one retry after a lost CAS is normally enough;
// the bound keeps a pathological mover from spinning.
const readyCommitAttempts = 3

// readyCommitRetryable reports whether a ready commit lost a lease race rather than
// hitting an authoritative fence. Only the race is retried; fences and expiry stay
// terminal and are never retried into a successful commit.
func readyCommitRetryable(err error) bool {
	return errors.Is(err, registry.ErrObservationStale) || errors.Is(err, registry.ErrAssignmentCASConflict)
}

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
	// The ready observation must carry the assignment lease the registry holds now,
	// not the one this attempt was claimed with: the lease moves on every node
	// heartbeat and renewal. Committing the stale value fails the CAS, and the old
	// path answered that with cleanup=true, which stopped a process that was already
	// serving. Re-read the authoritative identity instead and only fail closed when
	// the assignment is genuinely fenced or expired — and then without killing a
	// serving process.
	commitReady := func() error {
		live, readErr := h.store.ValidateAgentCleanupAssignment(ctx, spec.Scope)
		if readErr != nil {
			return h.commandStoreError(readErr)
		}
		if live == nil {
			return errReplicaNotFound
		}
		if live.Generation != rep.Generation || live.State == contract.ReplicaStopped || live.State == contract.ReplicaFailed {
			return &CommandError{Reason: contract.ReasonGenerationStale, Message: "assignment fenced before ready commit"}
		}
		now := h.now().UTC()
		if live.ValidUntil == nil || !live.ValidUntil.After(now) {
			return &CommandError{Reason: contract.ReasonLeaseExpired, Message: "assignment lease expired before ready commit"}
		}
		command.Status, command.ResultState = "succeeded", contract.ReplicaReady
		if err := h.store.RecordObservationAndCompleteAgentCommand(ctx, readyObservation(*live, host, port), command); err != nil {
			return err
		}
		rep.State = contract.ReplicaReady
		return nil
	}
	var commitErr error
	for attempt := 0; attempt < readyCommitAttempts; attempt++ {
		if commitErr = commitReady(); commitErr == nil {
			return *rep, nil
		}
		if !readyCommitRetryable(commitErr) {
			break
		}
	}
	var commandErr *CommandError
	reason := contract.ReasonGenerationStale
	if errors.As(commitErr, &commandErr) {
		reason = commandErr.Reason
	}
	if !readyCommitRetryable(commitErr) {
		// An authoritative fence or an expired lease: this attempt no longer owns the
		// assignment, so its process must not keep serving. Clean it up deterministically.
		return fail(reason, commitErr, true)
	}
	// A lost lease race only. Complete the command as failed so the scheduler is not left
	// waiting, but never with cleanup: the process may already serve, and a moved lease
	// must not stop it. Any earlier ready endpoint stays published until its own lease
	// expires.
	return fail(reason, commitErr, false)
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
// reconcileAuthorityRead marks a read that found the node's leases gone. Reading the
// inventory and finding lease expiry is the node's own proof that it lost ownership, so
// it takes the node offline (fail closed) exactly like a lost heartbeat; other read
// failures stay transient.
func reconcileAuthorityRead(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, registry.ErrLeaseExpired) || errors.Is(err, registry.ErrNodeLeaseCASConflict) {
		return errors.Join(ErrReconcileAuthority, err)
	}
	return err
}

func (h *Handler) ReconcileNode(ctx context.Context, nodeID string) error {
	if h.store == nil || h.backend == nil {
		return fmt.Errorf("lifecycle backend and store required")
	}
	node, nodeErr := h.nodes.GetRuntimeNode(ctx, nodeID)
	if nodeErr != nil {
		return reconcileAuthorityRead(fmt.Errorf("runtime node read: %w", nodeErr))
	}
	facts, factsErr := h.store.ListRuntimeReplicasByNode(ctx, nodeID)
	inventory, inventoryErr := h.backend.List(ctx)
	if factsErr != nil || inventoryErr != nil {
		return reconcileAuthorityRead(errors.Join(factsErr, inventoryErr))
	}
	now := h.now().UTC()
	// The inventory read can take tens of seconds — it shares the replica lifecycle lock,
	// which a restart holds across its stop-then-start window — while the node row above
	// was read before it. Deciding inactivity from that snapshot compares an old lease
	// against a fresh clock and takes a healthy node offline. Re-read the node under the
	// decision's own clock, and only then may a genuinely inactive node be authoritative.
	if node == nil || node.Cordoned || !node.LeaseExpiry.After(now) {
		fresh, freshErr := h.nodes.GetRuntimeNode(ctx, nodeID)
		if freshErr != nil {
			return reconcileAuthorityRead(fmt.Errorf("runtime node re-read: %w", freshErr))
		}
		node = fresh
		now = h.now().UTC()
	}
	nodeActive := node != nil && !node.Cordoned && node.LeaseExpiry.After(now)
	if !nodeActive {
		// The node's own authority is gone. Record what the reconcile saw before taking
		// every replica down, so the first cause is not inferred from shutdown noise.
		lease := time.Time{}
		cordoned := false
		if node != nil {
			lease, cordoned = node.LeaseExpiry, node.Cordoned
		}
		log.Printf("agent reconcile: node inactive node=%s present=%v cordoned=%v lease_expiry=%s now=%s",
			nodeID, node != nil, cordoned, lease.UTC().Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
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
		// The agent reads the registry in-process, so an expired assignment lease here
		// is authoritative for this node: the process must be terminalized and stopped
		// instead of being held as uncertain forever. Only this local decision carries
		// that reading; the remote/wire sentinel semantics stay unchanged, and a moved
		// lease (a lost CAS) stays uncertain and is retried on the next reconcile.
		authoritativelyInvalid := validated == nil && assignmentErr == nil ||
			registrywire.IsAuthoritativeGenerationStale(assignmentErr) || registrywire.IsAuthoritativeLeaseExpired(assignmentErr) ||
			errors.Is(assignmentErr, registry.ErrObservationStale) || errors.Is(assignmentErr, registry.ErrLeaseExpired)
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
		item, inInventory := local[rep.ReplicaID]
		running := inInventory && item.Alive
		// Terminal states are authoritative and are cleaned before anything else looks at
		// health: a stopped, failed or draining replica that still has a process must not
		// be promoted to ready from a leftover healthy process, and reconcile never
		// revives one. The scheduler owns replacement, through a new assignment. Such a
		// replica stays out of `valid`, so a dead inventory record for it is swept below.
		if rep.State == contract.ReplicaStopped || rep.State == contract.ReplicaFailed || rep.State == contract.ReplicaDraining {
			if running {
				if stopErr := h.backend.Stop(ctx, stopScope); stopErr != nil {
					allErrs = append(allErrs, fmt.Errorf("stop terminal replica %s: %w", rep.ReplicaID, stopErr))
					continue
				}
			}
			if rep.State != contract.ReplicaStopped {
				if err := h.record(ctx, rep, contract.ReplicaStopped, "", 0); err != nil && !ReconcileRecordBenign(err) {
					allErrs = append(allErrs, fmt.Errorf("terminalize cleaned replica %s: %w", rep.ReplicaID, err))
				}
			}
			continue
		}
		valid[rep.ReplicaID] = rep
		identityHealthy := running && backendMatches(rep, item)
		if identityHealthy {
			if err := h.recordReadyIfRunning(ctx, &rep, item.Host, item.Port); err != nil {
				allErrs = append(allErrs, fmt.Errorf("record ready replica %s: %w", rep.ReplicaID, err))
			}
			continue
		}
		// celld withholds {"ok":true} until its ready gate settles; after a handoff or a
		// dead peer lease that takes tens of seconds, and `Start` is answered as soon as
		// the process exists. Killing a process that is still booting leaves a lingering
		// node lease in the version bucket, and the replacement then waits that lease out
		// before it can serve: one slow boot becomes replica churn.
		//
		// Only a non-terminal replica may be kept: a stopped, failed or draining replica
		// is an authoritative decision, and a serving replica that lost health is
		// withdrawn and replaced exactly as upstream does. Slot, lease and process state
		// stay consistent because a kept replica is still `pending`/`starting`, so it
		// holds no endpoint and no slot beyond the assignment it already owns.
		if running && backendIdentityMatches(rep, item) && !backendMatches(rep, item) &&
			(rep.State == contract.ReplicaPending || rep.State == contract.ReplicaStarting) {
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
		// A lease that moved under us (node heartbeat / renewal) is a lost CAS race, not a
		// fence. Reporting it as generation_stale would make the scheduler terminalize a
		// serving replica on every renewal, so it stays retryable; genuine fences and
		// expired leases keep failing closed.
		if errors.Is(err, registry.ErrAssignmentCASConflict) {
			return nil, &CommandError{Reason: contract.ReasonColdActivating, Message: "assignment lease moved"}
		}
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
	case contract.ReplicaStarting:
		return nil
	case contract.ReplicaReady:
		// In-memory facts may show ready while registry is still pending (Start command
		// path racing with reconcile). Attempt starting so pending→ready is valid.
		if err := h.record(ctx, *rep, contract.ReplicaStarting, "", 0); err != nil {
			if errors.Is(err, registry.ErrReplicaTransitionInvalid) || errors.Is(err, registry.ErrObservationStale) {
				return nil
			}
			return err
		}
		rep.State = contract.ReplicaStarting
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
		return nil
	}
	if err := h.recordStartingIfNeeded(ctx, rep); err != nil {
		return err
	}
	if err := h.record(ctx, *rep, contract.ReplicaReady, host, port); err != nil {
		if ReconcileRecordBenign(err) {
			return nil
		}
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
