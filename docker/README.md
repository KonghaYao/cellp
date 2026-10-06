# Docker deployment

Product guide: **[Self-hosting](https://konghayao.github.io/cellp/guides/self-hosting)**.

Single-machine stack: **RustFS** (S3) + **cellpd** (API `:8790`, Gateway `:8787`).  
Per-version **celld** runtimes are spawned by cellpd at deploy time — they are not separate compose services.

## Quick start

```bash
# Build locally (requires celld submodule)
git submodule update --init celld

cp .env.example .env   # optional; compose defaults work without it
docker compose up -d --build
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8787/health
```

## GHCR image

Published from `main` and version tags (`v*`):

```text
ghcr.io/konghayo/cellp:latest
ghcr.io/konghayo/cellp:main
ghcr.io/konghayo/cellp:<git-sha>
ghcr.io/konghayo/cellp:<semver>   # on v* tags
```

Pull and run without building:

```bash
export CELLP_IMAGE=ghcr.io/konghayo/cellp:latest
docker compose up -d
```

## Required environment variables

Use repo-root **`.env.example`** for Docker (`cp .env.example .env`). Compose sets sensible defaults for container networking and the **embedded Node Agent** (AD-15).

On first start, `docker-entrypoint.sh` generates dev mTLS material under `/data/certs/elastic` (volume `cellp-elastic-certs`) when PEM files are absent. Override paths via `CELLP_AGENT_*_CERT_FILE` / `CELLP_AGENT_*_KEY_FILE` env vars and mount your own certs at the same paths.

| Variable | Default (compose) | Purpose |
|----------|-------------------|---------|
| `CELLP_DEPLOY_TOKEN` | `dev-local-token` | Deploy API auth |
| `CELLP_ADMIN_TOKEN` | `dev-local-token` | Admin API auth |
| `CELLP_REGISTRY_DB` | `/data/registry/cellp-registry.sqlite` | SQLite registry (volume) |
| `S3_ENDPOINT` | `http://rustfs:9000` | RustFS inside compose |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | `rustfsadmin` | S3 credentials |
| `AWS_REGION` | `us-east-1` | S3 region |
| `OFFSHOOT_STORE` | `s3://cellp-offshoot` | offshoot store (RustFS) |
| `OFFSHOOT_S3_ENDPOINT` | `http://rustfs:9000` | offshoot S3 endpoint |
| `OFFSHOOT_S3_PATH_STYLE` | `1` | Path-style S3 for RustFS |
| `OFFSHOOT_CHECKOUTS` | `/data/offshoot-checkouts` | Local checkout cache (volume) |
| `ARTIFACTS_DIR` | `/data/artifacts` | Staging for fetched bundles (volume) |
| `CELLP_ARTIFACTS_BUCKET` | `cellp-artifacts` | Allowed S3 artifact bucket |
| `CELLD_BUCKET` | `s3://cellp-celld/demo-app` | Base celld bucket prefix |
| `CELLD_PORT` | `8792` | Base port; per-version celld uses `8792+N` |
| `GATEWAY_PORT` / `PLATFORM_PORT` | `8787` / `8790` | Published ports |
| `CELLP_AGENT_EMBEDDED` | `1` | Embedded Node Agent in cellpd |
| `CELLP_AGENT_CAPACITY_UNITS` | `8` | Node capacity units |
| `CELLP_AGENT_MAX_BODY_BYTES` | `1048576` | Agent HTTP body limit |
| `CELLP_AGENT_*_CERT_FILE` | `/data/certs/elastic/...` | mTLS PEM paths (auto-generated on first boot) |

See `.env.example` for the full embedded-agent block (`CELLP_AGENT_NODE_ID`, heartbeat intervals, SPIFFE URIs, etc.).

**Production:** set strong `CELLP_DEPLOY_TOKEN` and `CELLP_ADMIN_TOKEN`. Replace auto-generated agent certs with your own PKI if exposing the agent port beyond localhost.  
**Debug:** `CELLP_CELLD_WATCH_PERSIST=1` persists per-version `CELLD_WATCH` dirs (default is ephemeral `$TMPDIR`).

## Volumes

| Volume | Mount | Purpose |
|--------|-------|---------|
| `rustfs-data` | RustFS `/data` | S3 object store |
| `cellp-registry` | `/data/registry` | SQLite registry |
| `cellp-artifacts` | `/data/artifacts` | Artifact staging |
| `cellp-offshoot-checkouts` | `/data/offshoot-checkouts` | offshoot checkout cache |
| `cellp-elastic-certs` | `/data/certs/elastic` | Embedded Node Agent mTLS (auto-init) |

## Build image only

```bash
docker build -f docker/Dockerfile -t cellp:local .
```

Fast celld loop (lab profile, larger image):

```bash
docker build -f docker/Dockerfile --build-arg CELLD_PROFILE=lab -t cellp:lab .
```

## Image contents

| Binary | Source |
|--------|--------|
| `cellpd` | `cellp/cmd/cellpd` (Go) |
| `cellp` | `cellp/cmd/cellp` (Go) |
| `celld` | `celld/` submodule (`cargo build -p celld --profile release`) |
| `offshoot` | `go install github.com/sricola/offshoot/cmd/offshoot@latest` |
| `esbuild` | npm global (celld deploy bundling) |

## CI

`.github/workflows/docker-publish.yml` builds and pushes to GHCR on push to `main` and `v*` tags.  
Uses `GITHUB_TOKEN` (no extra secrets). Submodule `celld` is checked out recursively.
