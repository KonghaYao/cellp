package gateway

import (
	"context"
	"net/http"

	"github.com/cellp/cellp/internal/elastic/contract"
	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/registry"
)

func (g *Gateway) elasticActivator() *activator.Activator {
	if g == nil || !contract.ElasticRuntimeEnabled() {
		return nil
	}
	if g.activator != nil {
		return g.activator
	}
	if g.store == nil {
		return nil
	}
	client := &activator.RegistryEnsureClient{Store: g.store}
	g.activator = activator.New(true, client, activator.DefaultConfig())
	return g.activator
}

// tryColdActivator returns true if the request was fully handled (e.g. 503).
func (g *Gateway) tryColdActivator(w http.ResponseWriter, r *http.Request, projectID, versionID string) bool {
	act := g.elasticActivator()
	if act == nil || !act.Enabled() {
		return false
	}
	v, err := g.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil || v == nil {
		return false
	}
	if v.Status != registry.StatusDeployReady {
		return false
	}
	lookup := func() (string, bool) {
		return g.snapshots.LookupUpstreamFromSnapshot(projectID, versionID)
	}
	if _, warm := lookup(); warm {
		return false
	}
	desiredGen := g.desiredGeneration(r.Context(), projectID, versionID)
	res := act.Admit(r.Context(), r, projectID, versionID, v.Status, desiredGen, lookup)
	if res.AllowProxy {
		return false
	}
	activator.WriteRetryResponse(w, res)
	return true
}

func (g *Gateway) desiredGeneration(ctx context.Context, projectID, versionID string) int64 {
	if g.store == nil {
		return 0
	}
	d, err := g.store.GetServingDesire(ctx, projectID, versionID)
	if err != nil || d == nil {
		return 0
	}
	return d.Generation
}
