package scheduler

import (
	"sync/atomic"
)

// PlacementDegraded is true when the scheduler placed on a node that already hosts
// the same version (anti-affinity violation), e.g. single-node dev.
var PlacementDegraded atomic.Bool

// SetPlacementDegraded records anti-affinity violation for the current tick.
func SetPlacementDegraded(v bool) {
	PlacementDegraded.Store(v)
}
