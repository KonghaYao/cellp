package orch

import (
	"log"
	"os"
	"sync"
)

var lenientStrictConflictOnce sync.Once

// deployFailClosed reports whether offshoot/D1 errors abort deploy (default true).
func deployFailClosed() bool {
	lenient := os.Getenv("CELLP_LENIENT_DEPLOY") == "1"
	if lenient {
		if os.Getenv("CELLP_STRICT_OFFSHOOT_FORK") == "1" {
			lenientStrictConflictOnce.Do(func() {
				log.Printf("orch: CELLP_LENIENT_DEPLOY=1 overrides deprecated CELLP_STRICT_OFFSHOOT_FORK")
			})
		}
		return false
	}
	return true
}
