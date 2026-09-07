package transport

const apiVersion = "v1"

// RoutePrefix is the controller-internal registry relay surface.
const RoutePrefix = "/" + apiVersion + "/internal/elastic-registry-relay"

const (
	routeGetRuntimeNode                 = RoutePrefix + "/get-runtime-node"
	routeGetRuntimeReplica              = RoutePrefix + "/get-runtime-replica"
	routeValidateAgentAssignment        = RoutePrefix + "/validate-agent-assignment"
	routeValidateAgentCleanupAssignment = RoutePrefix + "/validate-agent-cleanup-assignment"
	routeListRuntimeReplicasByNode      = RoutePrefix + "/list-runtime-replicas-by-node"
	routeRecordObservation              = RoutePrefix + "/record-observation"
	routeClaimAgentCommand              = RoutePrefix + "/claim-agent-command"
	routeRenewAgentCommandLease         = RoutePrefix + "/renew-agent-command-lease"
	routeCompleteAgentCommand           = RoutePrefix + "/complete-agent-command"
	routeRecordObservationAndComplete   = RoutePrefix + "/record-observation-and-complete"
	routeWithdrawReplica                = RoutePrefix + "/withdraw-replica"
	routeTerminalizeReplica             = RoutePrefix + "/terminalize-replica"
	routeGetVersionEnv                  = RoutePrefix + "/get-version-env"
)
