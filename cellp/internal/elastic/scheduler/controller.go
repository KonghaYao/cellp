package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
	"github.com/google/uuid"
)

const (
	defaultDrainGrace            = 30 * time.Second
	defaultAssignmentRenewWindow = 30 * time.Second
)

// Controller reconciles serving desires into generation-fenced assignments and Agent commands.
type Controller struct {
	Store   Store
	Guard   ControllerGuardChecker
	Clients ClientFactory
	Now     func() time.Time
}

// TickReport summarizes one scheduler pass.
type TickReport struct {
	Skipped  bool
	Versions int
}

// Enabled reports whether elastic scheduler logic may run.
func Enabled() bool {
	return true
}

// Tick reads registry state and drives placement plus lifecycle convergence.
func (c *Controller) Tick(ctx context.Context) (TickReport, error) {
	if !Enabled() {
		return TickReport{Skipped: true}, nil
	}
	if c == nil || c.Store == nil {
		return TickReport{}, nil
	}
	if c.Guard == nil {
		return TickReport{}, ErrGuardLost
	}
	if err := c.checkGuard(ctx); err != nil {
		return TickReport{}, err
	}
	SetPlacementDegraded(false)
	policies, err := c.Store.ListElasticServingPolicies(ctx)
	if err != nil {
		if ignoreContext(ctx, err) {
			return TickReport{}, nil
		}
		return TickReport{}, err
	}
	nodes, err := c.Store.ListRuntimeNodes(ctx)
	if err != nil {
		if ignoreContext(ctx, err) {
			return TickReport{}, nil
		}
		return TickReport{}, err
	}
	nodeIndex := indexNodes(nodes)
	now := c.now().UTC()
	for _, pol := range policies {
		if err := c.reconcileVersion(ctx, pol, nodeIndex, now); err != nil {
			if ignoreContext(ctx, err) {
				return TickReport{Versions: len(policies)}, nil
			}
			if TickFatal(err) {
				return TickReport{}, err
			}
			if IsTransientAgentOrRegistry(err) {
				log.Printf("scheduler: transient %s/%s: %v", pol.ProjectID, pol.VersionID, err)
				continue
			}
			return TickReport{}, err
		}
	}
	return TickReport{Versions: len(policies)}, nil
}

func (c *Controller) reconcileVersion(ctx context.Context, pol registry.ServingPolicyRow, nodes map[string]contract.RuntimeNode, now time.Time) error {
	policy := contract.ServingPolicy{
		Revision: pol.Revision, MinReplicas: pol.MinReplicas, MaxReplicas: pol.MaxReplicas,
		Priority: pol.Priority, BackgroundMode: pol.BackgroundMode, ElasticEnrolled: pol.ElasticEnrolled,
	}
	if err := contract.ValidateServingPolicyBackground(policy, contract.BackgroundGuardOptions{}); err != nil {
		return fmt.Errorf("serving policy: %w", err)
	}
	desire, err := c.Store.GetServingDesire(ctx, pol.ProjectID, pol.VersionID)
	if err != nil {
		return err
	}
	if desire == nil {
		return nil
	}
	desired, desireGen := resolveDesired(pol, desire)
	version, err := c.Store.GetVersion(ctx, pol.ProjectID, pol.VersionID)
	if err != nil {
		return err
	}
	if version != nil && versionStatusBlocksElasticPlacement(version.Status) {
		desired = 0
	}
	replicas, err := c.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
	if err != nil {
		return err
	}
	fleet, err := c.Store.ListRuntimeReplicasForReconcile(ctx)
	if err != nil {
		return err
	}
	targets := PickScaleDownTargets(replicas, desireGen, desired)
	targetSet := replicaSet(targets)
	for _, rep := range replicas {
		if isTerminalReplica(rep.State) {
			continue
		}
		if err := c.checkGuard(ctx); err != nil {
			return err
		}
		if rep.ValidUntil == nil || !rep.ValidUntil.After(now) {
			if err := c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); err != nil {
				return err
			}
			continue
		}
		node, knownNode := nodes[rep.NodeID]
		if !knownNode || !nodeOK(node, now) || rep.AssignedNodeGeneration <= 0 || rep.AssignedNodeGeneration != node.Generation {
			if err := c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); err != nil {
				return err
			}
			continue
		}
		_, selected := targetSet[rep.ReplicaID]
		if selected {
			if err := c.retireReplica(ctx, rep, &node, now); err != nil {
				return err
			}
			continue
		}
		if err := c.convergeReplica(ctx, rep, node, now); err != nil {
			return err
		}
	}
	replicas, err = c.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
	if err != nil {
		return err
	}
	fleet, err = c.Store.ListRuntimeReplicasForReconcile(ctx)
	if err != nil {
		return err
	}
	active := CountMatchingActive(replicas, now)
	for active < desired {
		if err := c.checkGuard(ctx); err != nil {
			return err
		}
		replicas, err = c.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
		if err != nil {
			return err
		}
		active = CountMatchingActive(replicas, now)
		if active >= desired {
			break
		}
		prevActive := active
		claimed, err := c.placeReplica(ctx, pol, desireGen, desired, nodes, fleet, replicas, now)
		if err != nil {
			if errors.Is(err, ErrCapacityExhausted) {
				return nil
			}
			return err
		}
		fleet, err = c.Store.ListRuntimeReplicasForReconcile(ctx)
		if err != nil {
			return err
		}
		replicas, err = c.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
		if err != nil {
			return err
		}
		active = CountMatchingActive(replicas, now)
		if !claimed && active == prevActive {
			break
		}
	}
	return c.reconcileVersionServingStatus(ctx, pol, desire, now)
}

