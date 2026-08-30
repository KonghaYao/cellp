# Limits

Intentional constraints so you do not plan on a Cloudflare-shaped roadmap that will not ship inside cellp.

## Product boundary

| You might want | cellp |
|----------------|--------|
| Signup, orgs, SSO, RBAC | **No** — two tokens |
| Git host, GitHub App, PR comments | **No** — CI calls HTTP |
| DNS, ACME, CDN, WAF, DDoS | **No** — your LB |
| Global edge PoPs | **No** — your region, your machines |
| Next.js / Node serverless | **No** |
| Hosted cellp cloud | **No** — you run the binary |

## Runtime (celld)

| You might want | Status |
|----------------|--------|
| Workers AI, Vectorize, Hyperdrive | **No** |
| Browser Rendering, Email Workers, Python Workers | **No** |
| Full Cloudflare KV/R2/Queue/DO/Workflows | **Partial** — see celld compat |
| `wrangler tail` | **No** |
| R2 object manager | **No** |
| Workflow pause/resume/restart | **No** |
| Wake-on-request for archived versions | **No** in v1 (explicit `wake`) |

## Scale (v1 honesty)

- **Ready versions** have no small hard cap; idle ones **archive** to drop processes.
- Registry is **SQLite**. Fine for a serious single-node (or VIP) control plane; not a multi-region consensus product.
- Deep health returns **503** when the deploy queue is past `CELLP_QUEUE_MAX`.
- Phase-6 “10 million” stories beyond SQLite scope are **out of v1**.

## Data plane

- Child versions should fork a **seed/staging** parent, not casually fork prod.
- Destroyed versions cannot be rolled back.
- Local `CELPD_WATCH` is cache. **S3/RustFS** is durable.

## What we *do* guarantee as the product

- Isolated runtime per ready version
- Preview and production URL shapes
- D1/KV/R2/Queue branch on children
- Promote saga + re-promote rollback
- REST + Dashboard over the same API
- Self-host without AWS/Cloudflare accounts
