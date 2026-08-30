# Local stack

The `dev/` directory is a full cellp on one machine: RustFS, cellpd (API + gateway), celld, offshoot.

## Bring it up

```bash
cp dev/.env.example dev/.env   # first time
./dev/scripts/up.sh
./dev/scripts/health.sh
```

Copying `.env` is required. Do not hand-edit files under `dev/data/`; use `./dev/scripts/reset.sh` if the registry is wedged.

## Ports

| Port | Service |
|------|---------|
| **8787** | Gateway (preview + prod HTTP) |
| **8790** | REST API + Prometheus `/metrics` |
| **8792+** | celld — one process per ready version, ports increment |
| **5190** | Dashboard Vite dev server (`cd web && npm run dev`) |
| **9000 / 19000** | RustFS S3 (see `dev/.env.example`) |
| **9001** | RustFS console |

Dev defaults: `CELLP_DEPLOY_TOKEN` and `CELLP_ADMIN_TOKEN` are `dev-local-token`.

## Useful scripts

| Script | What it does |
|--------|----------------|
| `up.sh` / `down.sh` | Start / stop the stack |
| `health.sh` | Probe every component (use this as your green light) |
| `reset.sh` | Wipe `dev/data/` |
| `simulate-cd.sh <project> <version>` | Fake a CI deploy |
| `seed-commerce-store.sh` | Commerce example + D1 seed |
| `seed-demo.sh` | Bindings playground (`demo-app`) |
| `logs.sh` | Tail logs |
| `gc.sh` | Registry GC (old jobs / destroyed versions) |

## Local CD

```bash
./dev/scripts/simulate-cd.sh demo-app v-test1
curl -sf http://127.0.0.1:8790/v1/projects/demo-app/versions/v-test1 \
  -H "Authorization: Bearer dev-local-token" | jq .status
# "ready"

curl -sf http://127.0.0.1:8787/demo-app/v-test1/
```

## What is mock vs real

On a laptop, some gateway behavior may be simulated so you can iterate without a full production offshoot-on-S3 setup. Functionally you still:

- POST versions
- Get preview URLs
- Inspect D1/KV in the Dashboard

Production-shaped offshoot-on-RustFS is a separate gate (`v0b`). For product usage, the local stack is the right default.

## Dashboard alongside the stack

```bash
# terminal 1
./dev/scripts/up.sh

# terminal 2
cd web && npm install && npm run dev
# http://127.0.0.1:5190
```

The Dashboard only calls `:8790`. It never talks to celld or S3.

## When stuck

```bash
./dev/scripts/logs.sh
./dev/scripts/down.sh
./dev/scripts/reset.sh
./dev/scripts/up.sh
```

More operator detail lives in the repo: [`dev/README.md`](https://github.com/KonghaYao/cellp/blob/main/dev/README.md).
