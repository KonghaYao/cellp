package activator

// Wire reason tokens for 503 responses (low cardinality; align with docs/plans/04-activator-cold-start.md).
const (
	ReasonWakeQueueFull      = "wake_queue_full"
	ReasonWakeTimeout        = "wake_timeout"
	ReasonCapacityExhausted  = "capacity_exhausted"
	ReasonWakeRetry          = "wake_retry"
	ReasonVersionArchived    = "version_archived"
	ReasonControlUnavailable = "control_unavailable"
)

const HeaderCellpReason = "X-Cellp-Reason"
