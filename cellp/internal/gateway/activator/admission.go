package activator

import "sync"

// FlightLimiter bounds concurrent activation flights (EnsureCapacity + poll)
// independently of waiter request/byte budgets.
type FlightLimiter struct {
	mu sync.Mutex

	globalMax   int
	globalInUse int

	perVersionMax int
	perVersion    map[string]int
}

// NewFlightLimiter returns a limiter with global and per-version flight caps.
func NewFlightLimiter(globalMax, perVersionMax int) *FlightLimiter {
	return &FlightLimiter{
		globalMax:     globalMax,
		perVersionMax: perVersionMax,
		perVersion:    make(map[string]int),
	}
}

// TryAcquire reserves one activation flight slot or returns false when full.
func (f *FlightLimiter) TryAcquire(projectID, versionID string) bool {
	if f == nil {
		return false
	}
	if f.globalMax <= 0 || f.perVersionMax <= 0 {
		return false
	}
	key := versionKey(projectID, versionID)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.globalInUse >= f.globalMax || f.perVersion[key] >= f.perVersionMax {
		return false
	}
	f.globalInUse++
	f.perVersion[key]++
	return true
}

// Release frees a previously acquired activation flight slot.
func (f *FlightLimiter) Release(projectID, versionID string) {
	if f == nil {
		return
	}
	key := versionKey(projectID, versionID)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.globalInUse > 0 {
		f.globalInUse--
	}
	if n := f.perVersion[key]; n > 0 {
		f.perVersion[key] = n - 1
		if f.perVersion[key] == 0 {
			delete(f.perVersion, key)
		}
	}
}
