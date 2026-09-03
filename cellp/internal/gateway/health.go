package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cellp/cellp/internal/health"
)

func (g *Gateway) handleHealthDeep(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]interface{}{}
	overall := "ok"
	code := http.StatusOK

	regStart := time.Now()
	if err := g.store.Ping(ctx); err != nil {
		checks["registry"] = health.CheckResult{Name: "registry", Status: "down", Detail: err.Error()}
		overall = "degraded"
		code = http.StatusServiceUnavailable
	} else {
		checks["registry"] = health.CheckResult{Name: "registry", Status: "ok", LatencyMs: time.Since(regStart).Milliseconds()}
	}

	routes, err := g.store.ListAllActiveRoutes(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	checks["routes"] = map[string]int{"active": len(routes)}

	if len(routes) > 0 {
		route := routes[0]
		host := route.UpstreamHost
		if host == "" {
			host = "127.0.0.1"
		}
		upstream := health.ProbeCelldHTTP(ctx, fmt.Sprintf("http://%s:%d/.well-known/celld/health", host, route.UpstreamPort))
		upstream.Name = "sample_upstream"
		checks["sample_upstream"] = upstream
		if upstream.Status == "down" {
			overall = "degraded"
			code = http.StatusServiceUnavailable
		}
	}

	writeJSON(w, code, map[string]interface{}{
		"status": overall,
		"checks": checks,
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
