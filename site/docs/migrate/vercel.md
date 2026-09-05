# From Vercel

cellp feels like “preview URL, then production” — with a Workers runtime and **versioned data**. It is not a place to host Next.js on Node.

## If you are on App Router / Node

Stop here: [Supported stacks](/migrate/stacks). Run Next on Vercel or a Node host. cellp will not execute `next start`.

If you can express the app as a **Worker** (`fetch` + bindings) — including static assets via wrangler `assets` — continue. OpenNext **Worker** bundles are [experimental](/migrate/frameworks), not tier 1.

## Git push vs POST version

**Vercel:** `git push` → build → Preview deployment / Production alias.

**cellp:** Git host + **your** CI:

```
build Workers bundle
  → upload artifact
  → POST /versions
  → preview_url / prod_url (Gateway Host)
  → POST /promote → prod Host
```

| Vercel | cellp |
|--------|--------|
| Preview Deployment | Version (`id` you choose) |
| Production | `prod_version_id` + **prod Host** (`prod_url`) |
| Branch URL | **None** — use the version id |
| PR previews | CI `POST /versions` + `parent_version_id` |
| Instant rollback alias | Re-[promote](/guides/rollback) a previous version |

Example workflow: [`ci-pr-preview.example.yml`](https://github.com/KonghaYao/cellp/blob/main/dev/examples/ci-pr-preview.example.yml).

## Environment variables

| Vercel | cellp |
|--------|--------|
| Production / Preview / Development | **Per-version** overrides |
| Encrypted Dashboard secrets | Strings + CI injection — [Env](/guides/environment-variables) |
| `VERCEL_*` | `PROJECT_ID` / `VERSION_ID` (read-only) |

## Data

Vercel Postgres/KV previews often **share** production or require a second instance. cellp’s default for children is **fork D1/KV/R2/Queue** from the parent version.

| Pattern | cellp |
|---------|--------|
| Vercel Postgres / Neon preview DB | Use [D1](/bindings/d1) on cellp, or call **your** database from the Worker (no Hyperdrive layer) |
| Edge Config / KV | [KV](/bindings/kv) with branch on preview versions |
| Blob | [R2](/bindings/r2) with branch on preview versions |

Workflow **instances** do not fork. Cron runs on **production** scheduling policy only ([Cron](/bindings/cron)). Durable Objects are not branched like D1—see [Durable Objects](/bindings/durable-objects).

## Ops

| Need | Vercel | cellp |
|------|--------|--------|
| Rollback | Redeploy / alias | Wake if needed + promote |
| Sleep unused previews | N/A (SaaS) | [Archive](/concepts/archive) |
| Long-lived QA | Keep the deployment | **Pin** the version |
| Logs | Built-in | Prometheus + files — [Observability](/guides/observability) |

## Auth and teams

Vercel teams / SSO have no equivalent. Put the Dashboard behind **your** proxy. cellp authenticates with two bearer tokens.
