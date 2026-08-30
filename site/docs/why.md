# Why cellp

A page for technical buyers, founders, and YC partners. Three minutes.

## The problem

Cloudflare Workers is an excellent programming model: isolates, D1, KV, R2, Queues, Cron. Two things break when you try to use it as a **serious private platform**:

1. **It is a public cloud.** Data, execution, and routing live in someone else's account. Many companies cannot (or will not) put core systems there.
2. **Preview is usually code-only.** A PR Worker that still reads production D1/KV is not a preview. Copying databases by hand does not scale.

Vercel solved “preview every git push” for **frontend + serverless functions**. It did not solve **Workers + durable bindings**, and it is not something you run on your own metal.

Self-hosting a Workers runtime (celld, workerd, …) solves the isolate. It does **not** give you version lifecycle, forked data, preview URLs, or a promote cutover.

## The wedge

**Version the app and the data together, on hardware you own.**

When CI posts a new version with `parent_version_id`:

- The Worker script comes from **this** artifact.
- D1, KV, R2, and Queues **branch from the parent** (copy-on-write, not a full dump).
- You get `/{project}/{version}/`.
- Production keeps serving `/{project}/` until you **promote**.

That is the product. Everything else is in service of it.

## Why this is a company-shaped problem

Platform teams already have Git, CI, object storage, and a load balancer. What they lack is a **Workers control plane** with:

- Isolated runtimes per version (so two deploys cannot clobber each other)
- Binding-aware forks (so preview data is real and isolated)
- An explicit production pointer (so rollback is “promote the previous version”)
- An operator API and UI that do not require a Cloudflare account

cellp is that control plane. It is boring on purpose: SQLite registry, RustFS, Docker Compose, two tokens. The interesting part is the **versioning model**.

## What we refuse to build

YC-shaped products die when they silently become “self-hosted Cloudflare.” We freeze the boundary:

- **No accounts.** `DEPLOY_TOKEN` for CI, `ADMIN_TOKEN` for the rest. Multi-tenant RBAC is an outer product.
- **No Git.** CI pushes artifacts. `git_sha` is a label, not a router.
- **No DNS / TLS / CDN / WAF.** Put Nginx or a cloud LB in front.
- **No global edge.** Latency SLA is “your region, your machines,” not anycast PoPs.
- **No Next.js hosting.** If the unit of compute is a Node server, this is the wrong platform.

Honesty is part of the pitch. Buyers who need those layers already have them.

## Who should use it today

- You ship **Workers** (`fetch` handlers + wrangler bindings), not Next.js SSR.
- You need **private** deployment (VPC, on-prem, air-gapped-ish).
- You want **PR / staging previews that include data**.
- You can run Docker and point a reverse proxy at a port.

Who should not: teams whose product is App Router on Node, or who want Cloudflare’s global network as the product.

## Proof, not slides

| Claim | How to verify |
|-------|----------------|
| Preview + prod on one machine | [Quick start](/get-started/) — commerce store on `:8787` |
| Data forks with the version | Deploy a child version with `parent_version_id`; write in preview; prod unchanged |
| Promote is a cutover | [Promote](/concepts/promote) — then [rollback](/guides/rollback) by re-promoting |
| Self-host without AWS | [Docker](/guides/self-hosting) · `ghcr.io/konghayo/cellp` |

Repository: [github.com/KonghaYao/cellp](https://github.com/KonghaYao/cellp)

## One-liner you can reuse

> **cellp is a self-hosted Workers control plane that versions App + Data on every deploy, so preview is a real fork and production is an explicit promote.**
