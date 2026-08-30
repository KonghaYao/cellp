# Observability

v1 is **Prometheus + process logs**. There is no request tail UI, no `wrangler tail`, and no OpenTelemetry product. That is deferred, not forgotten in a sprint.

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

## Logs

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

## Grafana / Loki

Bring your own. cellp will not ship a SaaS analytics product. Point Grafana at `/metrics` and ship logs to Loki/ELK however you already do.

## Promote windows

Watch gateway 5xx around promote. Rollback path: [Rollback](/guides/rollback).
