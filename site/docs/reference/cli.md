# CLI

The **`cellp`** binary is the operator entry point. Production runs **`cellp serve`** (cellpd + gateway). Laptops use **`cellp dev`** (in-process S3, no Docker).

Install: [Install](/guides/install) or `curl -fsSL …/scripts/install.sh | sh`.

## Commands

| Command | Purpose |
|---------|---------|
| `cellp dev` | Local control plane on `127.0.0.1` — API **8790**, gateway **8787**, path-style S3 (default **19000**) |
| `cellp serve` | Run cellpd from environment (Docker Compose, systemd, bare metal) |
| `cellp doctor` | Check `celld`, `offshoot`, optional `esbuild`, and port availability |
| `cellp version` | Print build version |

### `cellp dev`

```bash
cellp dev
cellp dev --project my-shop
cellp dev --no-deploy          # platform only; deploy via API or Dashboard
cellp dev --home ~/.cellp      # data directory (default ~/.cellp)
```

If the current directory has `wrangler.jsonc`, dev uploads and creates a **root** version (same artifact layout as CI). Requires **`celld`** on `PATH` (bundled next to `cellp` after install, or build from the `celld/` submodule).

Environment overrides used by dev (also apply when you run serve with the same vars):

| Variable | Default (dev) | Meaning |
|----------|---------------|---------|
| `CELLP_HOME` | `~/.cellp` | State root |
| `PLATFORM_PORT` | `8790` | cellpd API |
| `GATEWAY_PORT` | `8787` | User traffic |
| `CELLP_S3_ADDR` | `127.0.0.1:19000` | In-process artifact S3 |
| `CELLP_DEPLOY_TOKEN` / `CELLP_ADMIN_TOKEN` | `dev-local-token` | Bearer tokens |

See [cellp dev](/guides/dev) for the full laptop workflow.

### `cellp serve`

Loads configuration from the environment and starts cellpd (REST API, gateway, orchestrator). Same variables as [Self-hosting](/guides/self-hosting) and [Environment variables](/guides/environment-variables).

Common cellpd variables:

| Variable | Purpose |
|----------|---------|
| `CELLP_DEPLOY_TOKEN` / `CELLP_ADMIN_TOKEN` | Bearer tokens ([Auth](/reference/auth)); `PLATFORM_TOKEN` legacy fallback |
| `CELLP_REGISTRY_DB` | SQLite registry path |
| `GATEWAY_URL` | Public gateway origin (Host routing base) |
| `CELLP_INGRESS_BASE_DOMAIN` | Ingress Host suffix |
| `PLATFORM_PORT` / `GATEWAY_PORT` | Listen ports |
| `S3_ENDPOINT`, `AWS_*` / `RUSTFS_*` | RustFS (artifacts + celld buckets) |
| `CELLP_ARTIFACTS_BUCKET` | Allowed artifact bucket name |
| `ARTIFACTS_DIR` | Local artifact cache when using file layout |
| `OFFSHOOT_STORE` | offshoot metadata directory or `s3://…` |

Orchestrator tuning (optional):

| Variable | Purpose |
|----------|---------|
| `CELLP_QUEUE_MAX` | Deep health fails when deploy queue exceeds this |
| `CELLP_ORCH_WORKERS` | Parallel deploy workers (default 1) |
| `CELLP_LENIENT_DEPLOY=1` | Local-only: warn on branch/offshoot failures ([Environment variables](/guides/environment-variables)) |
| `CELLP_ARCHIVE_IDLE` / `CELLP_ARCHIVE_GRACE` | Idle archive and post-promote pin ([Archive](/concepts/archive)) |
| `CELLP_ARCHIVE_REAPER=0` | Disable idle archive ticker |
| `CELLP_ELASTIC_RUNTIME` | Elastic serving **off** by default ([Limits](/reference/limits)) |

Worker env per version is **not** configured here — use the API or [Environment variables](/guides/environment-variables).

## Related

- [REST API](/reference/api) · [OpenAPI in repo](https://github.com/KonghaYao/cellp/blob/main/cellp/api/openapi.yaml)
- [Deploy from CI](/guides/ci)
