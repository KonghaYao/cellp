package api_test

import (
	"testing"

	"github.com/cellp/cellp/internal/api"
)

func TestQueueMaxDefault(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "")
	if got := api.QueueMax(); got != 10000 {
		t.Fatalf("QueueMax() = %d, want 10000", got)
	}
}

func TestQueueMaxFromEnv(t *testing.T) {
	t.Setenv("CELLP_QUEUE_MAX", "500")
	if got := api.QueueMax(); got != 500 {
		t.Fatalf("QueueMax() = %d, want 500", got)
	}
}
