# From Cloudflare

You keep writing Workers. You stop deploying into a Cloudflare account.

## Deploy path

**Cloudflare today**

```bash
wrangler deploy     # account + bindings in CF
wrangler dev
```

**cellp**

```
build wrangler bundle
  → upload s3://cellp-artifacts/{project}/{version}/
  → POST /v1/projects/{project}/versions
  → poll until ready
  → preview_url from API (Host on gateway)
  → promote: POST …/promote
```

There is no `wrangler deploy` target for cellp. CI (or a script) is the client.

## celld vs cellp

| Concern | celld | cellp |
|---------|-------|--------|
| Worker APIs (`fetch`, D1, KV, …) | Implements wrangler bindings | Runs one celld per ready version |
| Data isolation on preview | Per-version fleet bucket | **Branches** D1/KV/R2/Queue from parent on child versions |
| Cron | Fires `scheduled` when armed | Arms crons only on **production** (see [Cron](/bindings/cron); pre-promote projects may arm every ready version) |
| Workflows | Runs instances | Instances **not** branched; operator list is read-only |
| Durable Objects | Partial runtime | No branch; not on bindings API |
| Ingress | Trusts forwarded headers from Gateway | Host / port [routing](/concepts/routing) |

Gap list for APIs: [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md). [Binding guides](/bindings/).

## Wrangler checklist

Before you cut over CI:

1. **Single main Worker** — no `[[services]]` multi-worker graph (unsupported on cellp today). Queue **consumers** are a **second** Worker artifact (often a second version)—see [Queues](/bindings/queues).
2. **Binding names** — keep `env.DB`, `env.MY_KV`, etc.; child versions [inherit identities](/concepts/bindings) on branch.
3. **Pre-build** — ship the worker entry + `assets` from CI; do not depend on celld to compile Tailwind, `.md` imports, or full framework SSR graphs.
4. **`wrangler.json(c)` in the bundle** — celld parses binding config from the artifact at deploy.
5. **No Cloudflare-only bindings** — Workers AI, Vectorize, Hyperdrive, Browser Rendering, Email, Python Workers are **No** in celld.
6. **Validate** — run through [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) for each binding you use.

Local proof: [local stack](/get-started/local) and the commerce example.

## Concepts

| Cloudflare | cellp |
|------------|--------|
| Worker in an **account** | **Version** = process + bucket |
| Preview URLs / env branches | Preview Host + prod Host ([Host-based routing](/concepts/preview)) |
| D1 `database_id` shared or copied | Root: import / seed. Child: **d1 branch** |
| KV / R2 / Queue shared | Child **branches** parent data |
| Workflow state in account | New preview version: **empty instances** |
| Cron on each deployed Worker | **Production** ready version arms schedules ([Cron](/bindings/cron)) |
| Production route / custom domain | Promote + **your** DNS/TLS |
| Wrangler-managed lifecycle | Bundle contains wrangler JSON; celld parses at deploy |
| Hyperdrive / managed Postgres | **No** — use [D1](/bindings/d1) or connect to **your** Postgres/MySQL from the Worker over the network |

`parent_version_id` means “fork App + Data from this version.” Point PRs at a **seed/staging** version—not live production.

Archived versions are stopped processes (503) until `POST …/wake`. They are not “sleeping Workers that wake on the first request.”

## Bindings

Use the [binding guides](/bindings/) and [bindings overview](/concepts/bindings). Identities inherit on branch so you do not rewrite `env.DB`.

**Not ported (on purpose or runtime No):**

| Capability | Status |
|------------|--------|
| `wrangler deploy` / `wrangler dev` against cellp | No — CI + API + gateway URL |
| `wrangler tail` | No |
| Workers AI / Vectorize / Hyperdrive / Browser Rendering / Email / Python Workers | **No** in celld |
| Global Anycast | No |
| Built-in DNS / CDN / TLS / WAF | No |
| Account members | No — two tokens |
| R2 object UI | No |
| Workflow pause/resume in **cellp** Dashboard/API | No — read-only list; celld may support lifecycle from Worker code |
| **Images** binding | **Partial** in celld — [Images](/bindings/images) |
| **Durable Objects** | **Partial** in celld — no cellp branch — [Durable Objects](/bindings/durable-objects) |

Judge a Worker against [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md), not against “it ran on CF.”

**Framework tiers:** tier-1 stacks (Astro, SvelteKit, Remix, Nuxt, Vite SPA) and **Next.js (experimental only)** are in [Framework tiers](/migrate/frameworks) and [Supported stacks](/migrate/stacks).

## Suggested migration

1. Confirm each binding is Yes/Partial in celld compat (D1, KV, Queues, R2, Workflows, Cron, DO/Images if used).
2. Run the [local stack](/get-started/local) and deploy the commerce example.
3. Point CI at artifact upload + `POST /versions`.
4. Use `parent_version_id` for PR previews; check Dashboard lineage.
5. Promote a known-good version; put your LB in front of the gateway.

## Durable Objects

celld has **partial** Durable Objects (RPC across isolates, WebSocket migration, and SQLite edge cases differ from Cloudflare). cellp does **not** branch DO storage like D1/KV/R2/Queue—each version’s DO state is isolated to that celld fleet. If DO is load-bearing, read the compat notes before you promise a date. Operator guide: [Durable Objects](/bindings/durable-objects).
