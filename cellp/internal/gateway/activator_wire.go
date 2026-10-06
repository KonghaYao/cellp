package gateway

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/cellp/cellp/internal/gateway/activator"
	"github.com/cellp/cellp/internal/metrics"
	"github.com/cellp/cellp/internal/registry"
)

// ConfigureElasticActivator installs the request-path writer before listeners start.
func (g *Gateway) ConfigureElasticActivator(client activator.EnsureCapacityClient, cfg activator.Config) error {
	if g == nil {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	g.activatorOnce.Do(func() {
		g.activator = activator.New(true, client, cfg)
	})
	return nil
}

// ShutdownElasticActivator cancels fast-fail wake work and waits for quiescence.
func (g *Gateway) ShutdownElasticActivator(ctx context.Context) error {
	if g == nil || g.activator == nil {
		return nil
	}
	return g.activator.Shutdown(ctx)
}

func (g *Gateway) elasticActivator() *activator.Activator {
	if g == nil {
		return nil
	}
	return g.activator
}

func (g *Gateway) writeActivationResponse(w http.ResponseWriter, result activator.AdmitResult) {
	metrics.RecordActivatorResult(result.Reason, false)
	activator.WriteRetryResponse(w, result)
}

// tryElasticReadyProxy routes enrolled ready traffic exclusively through the immutable snapshot.
func (g *Gateway) tryElasticReadyProxy(w http.ResponseWriter, r *http.Request, binding *registry.IngressBinding, projectID, versionID string) bool {
	upstream, ok := g.snapshots.LookupElasticUpstream(projectID, versionID)
	if ok {
		return g.proxySnapshotUpstream(w, r, binding, projectID, versionID, upstream)
	}
	policy, err := g.store.GetServingPolicy(r.Context(), projectID, versionID)
	if err != nil {
		g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
		return true
	}
	if policy == nil || !policy.ElasticEnrolled {
		return false
	}
	version, err := g.store.GetVersion(r.Context(), projectID, versionID)
	if err != nil || version == nil {
		g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
		return true
	}
	return g.runElasticColdWake(w, r, binding, version)
}

func (g *Gateway) proxySnapshotUpstream(w http.ResponseWriter, r *http.Request, binding *registry.IngressBinding, projectID, versionID, upstream string) bool {
	host, portRaw, err := net.SplitHostPort(upstream)
	port, portErr := strconv.Atoi(portRaw)
	if err != nil || portErr != nil || host == "" || port <= 0 || port > 65535 {
		g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
		return true
	}
	g.withIngressTrace(w, r, binding, projectID, versionID, func(rw http.ResponseWriter, req *http.Request) {
		g.proxyIngress(rw, req, &registry.Route{
			ProjectID: projectID, VersionID: versionID, Active: true,
			UpstreamHost: host, UpstreamPort: port,
		}, binding, projectID, versionID, g.elasticDialColdMiss(projectID, versionID))
	})
	return true
}

// elasticDialColdMiss turns a pre-dispatch connect failure into the Gateway's existing
// cold-start contract for an enrolled version: the snapshot named an endpoint whose
// process is already gone (a retired replica), which is a cold miss rather than a bad
// gateway. The request is never forwarded again — a mutation must not be replayed — the
// caller gets the activation response (503 + Retry-After) and retries on its own.
//
// The stale snapshot entry is not consulted to decide whether the version is warm: that
// entry is the very thing the refused connect just disproved, so asking it again would
// answer "warm" and fall through to 502. Capacity is refreshed through the same ensure
// the cold path uses, and anything after dispatch keeps the 502 path.
func (g *Gateway) elasticDialColdMiss(projectID, versionID string) func(http.ResponseWriter, *http.Request, error) bool {
	return func(w http.ResponseWriter, r *http.Request, proxyErr error) bool {
		act := g.elasticActivator()
		if !preDispatchDialFailure(proxyErr) || !act.Enabled() {
			return false
		}
		version, err := g.store.GetVersion(r.Context(), projectID, versionID)
		if err != nil || version == nil ||
			version.Status == registry.StatusArchived || version.Status == registry.StatusFailed {
			return false
		}
		policy, err := g.store.GetServingPolicy(r.Context(), projectID, versionID)
		if err != nil || policy == nil || !policy.ElasticEnrolled {
			return false
		}
		desired, err := g.store.GetServingDesire(r.Context(), projectID, versionID)
		if err != nil {
			g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
			return true
		}
		desiredGen := int64(0)
		if desired != nil {
			desiredGen = desired.Generation
		}
		res := act.WakeAfterDeadEndpoint(projectID, versionID, version.Status, desiredGen, func() (string, bool) {
			return g.snapshots.LookupElasticUpstream(projectID, versionID)
		})
		if res.AllowProxy {
			return false
		}
		log.Printf("gateway cold miss project=%q version=%q status=%s class=%s reason=%s",
			projectID, versionID, version.Status, classifyProxyError(proxyErr), res.Reason)
		g.writeActivationResponse(w, res)
		return true
	}
}

// preDispatchDialFailure reports a connect failure: it happened before any request byte
// reached an upstream, so nothing can have executed there. Failures after dispatch
// (reset mid-response, timeout) are not this.
func preDispatchDialFailure(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// tryColdActivator returns true if the request was fully handled.
func (g *Gateway) tryColdActivator(w http.ResponseWriter, r *http.Request, binding *registry.IngressBinding, version *registry.Version) bool {
	if version == nil || version.Status != registry.StatusDeployReady {
		return false
	}
	return g.runElasticColdWake(w, r, binding, version)
}

func elasticColdWakeEligible(version *registry.Version) bool {
	if version == nil || version.ReadyAt == nil {
		return false
	}
	return version.Status == registry.StatusDeployReady || version.Status == registry.StatusReady
}

// runElasticColdWake bumps desired capacity and polls the immutable snapshot for a warm endpoint.
func (g *Gateway) runElasticColdWake(w http.ResponseWriter, r *http.Request, binding *registry.IngressBinding, version *registry.Version) bool {
	act := g.elasticActivator()
	if act == nil || !act.Enabled() || !elasticColdWakeEligible(version) {
		return false
	}
	projectID, versionID := version.ProjectID, version.ID
	policy, err := g.store.GetServingPolicy(r.Context(), projectID, versionID)
	if err != nil {
		g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
		return true
	}
	if policy == nil || !policy.ElasticEnrolled {
		return false
	}
	lookup := func() (string, bool) {
		return g.snapshots.LookupElasticUpstream(projectID, versionID)
	}
	if _, warm := lookup(); warm {
		return false
	}
	desired, err := g.store.GetServingDesire(r.Context(), projectID, versionID)
	if err != nil {
		g.writeActivationResponse(w, activator.AdmitResult{Reason: activator.ReasonControlUnavailable})
		return true
	}
	desiredGen := int64(0)
	if desired != nil {
		desiredGen = desired.Generation
	}
	res := act.Admit(r.Context(), r, projectID, versionID, version.Status, desiredGen, lookup)
	if res.AllowProxy && res.Upstream != "" {
		metrics.RecordActivatorResult("", true)
		return g.proxySnapshotUpstream(w, r, binding, projectID, versionID, res.Upstream)
	}
	if res.AllowProxy {
		return false
	}
	g.writeActivationResponse(w, res)
	return true
}
