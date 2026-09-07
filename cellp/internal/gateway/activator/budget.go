package activator

import "sync"

// Budget limits concurrent bounded waits by request count and declared body bytes.
type Budget struct {
	mu sync.Mutex

	globalMax      int
	globalInUse    int
	globalBytesMax int64
	globalBytes    int64

	perVersionMax      int
	perVersionBytesMax int64
	perVersion         map[string]int
	perVersionBytes    map[string]int64
}

// NewBudget preserves the count-only constructor used by compatibility tests.
func NewBudget(globalMax, perVersionMax int) *Budget {
	return NewBudgetWithBytes(globalMax, perVersionMax, 1<<62, 1<<62)
}

// NewBudgetWithBytes returns a budget with count and byte caps.
func NewBudgetWithBytes(globalMax, perVersionMax int, globalBytesMax, perVersionBytesMax int64) *Budget {
	return &Budget{
		globalMax:          globalMax,
		perVersionMax:      perVersionMax,
		globalBytesMax:     globalBytesMax,
		perVersionBytesMax: perVersionBytesMax,
		perVersion:         make(map[string]int),
		perVersionBytes:    make(map[string]int64),
	}
}

func versionKey(projectID, versionID string) string {
	return projectID + "/" + versionID
}

// TryAcquire increments a count-only wait slot or returns false when full.
func (b *Budget) TryAcquire(projectID, versionID string) bool {
	return b.TryAcquireBytes(projectID, versionID, 0)
}

// TryAcquireBytes reserves one waiter and its declared body bytes atomically.
func (b *Budget) TryAcquireBytes(projectID, versionID string, bodyBytes int64) bool {
	if b == nil {
		return false
	}
	if bodyBytes < 0 {
		return false
	}
	key := versionKey(projectID, versionID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.globalMax <= 0 || b.perVersionMax <= 0 || b.globalBytesMax <= 0 || b.perVersionBytesMax <= 0 {
		return false
	}
	if b.globalInUse >= b.globalMax || b.perVersion[key] >= b.perVersionMax {
		return false
	}
	if bodyBytes > b.globalBytesMax-b.globalBytes || bodyBytes > b.perVersionBytesMax-b.perVersionBytes[key] {
		return false
	}
	b.globalInUse++
	b.globalBytes += bodyBytes
	b.perVersion[key]++
	b.perVersionBytes[key] += bodyBytes
	return true
}

// Release decrements a count-only wait slot.
func (b *Budget) Release(projectID, versionID string) {
	b.ReleaseBytes(projectID, versionID, 0)
}

// ReleaseBytes releases a waiter and its previously reserved body bytes.
func (b *Budget) ReleaseBytes(projectID, versionID string, bodyBytes int64) {
	if b == nil || bodyBytes < 0 {
		return
	}
	key := versionKey(projectID, versionID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.globalInUse > 0 {
		b.globalInUse--
	}
	if bodyBytes <= b.globalBytes {
		b.globalBytes -= bodyBytes
	} else {
		b.globalBytes = 0
	}
	if n := b.perVersion[key]; n > 0 {
		b.perVersion[key] = n - 1
		if b.perVersion[key] == 0 {
			delete(b.perVersion, key)
		}
	}
	if n := b.perVersionBytes[key]; bodyBytes <= n {
		b.perVersionBytes[key] = n - bodyBytes
		if b.perVersionBytes[key] == 0 {
			delete(b.perVersionBytes, key)
		}
	} else {
		delete(b.perVersionBytes, key)
	}
}
