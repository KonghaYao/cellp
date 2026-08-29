package orch

import (
	"os"
	"strconv"
)

const defaultWorkerCount = 1

// WorkerCount returns orchestrator worker goroutines from CELLP_ORCH_WORKERS (default 1).
func WorkerCount() int {
	if v := os.Getenv("CELLP_ORCH_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultWorkerCount
}
