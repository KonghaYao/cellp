# Observability

**Architecture (authoritative):** [OTEL-OBSERVABILITY](https://github.com/KonghaYao/cellp/blob/main/docs/plans/OTEL-OBSERVABILITY.md) · AD-14.

v1 ships **Prometheus + per-version process logs**. Request tail, the query facade, and a pluggable OTLP backend are **designed, not implemented**.

cellp will **not** ship a SaaS analytics product or an in-tree search engine. Production search is an optional Tempo / Loki / Grafana stack behind a version-scoped API.

## Metrics

Scrape cellpd:

```yaml
scrape_configs:
  - job_name: cellpd
    static_configs:
      - targets: ["cellpd:8790"]
    metrics_path: /metrics
```

You will see HTTP counters/histograms, orchestrator queue depth, and gateway upstream health. Exact names live in `cellp/internal/metrics`.

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

## Health

```bash
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8790/v1/health/deep
```

Deep health is the right probe for “can I deploy?” — registry, object store, runtimes, queue.

`GET /v1/runtime/routes` (admin) summarizes upstreams.

## AD-14 (not shipped)

| Layer | Contract |
|-------|----------|
| Emit | OTLP traces + logs; `cellp.project` / `cellp.version`; Gateway `traceparent` |
| Query | cellpd facade (`context`, `traces/{id}`, template `search`) — `ADMIN_TOKEN` only |
| Live | Process stream (SSE), not OTEL |
| Backend | `none` (default) · `memory` · `jaeger` · `lgtm` / `lgtm-prod` |

Bring your own Grafana for boards. Dashboard talks only to `:8790`.

## Promote windows

Watch gateway 5xx around promote. Rollback path: [Rollback](/guides/rollback).