func (c *Controller) reconcileVersionServingStatus(ctx context.Context, pol registry.ServingPolicyRow, desire *registry.ServingDesireRow, now time.Time) error {
	if desire == nil {
		return nil
	}
	version, err := c.Store.GetVersion(ctx, pol.ProjectID, pol.VersionID)
	if err != nil || version == nil {
		return err
	}
	replicas, err := c.Store.ListRuntimeReplicas(ctx, pol.ProjectID, pol.VersionID)
	if err != nil {
		return err
	}
	ready := false
	occupied := false
	for _, rep := range replicas {
		if contract.AssignmentOccupiesSlot(rep, now) {
			occupied = true
		}
		if rep.State == contract.ReplicaReady && contract.AssignmentOccupiesSlot(rep, now) {
			ready = true
			break
		}
	}
	if version.Status == contract.StatusDeployReady && desire.Reason == "activator_ensure" && ready {
		view, _, err := c.Store.BuildQualificationViewAfter(ctx, -1)
		if err != nil {
			return err
		}
		if !qualificationEndpointPresent(view, pol.ProjectID, pol.VersionID, now) {
			return nil
		}
		if err := c.checkGuard(ctx); err != nil {
			return err
		}
		if err := c.Store.CompareAndSetElasticVersionStatus(ctx, pol.ProjectID, pol.VersionID, contract.StatusDeployReady, contract.StatusReady, desire.Generation, desire.DesiredReplicas); err != nil && !errors.Is(err, registry.ErrElasticVersionStatusCASConflict) {
			return err
		}
		return nil
	}
	if version.Status == contract.StatusReady && pol.MinReplicas == 0 && desire.DesiredReplicas == 0 && !occupied {
		if err := c.checkGuard(ctx); err != nil {
			return err
		}
		if err := c.Store.CompareAndSetElasticVersionStatus(ctx, pol.ProjectID, pol.VersionID, contract.StatusReady, contract.StatusDeployReady, desire.Generation, desire.DesiredReplicas); err != nil && !errors.Is(err, registry.ErrElasticVersionStatusCASConflict) {
			return err
		}
	}
	return nil
}

func qualificationEndpointPresent(view registry.QualificationView, projectID, versionID string, now time.Time) bool {
	for _, set := range view.EndpointSets {
		if set.ProjectID != projectID || set.VersionID != versionID {
			continue
		}
		for _, endpoint := range set.Endpoints {
			if endpoint.State == contract.EndpointReady && endpoint.ValidUntil != nil && endpoint.ValidUntil.After(now) && endpoint.Address != "" {
				return true
			}
		}
	}
	return false
}

