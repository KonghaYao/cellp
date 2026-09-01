# Handlers

The default export is a Worker. Add only the handlers you need.

## `fetch` — HTTP

```js
export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url)
    const path = url.pathname.replace(/\/+$/, '') || '/'

    if (path === '/' && request.method === 'GET') {
      return new Response('<h1>store</h1>', {
        headers: { 'content-type': 'text/html; charset=utf-8' },
      })
    }

    if (path === '/api/products' && request.method === 'GET') {
      const { results } = await env.DB.prepare(
        'select id, name, price_cents from products where active = 1',
      ).all()
      return Response.json({ products: results })
    }

    return Response.json({ error: 'not found' }, { status: 404 })
  },
}
```

Gateway routes by **Host** ([Preview](/concepts/preview)). The Worker still sees pathname `/api/products` from `/` (no `/{project}/{version}` prefix). Use relative links in HTML.

`ctx.waitUntil(promise)` is available as celld implements it.

## `scheduled` — Cron

wrangler:

```jsonc
"triggers": { "crons": ["0 * * * *"] }
```

```js
export default {
  async fetch(request, env) { /* … */ },

  async scheduled(controller, env, ctx) {
    await env.CACHE.put('cron:last-run', String(controller.scheduledTime))
  },
}
```

Runs only while the version is **ready** (a live process). Archived versions do not tick. [Cron](/bindings/cron).

## Queues — produce from HTTP

wrangler producers + `env.QUEUE.send`:

```js
await env.FULFILLMENT.send({
  type: 'order.placed',
  order_id: 42,
  at: Date.now(),
})
```

::: warning Producer vs consumer
A queue **consumer** must export `queue()`. If that script has no `fetch`, celld compiles a 404 stub. HTTP `fetch` + `queue()` on **one** module is not the supported cellp pattern — keep a producer Worker for HTTP (`dev/examples/queue`) and a separate consumer if you need `queue()`.
:::

[Queues](/bindings/queues).

## Workflows — class + `create`

```js
import { WorkflowEntrypoint } from 'cloudflare:workers'

export class OrderReport extends WorkflowEntrypoint {
  async run(event, step) {
    return await step.do('summarize orders', async () => {
      const row = await this.env.DB.prepare(
        'select count(*) as orders from orders',
      ).first()
      return { ...row, generated_at: Date.now() }
    })
  }
}

export default {
  async fetch(request, env) {
    if (new URL(request.url).pathname === '/api/report' && request.method === 'POST') {
      const instance = await env.REPORTS.create({ params: { scope: 'orders' } })
      return Response.json({ id: instance.id })
    }
    return new Response('ok')
  },
}
```

`class_name` in wrangler must be `OrderReport`. Preview versions do **not** inherit in-flight instances. [Workflows](/bindings/workflows).

## Durable Objects

```js
export class Counter {
  constructor(state, env) {
    this.state = state
    this.env = env
  }
  async fetch(request) {
    let n = (await this.state.storage.get('n')) ?? 0
    n++
    await this.state.storage.put('n', n)
    return Response.json({ n, version: this.env.VERSION_ID })
  }
}

export default {
  async fetch(request, env) {
    const id = env.COUNTER.idFromName(env.VERSION_ID ?? 'default')
    return env.COUNTER.get(id).fetch(request)
  },
}
```

Repo: `dev/examples/counter`. Confirm gaps in celld compat before production use.

## Platform keys on `env`

Always present, read-only: `PROJECT_ID`, `VERSION_ID`. Your wrangler `vars` and Dashboard overrides sit beside them. [Env](/guides/environment-variables).

## Static HTML

You can `return new Response(html, { headers: { 'content-type': 'text/html' } })` from `fetch`, as the commerce storefront does. wrangler `assets` / Workers Sites work **within celld support** — see [Supported stacks](/migrate/stacks).
