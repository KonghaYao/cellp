package transport

import (
	"time"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/registry"
)

type envelope struct {
	Scope contract.RegistryRelayScope `json:"scope"`
}

type getRuntimeReplicaRequest struct {
	Scope     contract.RegistryRelayScope `json:"scope"`
	ReplicaID string                      `json:"replica_id"`
}

type validateAssignmentRequest struct {
	Scope    contract.RegistryRelayScope `json:"scope"`
	CmdScope contract.CommandScope       `json:"cmd_scope"`
	Now      time.Time                   `json:"now"`
}

type validateCleanupRequest struct {
	Scope    contract.RegistryRelayScope `json:"scope"`
	CmdScope contract.CommandScope       `json:"cmd_scope"`
}

type recordObservationRequest struct {
	Scope contract.RegistryRelayScope `json:"scope"`
	Obs   registry.ReplicaObservation `json:"observation"`
}

type claimCommandRequest struct {
	Scope   contract.RegistryRelayScope `json:"scope"`
	Command registry.AgentCommand       `json:"command"`
}

type renewCommandRequest struct {
	Scope   contract.RegistryRelayScope `json:"scope"`
	Command registry.AgentCommand       `json:"command"`
	Expiry  time.Time                   `json:"expiry"`
}

type completeCommandRequest struct {
	Scope   contract.RegistryRelayScope `json:"scope"`
	Command registry.AgentCommand       `json:"command"`
}

type recordAndCompleteRequest struct {
	Scope   contract.RegistryRelayScope `json:"scope"`
	Obs     registry.ReplicaObservation `json:"observation"`
	Command registry.AgentCommand       `json:"command"`
}

type withdrawRequest struct {
	Scope      contract.RegistryRelayScope `json:"scope"`
	ReplicaID  string                      `json:"replica_id"`
	ProjectID  string                      `json:"project_id"`
	VersionID  string                      `json:"version_id"`
	Generation int64                       `json:"generation"`
}

type terminalizeRequest struct {
	Scope      contract.RegistryRelayScope `json:"scope"`
	ReplicaID  string                      `json:"replica_id"`
	Generation int64                       `json:"generation"`
	State      contract.ReplicaState       `json:"state"`
}

type getVersionEnvRequest struct {
	Scope     contract.RegistryRelayScope `json:"scope"`
	ProjectID string                      `json:"project_id"`
	VersionID string                      `json:"version_id"`
}

type versionEnvResponse struct {
	Env map[string]string `json:"env"`
}

type runtimeNodeResponse struct {
	Node *wireRuntimeNode `json:"node"`
}

type runtimeReplicaResponse struct {
	Replica *wireRuntimeReplica `json:"replica"`
}

type replicasResponse struct {
	Replicas []wireRuntimeReplica `json:"replicas"`
}

type claimResponse struct {
	Claim registry.AgentCommandClaim `json:"claim"`
}
