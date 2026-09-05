# Environment variables

Two different layers:

1. **Worker env** — per version, what your `fetch` handler sees.
2. **cellpd env** — how the control plane, gateway, and orchestrator run.

## Worker env (per version)

Env is **per version**, not “Preview / Production / Development” like Vercel.

### Sources (lowest → highest)

| Source | Meaning |
|--------|---------|
| wrangler `vars` | Shipped in the bundle |
| **overrides** | Set via API or Dashboard Settings |
| platform | `PROJECT_ID`, `VERSION_ID` — **read-only** |

`GET /v1/projects/{p}/versions/{v}/env` returns each key with `source`: `wrangler` | `override` | `platform`.

Keys prefixed `CELLP_` / `CELLD_` and platform names are rejected on `POST /versions` and `PUT …/env` (max **64** keys, **8192** bytes per value).

### Set overrides

```bash
curl -sS -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$CELLP_URL/v1/projects/my-shop/versions/v1/env" \
  -d '{"vars":{"API_ORIGIN":"https://api.example.com"}}'
```

On a **ready** version this restarts celld so the Worker sees the new `env`. You cannot PUT platform keys.

You can also pass `env` on `POST /versions` so CI injects secrets at deploy time.

### Inheritance

A child version **copies parent overrides** (not platform keys). Then you can change them independently.

### Secrets

The API stores override values as strings. There is no sealed vault inside cellp.

For production secrets:

- Inject from CI / an external vault at `POST /versions`
- Or put a secret manager **in front** of cellp and never persist high-value keys in the registry

Do not treat Dashboard env as a password manager.

## cellpd configuration (operator)

Set on the **cellpd** process (`cellp serve`, Compose, systemd). See also [CLI](/reference/cli#cellp-serve) and [Self-hosting](/guides/self-hosting).

### Tokens & registry

| Variable | Purpose |
|----------|---------|
| `CELLP_DEPLOY_TOKEN` | CI create-version |
| `CELLP_ADMIN_TOKEN` | Admin API + Dashboard |
| `PLATFORM_TOKEN` | Legacy fallback when the specific token above is unset |
| `CELLP_REGISTRY_DB` | SQLite registry path |

### Ports & URLs

| Variable | Default | Purpose |
|----------|---------|---------|
| `PLATFORM_PORT` | `8790` | REST API |
| `GATEWAY_PORT` | `8787` | User traffic |
| `GATEWAY_URL` | `http://127.0.0.1:8787` | Public gateway origin for generated URLs |
| `CELLP_GATEWAY_VERIFY_URL` | — | Optional internal probe base (HTTP) after deploy |

### Ingress (Host routing)

| Variable | Purpose |
|----------|---------|
| `CELLP_INGRESS_BASE_DOMAIN` | DNS suffix for preview/prod Hosts (e.g. `ingress.local`, `lvh.me`) |
| `CELLP_PUBLIC_SCHEME_PREVIEW` | `http` or `https` in `preview_url` |
| `CELLP_PUBLIC_SCHEME_PROD` | Scheme for `prod_url` |
| `CELLP_PREVIEW_URL_TEMPLATE` | Optional override template |

Dashboard: `VITE_CELLP_INGRESS_BASE_DOMAIN` must match `CELLP_INGRESS_BASE_DOMAIN`.

### Object storage

| Variable | Purpose |
|----------|---------|
| `S3_ENDPOINT`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` | RustFS / S3 API |
| `CELLP_ARTIFACTS_BUCKET` | Artifact uploads (`cellp-artifacts`) |
| `CELLD_BUCKET` | Base prefix for per-version celld data |
| `OFFSHOOT_STORE` | offshoot metadata (`s3://cellp-offshoot` in production) |
| `ARTIFACTS_DIR` | Local staging when not using pure S3 fetch |

### Deploy orchestrator

By default, deploy is **fail-closed**: if offshoot fork/checkpoint/export, D1 import/branch, or KV/R2/Queue branch fails, the version becomes `failed` and must not serve preview traffic.

| Variable | Purpose |
|----------|---------|
| `CELLP_LENIENT_DEPLOY=1` | **Local debug only** — warn and continue toward `ready` |
| `CELLP_QUEUE_MAX` | Deep health / accept threshold when queue is full (default 10000) |
| `CELLP_ORCH_WORKERS` | Parallel deploy workers (default 1) |

### Archive & promote hygiene

| Variable | Purpose |
|----------|---------|
| `CELLP_ARCHIVE_IDLE` | Idle time before archive candidate |
| `CELLP_ARCHIVE_GRACE` | Grace after promote before archive |
| `CELLP_ARCHIVE_REAPER=0` | Disable idle archive ticker |
| `CELLP_ROLLBACK_KEEP` | Pin previous prod after promote (see [Archive](/concepts/archive)) |

### Elastic serving (planned default off)

| Variable | Purpose |
|----------|---------|
| `CELLP_ELASTIC_RUNTIME` | When truthy, enables elastic replica machinery (**not** the default v1 path) |

## Compared to Vercel

| Vercel | cellp |
|--------|--------|
| Project / Preview / Production env | One map per **version** |
| Encrypted Dashboard secrets | Plain strings + CI injection |
| Redeploy to apply | `PUT …/env` restarts that version |
