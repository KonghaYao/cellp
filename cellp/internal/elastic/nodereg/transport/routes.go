package transport

const apiVersion = "v1"

// RoutePrefix is the controller-internal node registration surface.
const RoutePrefix = "/" + apiVersion + "/internal/elastic-node-registry"

const (
	routeActivate  = RoutePrefix + "/activate"
	routeHeartbeat = RoutePrefix + "/heartbeat"
	routeCordon    = RoutePrefix + "/cordon"
	routeRelease   = RoutePrefix + "/release"
	routeStatus    = RoutePrefix + "/status"
)
