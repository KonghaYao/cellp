package serve

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

// Operator commands reach a version's cells through a live celld of that version
// (`celld kv|queue|r2` open a namespace on a node of the version bucket). A version that
// is scaled to zero therefore has no fleet to talk to. Waking it is the activator's job:
// the same ensure-desire bump the Gateway uses for a cold request, followed by an idle
// release so scale-to-zero still applies once the command finished.
const (
	operatorEnsureReason = "activator_ensure"
	operatorIdleReason   = "idle"
)

// OperatorServingStore is the registry surface an operator wake needs.
type OperatorServingStore interface {
	GetServingPolicy(ctx context.Context, projectID, versionID string) (*registry.ServingPolicyRow, error)
	GetServingDesire(ctx context.Context, projectID, versionID string) (*registry.ServingDesireRow, error)
	CompareAndSetDesired(ctx context.Context, projectID, versionID string, expectGen int64, desire registry.ServingDesireRow) error
	ListRuntimeReplicas(ctx context.Context, projectID, versionID string) ([]contract.RuntimeReplica, error)
}

// operatorServingEnsurer holds a version up for the duration of an operator command and
// returns it to its previous desire afterwards.
//
// A version is pinned even when a replica is already live: otherwise a scale-to-zero
// decision taken while the command runs would retire the very fleet the command is
// talking to. Concurrent operators share one pin — the first one in creates it, the last
// one out drops it — so they cannot release each other's wake. A version the activator
// (or a promotion) already holds keeps its own pin: only the desire this ensurer raised
// is lowered again.
type operatorServingEnsurer struct {
	store  OperatorServingStore
	ensure activator.EnsureCapacityClient
	wait   time.Duration
	poll   time.Duration

	mu     sync.Mutex
	active map[string]*operatorPin
}

type operatorPin struct {
	count  int
	owns   bool
	before *registry.ServingDesireRow
}

func newOperatorServingEnsurer(store OperatorServingStore, ensure activator.EnsureCapacityClient) *operatorServingEnsurer {
	return &operatorServingEnsurer{
		store: store, ensure: ensure,
		wait: operatorWakeWait, poll: operatorWakePoll,
		active: map[string]*operatorPin{},
	}
}

const (
	// operatorWakeWait covers a retired version's replacement: the version is woken, the
	// scheduler places a fresh replica, and celld withholds health until its ready gate
	// settles past the retired node's lease. Measured on the dev stack: ~140s from pin to
	// first ready replica after a post-qualification retire, so the budget is set well
	// above it. It is a wait budget for the operator call, not a relaxation of any gate:
	// the call still fails if no replica becomes ready.
	operatorWakeWait = 240 * time.Second
	operatorWakePoll = 200 * time.Millisecond
)

// Ensure holds the version up while an operator command runs. It returns a nil release
// when the version is served by the legacy track and needs no elastic pin.
func (e *operatorServingEnsurer) Ensure(ctx context.Context, projectID, versionID string) (func(), error) {
	if e == nil || e.store == nil {
		return nil, nil
	}
	policy, err := e.store.GetServingPolicy(ctx, projectID, versionID)
	if err != nil {
		return nil, err
	}
	// The legacy track serves the version directly; only enrolled versions are placed by
	// the elastic scheduler and can be cold or scaled to zero.
	if policy == nil || !policy.ElasticEnrolled {
		return nil, nil
	}
	key := projectID + "/" + versionID
	first, pin := e.enter(key)
	if first {
		if err := e.pinDesire(ctx, projectID, versionID, pin); err != nil {
			e.finish(key)
			return nil, err
		}
	}
	live, err := e.live(ctx, projectID, versionID)
	if err != nil {
		e.finish(key)
		return nil, err
	}
	if !live {
		if err := e.waitLive(ctx, projectID, versionID); err != nil {
			e.finish(key)
			return nil, err
		}
	}
	return func() { e.finish(key) }, nil
}

// enter registers one operator on the version's shared pin.
func (e *operatorServingEnsurer) enter(key string) (bool, *operatorPin) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pin, ok := e.active[key]; ok {
		pin.count++
		return false, pin
	}
	pin := &operatorPin{count: 1}
	e.active[key] = pin
	return true, pin
}