func (c *Controller) placeReplica(ctx context.Context, pol registry.ServingPolicyRow, desireGen int64, desired int, nodes map[string]contract.RuntimeNode, fleet, replicas []contract.RuntimeReplica, now time.Time) (bool, error) {
	nodeList := make([]contract.RuntimeNode, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}
	pickRes := PickNodeDetailed(PlacementInput{
		ProjectID: pol.ProjectID, VersionID: pol.VersionID,
		Nodes: nodeList, Replicas: fleet, Now: now,
	})
	pick := pickRes.Node
	if pick == nil {
		return false, ErrCapacityExhausted
	}
	if pickRes.Degraded {
		SetPlacementDegraded(true)
		log.Printf("scheduler: placement_degraded=true project=%s version=%s node=%s", pol.ProjectID, pol.VersionID, pick.NodeID)
	}
	client, err := c.clientFor(*pick)
	if err != nil {
		return false, err
	}
	validUntil := assignmentValidUntil(*pick)
	replicaID := NextReplicaID(pol.ProjectID, pol.VersionID, desireGen, fleet)
	if err := c.checkGuard(ctx); err != nil {
		return false, err
	}
	claim := registry.AssignmentClaim{
		ReplicaID: replicaID, ProjectID: pol.ProjectID, VersionID: pol.VersionID,
		NodeID: pick.NodeID, Generation: desireGen, ExpectedNodeGeneration: pick.Generation,
		ActiveLimit: desired, ValidUntil: validUntil,
	}
	if err := c.checkGuard(ctx); err != nil {
		return false, err
	}
	if err := c.Store.ClaimAssignment(ctx, claim); err != nil {
		if errors.Is(err, registry.ErrAssignmentCASConflict) {
			return false, nil
		}
		return false, err
	}
	rep := contract.RuntimeReplica{
		ReplicaID: replicaID, ProjectID: pol.ProjectID, VersionID: pol.VersionID,
		NodeID: pick.NodeID, Generation: desireGen, AssignedNodeGeneration: pick.Generation,
		State: contract.ReplicaPending, ValidUntil: &validUntil,
	}
	scope := commandScope(*pick, rep, validUntil, contract.ActionStartReplica)
	spec := contract.StartReplicaSpec{
		Scope:  scope,
		Bucket: replicaBucket(pol.ProjectID, pol.VersionID),
	}
	if err := c.checkGuard(ctx); err != nil {
		if termErr := c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); termErr != nil && !IsTransientAgentOrRegistry(termErr) {
			return true, errors.Join(err, termErr)
		}
		return true, err
	}
	_, err = client.StartReplica(ctx, spec, idempotencyKey(contract.ActionStartReplica, rep.ReplicaID, rep.Generation))
	if err != nil {
		if handleErr := c.handleAgentErr(ctx, rep, err); handleErr != nil {
			return true, handleErr
		}
		return true, nil
	}
	return true, nil
}

func (c *Controller) convergeReplica(ctx context.Context, rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if err := c.renewAssignmentIfNeeded(ctx, &rep, node, now); err != nil {
		if errors.Is(err, registry.ErrAssignmentCASConflict) || errors.Is(err, registry.ErrLeaseExpired) {
			return c.terminalizeStaleAssignment(ctx, rep)
		}
		if IsTransientAgentOrRegistry(err) {
			return nil
		}
		return err
	}
	switch rep.State {
	case contract.ReplicaPending, contract.ReplicaFailed, contract.ReplicaStopped:
		return c.startReplica(ctx, rep, node, now)
	case contract.ReplicaStarting, contract.ReplicaReady:
		return c.probeReplica(ctx, rep, node, now)
	case contract.ReplicaDraining:
		return c.stopReplica(ctx, rep, node, now)
	default:
		return nil
	}
}

