package orch_test

import (
	"testing"

	"github.com/cellp/cellp/internal/orch"
)

func TestWorkerCountDefault(t *testing.T) {
	t.Setenv("CELLP_ORCH_WORKERS", "")
	if got := orch.WorkerCount(); got != 1 {
		t.Fatalf("WorkerCount() = %d, want 1", got)
	}
}

func TestWorkerCountFromEnv(t *testing.T) {
	t.Setenv("CELLP_ORCH_WORKERS", "4")
	if got := orch.WorkerCount(); got != 4 {
		t.Fatalf("WorkerCount() = %d, want 4", got)
	}
}

func TestWorkerCountInvalidEnv(t *testing.T) {
	t.Setenv("CELLP_ORCH_WORKERS", "nope")
	if got := orch.WorkerCount(); got != 1 {
		t.Fatalf("WorkerCount() = %d, want 1 for invalid env", got)
	}
}
