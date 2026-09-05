package contract

// BackgroundMode guards scale-to-zero and multi-replica background workloads.
type BackgroundMode string

const (
	BackgroundModeUnknown          BackgroundMode = "unknown"
	BackgroundModeNone             BackgroundMode = "none"
	BackgroundModeResidentRequired BackgroundMode = "resident_required"
)

// ServingPolicy is the operator-facing desired bounds for a version fleet.
type ServingPolicy struct {
	Revision        int64          `json:"revision"`
	MinReplicas     int            `json:"min_replicas"`
	MaxReplicas     int            `json:"max_replicas"`
	Priority        int            `json:"priority"`
	BackgroundMode  BackgroundMode `json:"background_mode"`
	ElasticEnrolled bool           `json:"elastic_enrolled"`
}

// ServingDesire is the autoscaler output (sole writer: autoscaler).
type ServingDesire struct {
	DesiredReplicas int    `json:"desired_replicas"`
	Generation      int64  `json:"generation"`
	Reason          string `json:"reason"`
}

// ServingPhase is a derived view (not persisted as Version.status).
type ServingPhase string

const (
	ServingCold      ServingPhase = "cold"
	ServingWaking    ServingPhase = "waking"
	ServingStarting  ServingPhase = "starting"
	ServingWarm      ServingPhase = "warm"
	ServingDegraded  ServingPhase = "degraded"
)
