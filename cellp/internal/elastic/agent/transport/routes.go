package transport

import "github.com/cellp/cellp/internal/elastic/contract"

const apiVersion = "v1"

// Internal route prefix (not exposed on public gateway).
const routePrefix = "/" + apiVersion + "/internal/elastic-agent"

const (
	routeStart = routePrefix + "/start"
	routeProbe = routePrefix + "/probe"
	routeStop  = routePrefix + "/stop"
	routeDrain = routePrefix + "/drain"
	routeList  = routePrefix + "/list"
)

func actionForRoute(path string) (contract.LifecycleAction, bool) {
	switch path {
	case routeStart:
		return contract.ActionStartReplica, true
	case routeProbe:
		return contract.ActionProbeReplica, true
	case routeStop:
		return contract.ActionStopReplica, true
	case routeDrain:
		return contract.ActionDrainReplica, true
	case routeList:
		return contract.ActionListReplicas, true
	default:
		return "", false
	}
}