func (c *Controller) renewAssignmentIfNeeded(ctx context.Context, rep *contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if rep == nil || rep.ValidUntil == nil || rep.AssignedNodeGeneration != node.Generation {
		return registry.ErrAssignmentCASConflict
	}
	if rep.ValidUntil.Sub(now) > defaultAssignmentRenewWindow {
		return nil
	}
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	validated, err := c.Store.ValidateAgentAssignment(ctx, commandScope(node, *rep, *rep.ValidUntil, contract.ActionProbeReplica), now)
	if err != nil {
		return err
	}
	if validated == nil {
		return registry.ErrAssignmentCASConflict
	}
	newExpiry := assignmentValidUntil(node)
	if !newExpiry.After(*rep.ValidUntil) {
		return nil
	}
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	if err := c.Store.RenewAssignmentLease(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, node.Generation, *rep.ValidUntil, newExpiry); err != nil {
		if errors.Is(err, registry.ErrAssignmentCASConflict) || errors.Is(err, registry.ErrLeaseExpired) {
			return registry.ErrAssignmentCASConflict
		}
		return err
	}
	rep.ValidUntil = &newExpiry
	return nil
}

func (c *Controller) retireReplica(ctx context.Context, rep contract.RuntimeReplica, node *contract.RuntimeNode, now time.Time) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	if node == nil {
		return registry.ErrAssignmentCASConflict
	}
	switch rep.State {
	case contract.ReplicaReady:
		return c.drainReplica(ctx, rep, *node, now)
	case contract.ReplicaDraining:
		return c.stopReplica(ctx, rep, *node, now)
	default:
		if err := c.stopReplica(ctx, rep, *node, now); err != nil {
			if CommandErrorFatal(err) {
				return c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaStopped)
			}
			return err
		}
		return nil
	}
}

func (c *Controller) startReplica(ctx context.Context, rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	validUntil, err := replicaValidUntil(rep, node, now)
	if err != nil {
		return err
	}
	client, err := c.clientFor(node)
	if err != nil {
		return err
	}
	scope := commandScope(node, rep, validUntil, contract.ActionStartReplica)
	spec := contract.StartReplicaSpec{Scope: scope, Bucket: replicaBucket(rep.ProjectID, rep.VersionID)}
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	_, err = client.StartReplica(ctx, spec, idempotencyKey(contract.ActionStartReplica, rep.ReplicaID, rep.Generation))
	if err != nil {
		return c.handleAgentErr(ctx, rep, err)
	}
	return nil
}

func (c *Controller) probeReplica(ctx context.Context, rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	validUntil, err := replicaValidUntil(rep, node, now)
	if err != nil {
		return err
	}
	client, err := c.clientFor(node)
	if err != nil {
		return err
	}
	scope := commandScope(node, rep, validUntil, contract.ActionProbeReplica)
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	_, err = client.ProbeReplica(ctx, scope)
	if err != nil {
		return c.handleAgentErr(ctx, rep, err)
	}
	return nil
}

func (c *Controller) drainReplica(ctx context.Context, rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	validUntil, err := replicaValidUntil(rep, node, now)
	if err != nil {
		return err
	}
	client, err := c.clientFor(node)
	if err != nil {
		return err
	}
	scope := commandScope(node, rep, validUntil, contract.ActionDrainReplica)
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	_, err = client.DrainReplica(ctx, scope, now.Add(defaultDrainGrace))
	if err != nil {
		return c.handleAgentErr(ctx, rep, err)
	}
	return nil
}

func (c *Controller) stopReplica(ctx context.Context, rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	validUntil, err := replicaValidUntil(rep, node, now)
	if err != nil {
		return err
	}
	client, err := c.clientFor(node)
	if err != nil {
		return err
	}
	scope := commandScope(node, rep, validUntil, contract.ActionStopReplica)
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	_, err = client.StopReplica(ctx, scope)
	if err != nil {
		return c.handleAgentErr(ctx, rep, err)
	}
	return nil
}

