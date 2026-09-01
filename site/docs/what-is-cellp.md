# What is cellp

**cellp** is a **private Workers platform control plane**. It versions your Worker **and** its data on every deploy, serves a preview URL, and lets you promote that version to production.

It is not a Cloudflare account, not a Vercel clone, and not a Git host. You keep GitHub (or GitLab, or Forgejo), your CI, and your load balancer. cellp sits in the middle: **receive an artifact, run an isolated version, route traffic**.

## The product

A **project** is an app. A **version** is one deploy of that app.

| You get | What that means |
|---------|-----------------|
| **Preview URL** | `preview_url` — live Worker for that deploy (HTTP **Host** on gateway) |
| **Production URL** | `prod_url` — version you last promoted (prod Host unchanged on promote) |
| **Data that matches the code** | Child versions **branch** D1, KV, R2, and Queues from a parent. Writes stay in the preview. |
| **Dashboard** | Projects, versions, storage browsers, env vars — talking only to the REST API |
| **Self-host** | One-line install then `cellp dev` on a laptop (no Docker). Docker Compose + **RustFS** for a production-shaped VM. |

## What you write

A Cloudflare-style Worker:

```js
export default {
  async fetch(request, env) {
    const row = await env.DB.prepare('select count(*) as n from products').first()
    return Response.json(row)
  },
}
```

Bindings come from `wrangler.jsonc` inside the bundle (`d1_databases`, `kv_namespaces`, `r2_buckets`, `queues`, `workflows`, `triggers.crons`). The runtime is **[celld](https://github.com/KonghaYao/celld)** — a Rust Workers engine. cellp does not re-implement the isolate; it **orchestrates versions of it**.

## What cellp is not

These are **product decisions**, not missing tickets:

| Not in scope | Who owns it |
|--------------|-------------|
| User accounts, orgs, SSO, RBAC | Your IdP / outer layer, or two shared tokens |
| Git hosting, webhooks, PR bots | GitHub / GitLab / Forgejo + CI |
| DNS, CDN, TLS, WAF, DDoS | Your load balancer / Nginx / gateway |
| Global anycast PoPs | Not a CDN. Distributed control plane is a later scale-out, not edge |
| Next.js / Node serverless | Use Vercel or a Node host. cellp runs **Workers**, not Node |

Read the full comparison in [Compare](/compare) and the honest list in [Limits](/reference/limits).

## The stack (operator view)

| Piece | Role |
|-------|------|
| **cellpd** | API, orchestrator, gateway, SQLite registry |
| **celld** | Workers + D1 + KV + R2 + Queue + Workflow runtime (one process per ready version) |
| **offshoot** | Copy-on-write SQLite branching for App + Data |
| **RustFS** | Private S3 for artifacts, offshoot, and per-version blobs |
| **Dashboard** (`web/`) | Vite SPA. Never talks to celld directly |

You already have Git and CI. cellp does not replace them.

## Status

cellp **v1** is a working platform: CD loop, preview/prod routing, promote, D1 import/branch, KV/R2/Queue branch, archive/wake, Dashboard, Docker image on GHCR.

It is built to be **run by you**. There is no hosted cellp cloud.

Next: [Why cellp](/why) · [How it works](/how-it-works) · [Quick start](/get-started/)
