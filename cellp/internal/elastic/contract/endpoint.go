package contract

import "time"

// EndpointState controls snapshot inclusion and new request admission.
type EndpointState string

const (
	EndpointReady    EndpointState = "ready"
	EndpointDraining EndpointState = "draining"
)

// Endpoint is a routable upstream for Gateway snapshot building.
type Endpoint struct {
	ReplicaID string        `json:"replica_id"`
	Address   string        `json:"address"`
	State     EndpointState `json:"state"`
	ValidUntil *time.Time   `json:"valid_until,omitempty"`
}

// EndpointSet is ready endpoints for one version binding key.
type EndpointSet struct {
	ProjectID string     `json:"project_id"`
	VersionID string     `json:"version_id"`
	Endpoints []Endpoint `json:"endpoints"`
}
