# Docker deployment

Single-machine stack: **RustFS** (S3) + **cellpd** (API `:8790`, Gateway `:8787`).  
Per-version **celld** runtimes are spawned by cellpd at deploy time — they are not separate compose services.

## Quick start

```bash
# Build locally (requires celld submodule)
git submodule update --init celld

docker compose up -d --build
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8787/health
```

Optional Valkey (not required by cellpd today):

```bash
docker compose --profile valkey up -d
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

Match names from `dev/.env.example`. Compose sets sensible defaults for container networking.

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

**Production:** set strong `CELLP_DEPLOY_TOKEN` and `CELLP_ADMIN_TOKEN`.  
**Debug:** `CELLP_CELLD_WATCH_PERSIST=1` persists per-version `CELLD_WATCH` dirs (default is ephemeral `$TMPDIR`).

## Volumes

| Volume | Mount | Purpose |
|--------|-------|---------|
| `rustfs-data` | RustFS `/data` | S3 object store |
| `cellp-registry` | `/data/registry` | SQLite registry |
| `cellp-artifacts` | `/data/artifacts` | Artifact staging |
| `cellp-offshoot-checkouts` | `/data/offshoot-checkouts` | offshoot checkout cache |

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
| `celld` | `celld/` submodule (`cargo build -p celld --profile release`) |
| `offshoot` | `go install github.com/sricola/offshoot/cmd/offshoot@latest` |
| `esbuild` | npm global (celld deploy bundling) |

## CI

`.github/workflows/docker-publish.yml` builds and pushes to GHCR on push to `main` and `v*` tags.  
Uses `GITHUB_TOKEN` (no extra secrets). Submodule `celld` is checked out recursively.
