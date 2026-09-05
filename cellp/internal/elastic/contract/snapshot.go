package contract

// IngressBinding is a host/port → version mapping entry in a snapshot.
type IngressBinding struct {
	Role       string `json:"role"`
	Host       string `json:"host"`
	ListenPort int    `json:"listen_port"`
	ProjectID  string `json:"project_id"`
	VersionID  string `json:"version_id"`
}

// RouteSnapshot is the immutable Gateway read model (revision must increase monotonically).
type RouteSnapshot struct {
	Revision       int64            `json:"revision"`
	PolicyRevision int64            `json:"policy_revision"`
	Bindings       []IngressBinding `json:"bindings"`
	EndpointSets   []EndpointSet    `json:"endpoint_sets"`
}
