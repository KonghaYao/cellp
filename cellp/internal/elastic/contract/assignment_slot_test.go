package contract

import (
	"testing"
	"time"
)

func TestAssignmentOccupiesSlotSemantics(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	base := RuntimeReplica{
		ReplicaID: "r1", AssignedNodeGeneration: 1, State: ReplicaReady, ValidUntil: &future,
	}
	if !AssignmentOccupiesSlot(base, now) {
		t.Fatal("ready assignment should occupy")
	}
	stopped := base
	stopped.State = ReplicaStopped
	if AssignmentOccupiesSlot(stopped, now) {
		t.Fatal("terminal should not occupy")
	}
	expired := base
	expired.ValidUntil = &past
	if AssignmentOccupiesSlot(expired, now) {
		t.Fatal("expired should not occupy")
	}
	stale := base
	stale.AssignedNodeGeneration = 9
	if !AssignmentOccupiesSlot(stale, now) {
		t.Fatal("stale generation should fail-closed occupy until terminalized")
	}
}
