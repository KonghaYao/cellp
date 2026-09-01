# Write a Worker

You ship a **Cloudflare-style Worker**: a JavaScript module plus a `wrangler.jsonc` that names your data. cellp does not host Next.js and does not generate the Worker for you.

This page is the developer path: **code → config → first version → URL**.

## What you create

```text
my-shop/
  wrangler.jsonc    # entry + bindings (this is how you configure platform data)
  index.js          # the Worker
```

That folder **is** the deploy artifact. Put it at `s3://cellp-artifacts/{project}/{version}/` (locally: `dev/data/artifacts/{project}/{version}/`).

The orchestrator deploys that directory **only if it contains `wrangler.jsonc`**. A `wrangler.json`-only folder is parsed for listings, but **deploy falls back to `dev/examples/counter`**. Name the file `wrangler.jsonc`.

You do not run `celld deploy` yourself for this path — cellpd does it. There is no “create D1 in the dashboard first” step.

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

`name` is the Worker name inside celld. The **cellp project id** is used in **ingress Host** names — keep `name` and project id aligned.

Add databases, KV, R2, queues, cron, and workflows in this file. That is [how you configure platform data](/build/wrangler).

## 3. Create a project (once)

Locally both tokens default to `dev-local-token`. If you split them: **create project** and **poll version** need `ADMIN_TOKEN`; **POST /versions** needs `DEPLOY_TOKEN`. Using the admin token on `POST /versions` is **403** when the two secrets differ.

`POST /versions` will also create the project if it does not exist, so this step is optional.

```bash
curl -sS -X POST http://127.0.0.1:8790/v1/projects \
  -H "Authorization: Bearer dev-local-token" \
  -H "Content-Type: application/json" \
  -d '{"id":"my-shop"}'
```

Or use the Dashboard → Projects → create.

## Local `cellp dev`

After [install](/guides/install), from this folder:

```bash
cellp dev
```

That starts the platform (no Docker) and deploys cwd as version `dev`. See [cellp dev](/guides/dev).

You can also stage files yourself (CI-shaped):

```bash
mkdir -p dev/data/artifacts/my-shop/v1
cp wrangler.jsonc index.js dev/data/artifacts/my-shop/v1/

curl -sS -X POST http://127.0.0.1:8790/v1/projects/my-shop/versions \
  -H "Authorization: Bearer dev-local-token" \
  -H "Content-Type: application/json" \
  -d '{"id":"v1"}'
```

Poll until `status` is `ready` (loop; a single GET is a snapshot). Locally the same token works; in production poll with **admin**:

```bash
curl -sS http://127.0.0.1:8790/v1/projects/my-shop/versions/v1 \
  -H "Authorization: Bearer dev-local-token" | jq .status
```

Open the **`preview_url`** from `GET …/versions/v1` (Host + `:8787` in dev). Example after [ingress setup](https://github.com/KonghaYao/cellp/blob/main/dev/INGRESS-HOST.md):

```bash
curl -sS http://127.0.0.1:8790/v1/projects/my-shop/versions/v1 \
  -H "Authorization: Bearer dev-local-token" | jq -r .preview_url
# → http://v1.my-shop.ingress.local:8787/
```

If this is the **first** ready version on the project, the **prod Host** already points at it — no promote yet. Later deploys stay preview-only until you [promote](/concepts/promote).

`./dev/scripts/simulate-cd.sh` is **not** this flow: it extra-`celld deploy`s the **counter** example unless you pass a third path, always sends `parent_version_id: null`, and does not copy your files into `artifacts/`. Prefer the copy + `POST /versions` steps above. For commerce: `./dev/scripts/simulate-cd.sh commerce-store v-dev2 dev/examples/commerce`.

## What to read next

| You want to | Page |
|-------------|------|
| Attach D1 / KV / R2 / queues in wrangler | [Configure bindings](/build/wrangler) |
| Seed tables, banners, forks | [Platform data](/build/data) |
| `scheduled`, queues, workflows, Durable Objects | [Handlers](/build/handlers) |
| Copy a full store | [Commerce example](/get-started/example) |
| CI instead of a laptop | [Deploy from CI](/guides/ci) |
