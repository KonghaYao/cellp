# Observability

**Today:** Prometheus metrics on cellpd, structured logs on stdout, and per-version celld process output. **Planned (not shipped):** OTLP trace/log export and a version-scoped query API behind the same admin token — backends would be pluggable (memory, Jaeger, or your own LGTM stack).

cellp will **not** ship a SaaS analytics product or an in-tree log search engine.

## Metrics

Scrape **cellpd** at the **root** path (not under `/v1`):

```yaml
scrape_configs:
  - job_name: cellpd
    static_configs:
      - targets: ["cellpd:8790"]
    metrics_path: /metrics
```

You will see HTTP counters/histograms, orchestrator queue depth, and gateway upstream health. Metric names are defined in the `cellp` Go module (`internal/metrics`).

```bash
curl -s http://127.0.0.1:8790/metrics | head
```

## Logs (today)

| Component | Where |
|-----------|--------|
| cellpd | Docker / systemd / stdout — API, saga, archive reaper |
| celld | Per-version process — `console.log` and binding CLI |
| RustFS | Its own metrics/logs |

Archive reaper lines contain `orch: archive reaper`.

There is no `wrangler tail` equivalent. Use process logs and metrics until OTLP live tail ships.

## Health

```bash
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8790/v1/health/deep
```

Deep health is the right probe for “can I deploy?” — registry, object store, runtimes, queue. Returns **503** when the deploy queue exceeds `CELLP_QUEUE_MAX`.

`GET /v1/runtime/routes` (admin) summarizes upstreams.

## Planned observability (OTLP, not shipped)

| Layer | Contract (design) |
|-------|-------------------|
| Emit | OTLP traces + logs; `cellp.project` / `cellp.version`; Gateway `traceparent` |
| Query | cellpd facade (`context`, `traces/{id}`, template `search`) — `ADMIN_TOKEN` only |
| Live | Process stream (SSE), not OTEL |
| Backend | Operator-chosen collector (Jaeger, Grafana stack, etc.) |

Bring your own Grafana for boards. The Dashboard talks only to `:8790`.

## Promote windows

Watch gateway 5xx around promote. Rollback path: [Rollback](/guides/rollback).
