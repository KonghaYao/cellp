# Self-hosting

cellp is meant to run on **your** machines.

- **Laptop, no Docker:** [Install](/guides/install) then [cellp dev](/guides/dev).
- **Production-shaped VM:** Docker Compose with **RustFS** + **cellpd** (`cellp serve` / `ghcr.io/konghayo/cellp`). celld processes are spawned by cellpd — not extra Compose services. **offshoot** App + Data branches use the same RustFS tier (`OFFSHOOT_STORE`), not the laptop directory store used by `cellp dev`.

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

### Core

| Variable | Purpose |
|----------|---------|
| `CELLP_DEPLOY_TOKEN` | CI `POST /v1/projects/{p}/versions` only |
| `CELLP_ADMIN_TOKEN` | Admin API + Dashboard |
| `CELLP_REGISTRY_DB` | SQLite path (volume) |
| `PLATFORM_PORT` / `GATEWAY_PORT` | `8790` / `8787` |
| `GATEWAY_URL` | Public gateway origin (used in API URLs; set to what browsers/LB use) |

### Ingress

| Variable | Purpose |
|----------|---------|
| `CELLP_INGRESS_BASE_DOMAIN` | Host suffix (`{version}.{project}.{base}` preview, `{project}.{base}` prod) |
| `CELLP_PUBLIC_SCHEME_PREVIEW` / `CELLP_PUBLIC_SCHEME_PROD` | Schemes in `preview_url` / `prod_url` |

Point DNS or `/etc/hosts` at your gateway for those Host patterns. TLS is **your** reverse proxy.

### Storage

| Variable | Purpose |
|----------|---------|
| `S3_ENDPOINT` | RustFS inside the network |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | S3 credentials |
| `OFFSHOOT_STORE` | e.g. `s3://cellp-offshoot` |
| `CELLP_ARTIFACTS_BUCKET` | Allowed artifact bucket |
| `CELLD_BUCKET` | Base prefix for per-version celld data |
| `ARTIFACTS_DIR` | Local fetch staging |

**Production:** use strong tokens. Put TLS on a reverse proxy in front of `:8787` and `:8790`. Do not expose RustFS console publicly.

## Dashboard (optional)

Build the SPA from `web/` with production API origin:

```bash
cd web
cp .env.example .env
# VITE_CELLP_API_URL=https://cellp-api.internal.example   # no /v1 suffix
# VITE_CELLP_ADMIN_TOKEN=…
# VITE_CELLP_GATEWAY_URL=https://workers.example
# VITE_CELLP_INGRESS_BASE_DOMAIN=workers.example
pnpm install && pnpm build
```

Serve `dist/` behind the same access controls as `:8790`. See [Dashboard](/get-started/dashboard).

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

Scrape Prometheus at `http://<cellpd>:8790/metrics` (root path, not `/v1`).

cellp will not obtain Let's Encrypt certificates or write DNS records.

## What we do not run

PostgreSQL, Caddy, Forgejo, AWS S3, Cloudflare R2 as **cellp dependencies**. Swap any of those in as *your* outer layer if you want; the control plane still speaks SQLite + RustFS.

Repo guide: [`docker/README.md`](https://github.com/KonghaYao/cellp/blob/main/docker/README.md).