func (c *Controller) handleAgentErr(ctx context.Context, rep contract.RuntimeReplica, err error) error {
	if err == nil {
		return nil
	}
	if GuardLostFatal(err) {
		return err
	}
	if CommandErrorFatal(err) {
		if guardErr := c.checkGuard(ctx); guardErr != nil {
			return guardErr
		}
		if termErr := c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); termErr != nil && !IsTransientAgentOrRegistry(termErr) {
			return errors.Join(err, termErr)
		}
		return nil
	}
	if IsTransientAgentOrRegistry(err) {
		return err
	}
	return err
}

func (c *Controller) terminalizeStaleAssignment(ctx context.Context, rep contract.RuntimeReplica) error {
	if err := c.checkGuard(ctx); err != nil {
		return err
	}
	if err := c.Store.TerminalizeReplica(ctx, rep.ReplicaID, rep.NodeID, rep.Generation, contract.ReplicaFailed); err != nil {
		if IsTransientAgentOrRegistry(err) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Controller) clientFor(node contract.RuntimeNode) (RuntimeNodeClient, error) {
	if c.Clients == nil {
		return nil, fmt.Errorf("client factory required")
	}
	return c.Clients(node)
}

func (c *Controller) checkGuard(ctx context.Context) error {
	if c == nil || c.Guard == nil {
		return ErrGuardLost
	}
	return guardError(c.Guard.HoldsWriteLock(ctx))
}

func (c *Controller) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func versionStatusBlocksElasticPlacement(status string) bool {
	switch status {
	case registry.StatusFailed, registry.StatusArchived, registry.StatusDestroyed, registry.StatusDraining:
		return true
	default:
		return false
	}
}

func resolveDesired(pol registry.ServingPolicyRow, desire *registry.ServingDesireRow) (int, int64) {
	desired := pol.MinReplicas
	gen := desire.Generation
	if desire != nil {
		desired = desire.DesiredReplicas
	}
	if desired < pol.MinReplicas {
		desired = pol.MinReplicas
	}
	if desired > pol.MaxReplicas {
		desired = pol.MaxReplicas
	}
	if desired < 0 {
		desired = 0
	}
	return desired, gen
}

func replicaBucket(projectID, versionID string) string {
	return fmt.Sprintf("s3://cellp-celld/%s/%s", projectID, versionID)
}

func assignmentValidUntil(node contract.RuntimeNode) time.Time {
	return node.LeaseExpiry.UTC().Truncate(time.Microsecond)
}

func replicaValidUntil(rep contract.RuntimeReplica, node contract.RuntimeNode, now time.Time) (time.Time, error) {
	if rep.ValidUntil != nil && rep.ValidUntil.After(now) {
		return rep.ValidUntil.UTC().Truncate(time.Microsecond), nil
	}
	until := assignmentValidUntil(node)
	if !until.After(now) {
		return time.Time{}, registry.ErrLeaseExpired
	}
	return until, nil
}

func commandScope(node contract.RuntimeNode, rep contract.RuntimeReplica, validUntil time.Time, action contract.LifecycleAction) contract.CommandScope {
	return contract.CommandScope{
		NodeID: node.NodeID, ProjectID: rep.ProjectID, VersionID: rep.VersionID, ReplicaID: rep.ReplicaID,
		Generation: rep.Generation, LeaseExpiry: validUntil, Nonce: commandNonce(),
		Action: action,
	}
}

func commandNonce() string {
	return "sch-" + uuid.NewString()
}

func idempotencyKey(action contract.LifecycleAction, replicaID string, generation int64) string {
	return fmt.Sprintf("scheduler.%s.%s.%d", action, replicaID, generation)
}

func indexNodes(nodes []contract.RuntimeNode) map[string]contract.RuntimeNode {
	out := make(map[string]contract.RuntimeNode, len(nodes))
	for _, n := range nodes {
		out[n.NodeID] = n
	}
	return out
}

func nodeOK(node contract.RuntimeNode, now time.Time) bool {
	return nodeEligible(node, now)
}

func replicaSet(reps []contract.RuntimeReplica) map[string]struct{} {
	out := make(map[string]struct{}, len(reps))
	for _, rep := range reps {
		out[rep.ReplicaID] = struct{}{}
	}
	return out
}
