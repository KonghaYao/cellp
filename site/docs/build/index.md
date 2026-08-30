# Write a Worker

You ship a **Cloudflare-style Worker**: a JavaScript module plus a `wrangler.jsonc` that names your data. cellp does not host Next.js and does not generate the Worker for you.

This page is the developer path: **code → config → first version → URL**.

## What you create

```text
my-shop/
  wrangler.jsonc    # entry + bindings (this is how you configure platform data)
  index.js          # the Worker
```

That folder **is** the deploy artifact. Put it at `s3://cellp-artifacts/{project}/{version}/` (locally: `dev/data/artifacts/{project}/{version}/`). cellp reads `wrangler.jsonc` / `wrangler.json` from there. There is no extra “create D1 in the dashboard first” step.

## 1. The Worker

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url)
    if (url.pathname === '/health') {
      return Response.json({ ok: true })
    }
    return new Response('hello from cellp\n', {
      headers: { 'content-type': 'text/plain; charset=utf-8' },
    })
  },
}
```

Same contract as Cloudflare Workers:

| Argument | What it is |
|----------|------------|
| `request` | Incoming `Request` (path is your app path, not `/{project}/{version}`) |
| `env` | Bindings + vars from wrangler, plus platform `PROJECT_ID` / `VERSION_ID` |
| `ctx` | `waitUntil` / `passThroughOnException` as celld implements them |

You can use TypeScript if your CI emits JS (or a bundle) that `main` points at. celld bundles with **esbuild**.

## 2. Declare the app in wrangler

```jsonc
{
  "name": "my-shop",
  "main": "index.js",
  "compatibility_date": "2026-01-01"
}
```

`name` is the Worker name inside celld. The **cellp project id** (URL segment `/{project}/`) is chosen when you create the project — keep them the same to stay sane.

Add databases, KV, R2, queues, cron, and workflows in this file. That is [how you configure platform data](/build/wrangler).

## 3. Create a project (once)

Local tokens default to `dev-local-token`.

```bash
curl -sS -X POST http://127.0.0.1:8790/v1/projects \
  -H "Authorization: Bearer dev-local-token" \
  -H "Content-Type: application/json" \
  -d '{"id":"my-shop"}'
```

Or use the Dashboard → Projects → create.

## 4. Drop the files in as version `v1`

With the [local stack](/get-started/local) already up:

```bash
mkdir -p dev/data/artifacts/my-shop/v1
cp wrangler.jsonc index.js dev/data/artifacts/my-shop/v1/

curl -sS -X POST http://127.0.0.1:8790/v1/projects/my-shop/versions \
  -H "Authorization: Bearer dev-local-token" \
  -H "Content-Type: application/json" \
  -d '{"id":"v1"}'
```

Poll until `status` is `ready`:

```bash
curl -sS http://127.0.0.1:8790/v1/projects/my-shop/versions/v1 \
  -H "Authorization: Bearer dev-local-token" | jq .status
```

Open **http://127.0.0.1:8787/my-shop/v1/**

Helper for the bundled examples: `./dev/scripts/simulate-cd.sh <project> <version> [path-to-worker]`.

## 5. Put it on production when you mean to

Preview is `/{project}/{version}/`. Production is `/{project}/` only after [promote](/concepts/promote).

## What to read next

| You want to | Page |
|-------------|------|
| Attach D1 / KV / R2 / queues in wrangler | [Configure bindings](/build/wrangler) |
| Seed tables, banners, forks | [Platform data](/build/data) |
| `scheduled`, queues, workflows, Durable Objects | [Handlers](/build/handlers) |
| Copy a full store | [Commerce example](/get-started/example) |
| CI instead of a laptop | [Deploy from CI](/guides/ci) |
