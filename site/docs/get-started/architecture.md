# Architecture at a glance

cellp is a **private Workers control plane** on your hardware: external CI (or `cellp dev`) registers **versions**; each ready version gets an isolated **data bucket** and a **celld** process; HTTP traffic enters through a **gateway** that picks the version from the **Host** header.

## Components

| Piece | Role |
|-------|------|
| **cellpd** | Control plane: REST API, job orchestrator, gateway, SQLite registry |
| **celld** | Cloudflare Workers–compatible runtime (maintained in the cellp repo as `celld/`); **one process per ready version** |
| **Object storage** | Artifacts, per-version celld data, and offshoot blobs — **RustFS** in Docker/Compose; embedded local S3 with `cellp dev` |
| **offshoot** | Copy-on-write **App + Data** branches (SQLite export/import, S3-backed in production) |
| **Dashboard** | Vite + React SPA; talks **only** to the API on `:8790` — never to celld ports or S3 |

Workers code and bindings are declared in **wrangler**; the platform does not host Git or run your CI.

## Default ports (local)

| Port | Service |
|------|---------|
| **8787** | Gateway — preview and production HTTP (route by `Host`) |
| **8790** | REST API, admin operations, Dashboard backend, `/metrics` |
| **8792+** | celld upstreams (one port per ready version, assigned by cellpd) |
| **5190** | Dashboard dev server (`pnpm --filter cellp-dashboard dev`) |
| **9000 / 19000** | S3 API — RustFS in the contributor stack, or embedded store with `cellp dev` |

See [Local stack](/get-started/local) for script names and [cellp dev](/guides/dev) for the no-Docker path.

## Request path

```text
Browser / curl
  → your TLS load balancer (optional, you operate)
  → cellpd gateway :8787  (Host selects project + version)
  → celld for that version
```

Admin and automation:

```text
Dashboard / curl / CI
  → cellpd API :8790  (Bearer deploy or admin token)
  → orchestrator spawns celld, runs offshoot/D1 jobs, updates registry
```

Path-shaped URLs `/{project}/{version}/` are **deprecated**; use **Host-based** preview and prod URLs from the API or Dashboard. Details: [Preview & production](/concepts/preview).

## Laptop vs production-shaped

| | `cellp dev` | Docker Compose / VM |
|--|-------------|---------------------|
| Object storage | Embedded S3 under `~/.cellp` | **RustFS** |
| offshoot store | Local directory tier | **S3** (`OFFSHOOT_STORE`, e.g. `s3://cellp-offshoot`) |
| Goal | Fast product loop | Same layout as a serious self-host |

The contributor **local stack** (`./dev/scripts/up.sh`) uses Docker RustFS on a laptop. **Offshoot** may use a **local directory tier** there, while production Compose keeps branches on **S3** (`OFFSHOOT_STORE`). Gateway behavior is still **Host ingress** in both cases. For a VM-shaped deploy, start with [Self-hosting](/guides/self-hosting).

## What cellp does not include

No managed accounts, DNS, TLS certificates, WAF, or global edge PoPs — you terminate TLS and protect admin surfaces. No PostgreSQL, Caddy, or Forgejo as platform dependencies. See [Limits](/reference/limits) for honesty about single-node registry and roadmap items.

## Related

- [How it works](/how-it-works)
- [Install](/guides/install) · [Quick start](/get-started/)
- [Operator journey](/get-started/operator-journey)
