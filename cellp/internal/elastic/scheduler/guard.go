package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cellp/cellp/internal/registry"
)

// ErrGuardLost is returned when the singleton controller guard is no longer held.
var ErrGuardLost = errors.New("controller_guard_lost")

// ControllerGuardChecker verifies the active cellpd writer lease before mutating state.
type ControllerGuardChecker interface {
	HoldsWriteLock(ctx context.Context) error
}

// RegistryGuard checks holder id and pid against registry.controller_guard.
type RegistryGuard struct {
	Store    registry.ServingStore
	HolderID string
	PID      int
}

// HoldsWriteLock returns nil only when this process still owns the guard row.
func (g RegistryGuard) HoldsWriteLock(ctx context.Context) error {
	if g.Store == nil || strings.TrimSpace(g.HolderID) == "" {
		return ErrGuardLost
	}
	st, err := g.Store.GetControllerGuard(ctx)
	if err != nil {
		return err
	}
	if st == nil || st.HolderID != g.HolderID {
		return ErrGuardLost
	}
	if g.PID > 0 && st.HolderPID > 0 && st.HolderPID != g.PID {
		return ErrGuardLost
	}
	return nil
}

// NoopGuard explicitly skips guard checks in isolated unit tests only.
// Production wiring must always use RegistryGuard.
type NoopGuard struct{}

// HoldsWriteLock always succeeds.
func (NoopGuard) HoldsWriteLock(context.Context) error { return nil }

// NewRegistryGuard builds a guard checker for the current process.
func NewRegistryGuard(store registry.ServingStore, holderID string) RegistryGuard {
	return RegistryGuard{Store: store, HolderID: holderID, PID: os.Getpid()}
}

// GuardLostFatal reports whether err requires stopping the scheduler loop.
func GuardLostFatal(err error) bool {
	return errors.Is(err, ErrGuardLost)
}

func guardError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrGuardLost) {
		return err
	}
	return fmt.Errorf("controller guard: %w", err)
}
