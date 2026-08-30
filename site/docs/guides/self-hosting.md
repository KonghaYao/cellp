# Self-hosting

cellp is meant to run on **your** machines.

- **Laptop, no Docker:** [Install](/guides/install) then [cellp dev](/guides/dev).
- **Production-shaped VM:** Docker Compose with **RustFS** + **cellpd** (`cellp serve` / `ghcr.io/konghayo/cellp`). celld processes are spawned by cellpd — not extra Compose services.

## Quick start

```bash
git clone https://github.com/KonghaYao/cellp.git && cd cellp
git submodule update --init celld
docker compose up -d --build
curl -sf http://127.0.0.1:8790/v1/health
curl -sf http://127.0.0.1:8787/health
```

## GHCR image

Published from `main` and `v*` tags:

```text
ghcr.io/konghayo/cellp:latest
ghcr.io/konghayo/cellp:main
ghcr.io/konghayo/cellp:<git-sha>
ghcr.io/konghayo/cellp:<semver>
```

```bash
export CELLP_IMAGE=ghcr.io/konghayo/cellp:latest
docker compose up -d
```

## Environment

Match names in `dev/.env.example`. Compose fills container networking defaults.

| Variable | Purpose |
|----------|---------|
| `CELLP_DEPLOY_TOKEN` | CI `POST /versions` |
| `CELLP_ADMIN_TOKEN` | Admin API + Dashboard |
| `CELLP_REGISTRY_DB` | SQLite path (volume) |
| `S3_ENDPOINT` | RustFS inside the network |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | S3 credentials |
| `OFFSHOOT_STORE` | e.g. `s3://cellp-offshoot` |
| `CELLP_ARTIFACTS_BUCKET` | Allowed artifact bucket |
| `CELLD_BUCKET` | Base prefix for per-version celld data |
| `GATEWAY_PORT` / `PLATFORM_PORT` | `8787` / `8790` |

**Production:** use strong tokens. Put TLS on a reverse proxy in front of `:8787` and `:8790`. Do not expose RustFS console publicly.

## Volumes

| Volume | Why |
|--------|-----|
| `rustfs-data` | Objects (artifacts, offshoot, celld blobs) |
| `cellp-registry` | SQLite registry |
| `cellp-artifacts` | Fetch staging |
| `cellp-offshoot-checkouts` | Checkout cache |

## Build yourself

```bash
docker build -f docker/Dockerfile -t cellp:local .
# faster celld, larger image:
docker build -f docker/Dockerfile --build-arg CELLD_PROFILE=lab -t cellp:lab .
```

The image includes `cellpd`, `cellp`, `celld`, `offshoot`, and `esbuild`.

## Front door

```
Internet → your LB (TLS, WAF) → cellpd :8787  (Workers traffic)
                              → cellpd :8790  (API, locked down)
                              → Dashboard static (optional, locked down)
```

cellp will not obtain Let's Encrypt certificates or write DNS records.

## What we do not run

PostgreSQL, Caddy, Forgejo, AWS S3, Cloudflare R2 as **cellp dependencies**. Swap any of those in as *your* outer layer if you want; the control plane still speaks SQLite + RustFS.

Repo guide: [`docker/README.md`](https://github.com/KonghaYao/cellp/blob/main/docker/README.md).
