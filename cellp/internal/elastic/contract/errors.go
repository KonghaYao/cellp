package contract

// ReasonCode is a low-cardinality error/reason enum for APIs and logs.
type ReasonCode string

const (
	ReasonVersionNotFound      ReasonCode = "version_not_found"
	ReasonVersionNotReady      ReasonCode = "version_not_ready"
	ReasonVersionNotArchived   ReasonCode = "version_not_archived"
	ReasonCapacityExhausted    ReasonCode = "capacity_exhausted"
	ReasonSnapshotInvalid      ReasonCode = "snapshot_invalid"
	ReasonSnapshotUnavailable  ReasonCode = "snapshot_unavailable"
	ReasonGenerationStale      ReasonCode = "generation_stale"
	ReasonElasticDisabled      ReasonCode = "elastic_disabled"
	ReasonBackgroundUnknown    ReasonCode = "background_unknown"
	ReasonAuthFailed           ReasonCode = "auth_failed"
	ReasonReplayRejected       ReasonCode = "replay_rejected"
	ReasonColdActivating       ReasonCode = "cold_activating"
	ReasonRequestTooLarge      ReasonCode = "request_too_large"
)

// String returns the wire form.
func (r ReasonCode) String() string { return string(r) }
