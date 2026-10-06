# Observability

**Today:** Prometheus metrics on cellpd, structured logs on stdout, per-version celld process output, and **AD-14 telemetry** (OTLP export + version-scoped query facade). Backends are pluggable: `memory` (default for tests), `lgtm` (local stack), or `jaeger`.

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

Live tail uses the query facade `GET .../telemetry/logs/stream` (SSE) or process logs. There is no `wrangler tail` protocol.

## Telemetry query facade (AD-14)

Admin token only (`CELLP_ADMIN_TOKEN`). All paths under:

`/v1/projects/{project}/versions/{version}/telemetry/`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `context` | Backend, enabled, optional Grafana deep link |
| GET | `traces/{trace_id}` | Trace tree + correlated logs |
| POST | `search` | Template search (`slow`, `error`, `status`, `body`, `request_id`) |
| GET | `logs/stream` | Live celld stdout (SSE) |

```bash
export TOKEN=dev-local-token
curl -s -H "Authorization: Bearer $TOKEN" \
  http://127.0.0.1:8790/v1/projects/demo-app/versions/v1/telemetry/context | jq

curl -s -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"template":"slow","start":"2026-01-01T00:00:00Z","end":"2026-12-31T23:59:59Z","limit":20,"slow_ms":500}' \
  http://127.0.0.1:8790/v1/projects/demo-app/versions/v1/telemetry/search | jq
```

Dashboard: project → Storage → version → **Telemetry** tab.

### Local LGTM stack

```bash
./dev/scripts/up.sh --profile otel
# Sets CELLP_OTEL_BACKEND=lgtm, collector :4318, Grafana :3000
```

## Health

```bash
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8790/v1/health/deep
```

Deep health is the right probe for “can I deploy?” — registry, object store, runtimes, queue. Returns **503** when the deploy queue exceeds `CELLP_QUEUE_MAX`.

`GET /v1/runtime/routes` (admin) summarizes upstreams.

## Backend selection

| `CELLP_OTEL_BACKEND` | Use |
|----------------------|-----|
| `none` | Default — no OTLP ingest |
| `memory` | In-process ring buffer — tests and local dev |
| `lgtm` | Collector + Tempo + Loki + Grafana (`up.sh --profile otel`) |
| `jaeger` | Jaeger all-in-one OTLP |

Bring your own Grafana for advanced boards via `context.deep_link`. The Dashboard talks only to `:8790`.

## Promote windows

Watch gateway 5xx around promote. Rollback path: [Rollback](/guides/rollback).
