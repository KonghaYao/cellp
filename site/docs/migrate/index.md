# Migrate to cellp

cellp replaces **where** you deploy Workers-style apps—not the Worker APIs you already use.

## Start here

| You are coming from | Read first |
|---------------------|------------|
| **Cloudflare Workers** (wrangler, D1, KV, R2) | [From Cloudflare](/migrate/cloudflare) |
| **Vercel** (git previews, Next.js, serverless) | [From Vercel](/migrate/vercel) |
| **Choosing a stack** | [Supported stacks](/migrate/stacks) · [Framework tiers](/migrate/frameworks) |

## What stays the same

- Worker shape: `export default { fetch }`, wrangler binding names, D1/KV/R2/Queue/Workflow/Cron as capabilities (within [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md)).
- Preview → promote mental model (like Vercel previews, but the unit is a **version** with forked data where cellp branches bindings).

## What changes

- **Deploy:** build a bundle → upload to **your** S3-compatible store → `POST /v1/projects/{project}/versions`. No `wrangler deploy` to a Cloudflare account.
- **Routing:** [Host-based ingress](/concepts/preview) on your gateway (`preview_url` / `prod_url` from the API). Path-shaped URLs are deprecated.
- **Production:** explicit [promote](/concepts/promote); DNS/TLS/CDN are **your** layer.
- **Accounts:** two bearer tokens—not Cloudflare or Vercel teams.
- **Data on preview:** cellp **branches** D1/KV/R2/Queue; workflow instances and Durable Objects **do not** copy parent runtime state. Cron **ticks** follow production scheduling policy ([Cron](/bindings/cron)).

## celld vs cellp (one sentence each)

- **celld** — the Workers runtime in your repo; implements `env.*` from wrangler.
- **cellp** — versions, fork/branch, Gateway, promote, and operator APIs on top of celld.

Binding reference: [/bindings/](/bindings/). Compare platforms: [Compare](/compare).
