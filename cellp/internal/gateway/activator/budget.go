package activator

import (
	"sync"
)

// Budget limits concurrent bounded waits (global + per-version).
type Budget struct {
	mu sync.Mutex

	globalMax   int
	globalInUse int

	perVersionMax int
	perVersion    map[string]int
}

// NewBudget returns a budget with the given caps (must be > 0 for enforcement).
func NewBudget(globalMax, perVersionMax int) *Budget {
	if globalMax <= 0 {
		globalMax = 256
	}
	if perVersionMax <= 0 {
		perVersionMax = 32
	}
	return &Budget{
		globalMax:     globalMax,
		perVersionMax: perVersionMax,
		perVersion:    make(map[string]int),
	}
}

func versionKey(projectID, versionID string) string {
	return projectID + "/" + versionID
}

// TryAcquire increments wait slots or returns false when full.
func (b *Budget) TryAcquire(projectID, versionID string) bool {
	if b == nil {
		return true
	}
	key := versionKey(projectID, versionID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.globalInUse >= b.globalMax {
		return false
	}
	if b.perVersion[key] >= b.perVersionMax {
		return false
	}
	b.globalInUse++
	b.perVersion[key]++
	return true
}

// Release decrements wait slots after a waiter completes.
func (b *Budget) Release(projectID, versionID string) {
	if b == nil {
		return
	}
	key := versionKey(projectID, versionID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.globalInUse > 0 {
		b.globalInUse--
	}
	if n := b.perVersion[key]; n > 0 {
		b.perVersion[key] = n - 1
		if b.perVersion[key] == 0 {
			delete(b.perVersion, key)
		}
	}
}