// finish leaves the version's shared pin, dropping the desire once the last operator is out.
func (e *operatorServingEnsurer) finish(key string) {
	e.mu.Lock()
	pin := e.active[key]
	if pin == nil {
		e.mu.Unlock()
		return
	}
	pin.count--
	last := pin.count <= 0
	owns, before := pin.owns, pin.before
	if last {
		delete(e.active, key)
	}
	e.mu.Unlock()
	if !last || !owns {
		return
	}
	e.dropPin(context.WithoutCancel(context.Background()), key, before)
}

// pinDesire raises the serving desire and records whether this ensurer created the pin.
func (e *operatorServingEnsurer) pinDesire(ctx context.Context, projectID, versionID string, pin *operatorPin) error {
	if e.ensure == nil {
		return errors.New("operator wake unavailable")
	}
	before, err := e.store.GetServingDesire(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	// A version already held by the activator (or a promotion) keeps that pin.
	if before != nil && before.DesiredReplicas >= 1 {
		e.mu.Lock()
		pin.before = before
		e.mu.Unlock()
		return nil
	}
	if err := e.ensure.EnsureCapacity(ctx, projectID, versionID, 1); err != nil {
		return fmt.Errorf("wake %s/%s: %w", projectID, versionID, err)
	}
	after, err := e.store.GetServingDesire(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	owns := after != nil && after.DesiredReplicas >= 1 && (before == nil || after.Generation != before.Generation)
	e.mu.Lock()
	pin.before, pin.owns = before, owns
	e.mu.Unlock()
	return nil
}

// dropPin lowers the desire this ensurer raised, and only that one.
func (e *operatorServingEnsurer) dropPin(ctx context.Context, key string, before *registry.ServingDesireRow) {
	projectID, versionID, ok := strings.Cut(key, "/")
	if !ok {
		return
	}
	current, err := e.store.GetServingDesire(ctx, projectID, versionID)
	if err != nil || current == nil {
		return
	}
	if current.DesiredReplicas < 1 || current.Reason != operatorEnsureReason {
		return
	}
	restore, reason := 0, operatorIdleReason
	if before != nil && before.DesiredReplicas > 0 {
		restore = before.DesiredReplicas
		if before.Reason != "" {
			reason = before.Reason
		}
	}
	_ = e.store.CompareAndSetDesired(ctx, projectID, versionID, current.Generation, registry.ServingDesireRow{
		ProjectID: projectID, VersionID: versionID,
		DesiredReplicas: restore, Generation: current.Generation + 1, Reason: reason,
	})
}

func (e *operatorServingEnsurer) live(ctx context.Context, projectID, versionID string) (bool, error) {
	replicas, err := e.store.ListRuntimeReplicas(ctx, projectID, versionID)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	for _, rep := range replicas {
		if rep.State == contract.ReplicaReady && contract.AssignmentOccupiesSlot(rep, now) {
			return true, nil
		}
	}
	return false, nil
}

func (e *operatorServingEnsurer) waitLive(ctx context.Context, projectID, versionID string) error {
	deadline := time.Now().Add(e.wait)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		live, err := e.live(ctx, projectID, versionID)
		if err != nil {
			return err
		}
		if live {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no live replica for %s/%s", projectID, versionID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(e.poll):
		}
	}
}

// release drops the pin this wake placed, so the version returns to its previous desire
// and scale-to-zero keeps applying. Only the wake's own pin is dropped: a replica the
// activator (or a promotion) raised for its own reason stays up.
func (e *operatorServingEnsurer) release(ctx context.Context, projectID, versionID string, before *registry.ServingDesireRow) {
	current, err := e.store.GetServingDesire(ctx, projectID, versionID)
	if err != nil || current == nil {
		return
	}
	if current.DesiredReplicas < 1 || current.Reason != operatorEnsureReason {
		return
	}
	restore := 0
	if before != nil && before.DesiredReplicas > 0 {
		restore = before.DesiredReplicas
	}
	reason := operatorIdleReason
	if before != nil && before.DesiredReplicas > 0 && before.Reason != "" {
		reason = before.Reason
	}
	_ = e.store.CompareAndSetDesired(ctx, projectID, versionID, current.Generation, registry.ServingDesireRow{
		ProjectID: projectID, VersionID: versionID,
		DesiredReplicas: restore, Generation: current.Generation + 1, Reason: reason,
	})
}
