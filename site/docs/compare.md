# Compare

cellp is often measured against Cloudflare Workers and Vercel. Use this page to decide quickly. The differences are intentional.

## At a glance

| Dimension | cellp | Cloudflare Workers | Vercel |
|-----------|-------|--------------------|--------|
| Unit of deploy | **Version** (you name it) | Worker / Pages project in an account | Git deployment |
| Preview | Host `{version}.{project}.{base}` ([routing](/concepts/routing)) | Preview URLs / environments | Every git push / PR |
| Preview **data** | D1 · KV · R2 · Queue **branch** from parent | Usually shared or copied by hand | Often shared DB, or separate storage |
| Preview **workflows** | Instances **not** copied | Account-scoped runtime | N/A (different model) |
| **Cron** | Prod version arms schedules; while **`prod_version_id` is empty**, each deploying ready version may arm | Per Worker / environment | Platform cron / external |
| Production | First **ready** sets prod automatically; later **promote** | Route / domain in the account | Production branch alias |
| Git | External. CI calls the API | Wrangler / dashboard / Git integration | First-class Git app |
| Accounts / SSO | **None** (two tokens) | Cloudflare account + members | Vercel teams |
| Runtime | celld (Workers APIs, partial) | Cloudflare edge isolates | Node / Edge (framework-defined) |
| Next.js SSR | **No** (experimental OpenNext Worker only) | Pages / not the Workers-only path | **Yes** (home turf) |
| Hosting | **Your** machines | Cloudflare network | Vercel cloud |
| DNS / TLS / CDN | Your LB | Built in | Built in |
| Object store | RustFS (private S3) | R2 (Cloudflare) | Blob / external |
| Global PoPs | No | Yes | Yes |
| Durable Objects | celld **partial**; no cellp data branch | Full product | N/A |

## vs Cloudflare Workers

**Keep:** the Worker source shape, wrangler binding names, and D1/KV/R2/Queue/Workflow/Cron **where celld implements them**.

**Change:** you do not `wrangler deploy` into a Cloudflare account. You build a bundle, upload it to **your** S3-compatible store, and `POST /versions`. Preview uses **Host** ingress, not path-prefixed URLs.

**Gain:** private deployment; preview that forks D1/KV/R2/Queue; promote/rollback you control; no vendor control plane.

**Lose:** Anycast edge, Workers AI, Vectorize, Hyperdrive, `wrangler tail`, account-level DNS, and Cloudflare-only bindings marked **No** in [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md). See [From Cloudflare](/migrate/cloudflare) and [Limits](/reference/limits).

**Split responsibilities:** API fidelity is **celld**; version branching, cron arming, and operator APIs are **cellp**—see [Binding guides](/bindings/).

## vs Vercel

**Keep:** the *feeling* of preview URL → click around → promote to production.

**Change:** the unit is a **Workers version**, not a git deployment of a Next.js app. cellp does not clone your repo or run `next start`.

**Gain:** durable bindings versioned with the app (the thing Vercel preview + shared Postgres usually get wrong).

**Lose:** framework ecosystem, instant git integration, image optimization, Edge Middleware as Vercel defines it. If your app is App Router on Node, stay on Vercel. See [From Vercel](/migrate/vercel) and [Supported stacks](/migrate/stacks).

## Mental model cheat sheet

```
Vercel:     git push  →  deployment  →  preview / production alias
Cloudflare: wrangler deploy  →  Worker in an account  →  routes / domains
cellp:      CI upload + POST /versions  →  isolated version  →  preview Host
            POST /promote  →  production Host (cron arms on prod)
```

## When cellp is the right layer

You want **Workers semantics + private metal + data-aware previews**.

You already own Git, CI, and HTTPS. You are not asking cellp to be your company IdP or your CDN.

Next: [Get started](/get-started/) or [How it works](/how-it-works).
