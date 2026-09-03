---
layout: home
hero:
  name: cellp
  text: Fork the Worker and the data
  tagline: Private Workers control plane on your hardware — not a Cloudflare account. Each child version copy-on-write branches D1, KV, R2, and Queues instead of sharing production data.
  actions:
    - theme: brand
      text: Install
      link: /guides/install
    - theme: alt
      text: cellp dev
      link: /guides/dev
    - theme: alt
      text: GitHub
      link: https://github.com/KonghaYao/cellp
features:
  - title: Branch
    details: The thing that is actually new. Child versions copy-on-write from a parent. QA hits realistic data. Preview writes never land in production.
    link: /concepts/preview
    linkText: How preview forks
  - title: Worker
    details: The same fetch handler and wrangler.jsonc you already write. Each ready version is its own celld process, port, and URL.
    link: /build/
    linkText: Write a Worker
  - title: D1
    details: SQLite that branches. Root versions import a seed. Children run d1 branch — LTX against the parent bucket, not a dump.
    link: /bindings/d1
    linkText: D1
  - title: R2
    details: Object prefixes overlay the parent. Preview puts and deletes stay in the child. Production objects are not rewritten.
    link: /bindings/r2
    linkText: R2
  - title: Queue
    details: Producers and consumers fork with the version. Messages enqueued in preview stay in preview.
    link: /bindings/queues
    linkText: Queues
  - title: KV
    details: Namespaces branch with the version. `env.CACHE` in a PR is not production's cache.
    link: /bindings/kv
    linkText: KV
---

<p class="cellp-kicker">Worker + D1 + KV + R2 + Queue · versioned together · celld runs it · cellp orchestrates it</p>

## Position

**cellp** is the **control plane** for private Workers: CI posts a version, you get a preview Host, you **promote** to production. **[celld](https://github.com/KonghaYao/celld)** is the **runtime** (Workers APIs, bindings). cellp does not re-implement the isolate — it runs one celld per ready version and forks data with **offshoot**.

This is **not** self-hosted Cloudflare: no account, no Anycast edge, no built-in DNS/TLS/CDN. You keep Git, CI, and your load balancer. [Why cellp](/why) · [Compare](/compare) · [From Cloudflare](/migrate/cloudflare)

## cellp + celld

| Piece | Role |
|-------|------|
| **celld** | Execute `fetch`, D1, KV, R2, Queues, Workflows, Cron from your wrangler bundle |
| **cellp** | Registry, Gateway (Host ingress), promote saga, D1/KV/R2/Queue branch orchestration |
| **offshoot** | Copy-on-write data fork between parent and child versions |
| **RustFS** | Private S3 for artifacts and per-version storage |

[What is cellp](/what-is-cellp) · [How it works](/how-it-works) · [Coding Agent on cellp](/research/coding-agent-on-cellp)

<BindingStrip :items="[
  { name: 'Worker', href: '/build/' },
  { name: 'D1', href: '/bindings/d1', forks: true },
  { name: 'KV', href: '/bindings/kv', forks: true },
  { name: 'R2', href: '/bindings/r2', forks: true },
  { name: 'Queue', href: '/bindings/queues', forks: true }
]" />

## What branches with the version

**Branch.** A child version is not a new empty environment that still talks to prod. You pass `parent_version_id`. cellp keeps the **Worker script from this deploy**, and **forks every data binding from the parent**:

<CalloutGrid :items="[
  { title: 'Worker', badge: 'this artifact', body: 'New isolate, new URL. The script is from this wrangler bundle — not a diff of the parent Worker.' },
  { title: 'D1', badge: 'branches', body: 'celld d1 branch. Child bucket stores base.json + LTX. Preview SQL writes stay in the preview.' },
  { title: 'KV', badge: 'branches', body: 'Namespace fork. Large values chain to the parent blob store. Preview puts do not mutate prod keys.' },
  { title: 'R2', badge: 'branches', body: 'Prefix overlay and tombstones. Preview objects and deletes are local to the version.' },
  { title: 'Queue', badge: 'branches', body: 'The queue name in wrangler is the same; the data is a branch. Messages enqueued here never drain in prod.' }
]" />

Workflow instances and Cron are **not** branched — they start from this artifact. That is intentional. [Bindings](/concepts/bindings) · [Preview](/concepts/preview)

## The idea in one sentence

**cellp is the control plane that versions the Worker and its D1, KV, R2, and Queues together**, then gives you a preview URL and a one-shot promote to production.

<Flow :steps="[
  'Install (`curl | sh`) and run `cellp dev`, or upload a wrangler bundle to RustFS in CI',
  'POST /versions with parent_version_id — Worker from this artifact, D1 / KV / R2 / Queue branched from the parent',
  'Each version gets its own celld process and preview Host URL',
  'You hit the preview until you are satisfied. Writes stay in that version.',
  'POST …/promote switches prod Host to that version. Rollback is promote the previous one.'
]" />

## Who this is for

<CalloutGrid :items="[
  { title: 'Teams leaving Cloudflare', body: 'You want Worker + D1 + KV + R2 + Queue on hardware you own, with previews that actually fork the data.' },
  { title: 'Platform / infra companies', body: 'You already run Git, CI, and a load balancer. You need a Workers control plane with binding-aware branches — not another PaaS.' },
  { title: 'YC and technical buyers', body: 'The wedge is branch: App + Data versioning for private Workers. Clear non-goals. Runnable in minutes on a laptop.' }
]" />

## Also true (less exciting, still required)

- **Self-hosted.** `curl | sh` then `cellp dev` on a laptop. Docker Compose + RustFS on a VM. No Cloudflare account, no AWS S3.
- **Promote is explicit.** Production is not “whatever main built last.”
- **Honest boundary.** No Git hosting, user accounts, DNS, CDN, or TLS. Your CI pushes versions. Your load balancer terminates HTTPS.
- **Dashboard.** Projects, versions, D1 browser, KV, queues — same REST API as CI.

## Start in three commands

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp dev
```

If the current directory has `wrangler.jsonc`, it is deployed as version `dev`. Otherwise the platform is up and you can [write a Worker](/build/).

[Install](/guides/install) · [cellp dev](/guides/dev) · [Quick start](/get-started/)
