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
  → preview_url from API (Host on gateway :8787)
  → promote: POST …/promote
```

There is no `wrangler deploy` target for cellp. CI (or a script) is the client.

## Concepts

| Cloudflare | cellp |
|------------|--------|
| Worker in an **account** | **Version** = process + bucket |
| Preview URLs / env branches | Preview Host + prod Host (path `/{project}/…` deprecated) |
| D1 `database_id` shared or copied | Root: import. Child: **d1 branch** |
| KV / R2 / Queue shared | Child **branches** parent data |
| Production route / custom domain | Promote + **your** DNS/TLS |
| Wrangler-managed `wrangler.toml` lifecycle | Bundle contains wrangler JSON; celld parses at deploy |

`parent_version_id` means “fork App + Data from this version.” Point PRs at a **seed/staging** version.

Archived versions are stopped processes (503) until `POST …/wake`. They are not “sleeping Workers that wake on the first request.”

## Bindings

Use the [bindings overview](/concepts/bindings). Identities inherit on branch so you do not rewrite `env.DB`.

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
| Workflow pause/resume | No |

Judge a Worker against [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md), not against “it ran on CF.”

**Framework tiers:** cellp **tier-1** stacks (Astro, SvelteKit, Remix, Nuxt, Vite SPA) and **Next.js (experimental only)** are documented in [Framework tiers](/migrate/frameworks) and [Supported stacks](/migrate/stacks).

## Suggested migration

1. Confirm each binding is Yes/Partial in celld compat (D1, KV, Queues, R2, Workflows, Cron).
2. Run the [local stack](/get-started/local) and deploy the commerce example.
3. Point CI at artifact upload + `POST /versions`.
4. Use `parent_version_id` for PR previews; check Dashboard lineage.
5. Promote a known-good version; put your LB in front of the gateway.

## Durable Objects

celld has **partial** Durable Objects. If DO is load-bearing, read the compat notes before you promise a date. cellp does not add a second DO layer.
