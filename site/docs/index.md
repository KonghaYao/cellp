---
layout: home
hero:
  name: cellp
  text: Versioned Workers, on your hardware
  tagline: A private control plane for Cloudflare-style Workers. Every deploy versions the app and its data. Preview is a real fork. Promote when you are ready.
  actions:
    - theme: brand
      text: Get started
      link: /get-started/
    - theme: alt
      text: Why cellp
      link: /why
    - theme: alt
      text: GitHub
      link: https://github.com/KonghaYao/cellp
features:
  - title: App + data, same version
    details: A preview is not just a new Worker. D1, KV, R2, and Queues fork with the deploy so QA hits realistic data without writing into production.
  - title: Preview, then promote
    details: "Each version gets its own URL. Production is an explicit promote: drain, cut over, keep a rollback path — not whatever main built last."
  - title: Workers you already write
    details: "Workers you already write: fetch handlers plus wrangler bindings. celld runs the isolate. cellp versions, routes, and operates it."
  - title: 100% self-hosted
    details: No Cloudflare account, no AWS S3, no Vercel. Runs on your machines with RustFS, SQLite, and Docker Compose.
  - title: Honest product boundary
    details: cellp does not do Git hosting, user accounts, DNS, CDN, or TLS. Your CI pushes versions. Your load balancer terminates HTTPS.
  - title: Operator dashboard
    details: Projects, deployments, D1 browser, KV, queues, env vars — all through the same REST API your CI already calls.
---

<p class="cellp-kicker">For developers · for operators · for YC</p>

## The idea in one sentence

**cellp is the control plane that makes every Workers deploy a version of both the code and the data**, then gives you a preview URL and a one-shot promote to production.

<Flow :steps="[
  'CI builds a wrangler bundle and uploads it to your object store',
  'POST /v1/projects/{id}/versions creates a version (optionally forked from a parent)',
  'cellp starts an isolated runtime, branches D1 / KV / R2 / Queue, and returns a preview URL',
  'You hit /{project}/{version}/ until you are satisfied',
  'POST …/promote cuts production traffic to /{project}/'
]" />

## Who this is for

<CalloutGrid :items="[
  { title: 'Teams leaving Cloudflare', body: 'You want Workers + D1 + KV semantics on hardware you own, with preview environments that do not share production data.' },
  { title: 'Platform / infra companies', body: 'You already run Git, CI, and a load balancer. You need a Workers runtime and a versioned control plane — not another PaaS.' },
  { title: 'YC and technical buyers', body: 'A sharp wedge: App + Data versioning for private Workers. Clear non-goals. Runnable in minutes on a laptop.' }
]" />

## Start in three commands

```bash
git clone https://github.com/KonghaYao/cellp.git && cd cellp
git submodule update --init celld
cp dev/.env.example dev/.env && ./dev/scripts/up.sh && ./dev/scripts/seed-commerce-store.sh
```

Then open `http://127.0.0.1:8787/commerce-store/v1/` — a storefront backed by D1, KV, R2, Queues, Workflows, and Cron.

[Full quick start →](/get-started/) · [What is cellp →](/what-is-cellp) · [Compare Cloudflare & Vercel →](/compare)
