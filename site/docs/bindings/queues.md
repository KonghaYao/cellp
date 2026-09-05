# Queues

Produce messages with `env.<binding>.send`. Consume with a **separate** script — not the same module as `fetch`. **celld** enforces one consumer Worker per queue; **cellp** **branches** queue data on child preview versions and exposes peek/pause/redrive operator APIs.

On cellp, producer and consumer are usually **two deploy artifacts**—often **two versions** in the same project—because multi-`[[services]]` in one wrangler bundle is not supported.

[Bindings overview](/concepts/bindings) · [Binding guides](./index) · [Platform data](/build/data)

## 1. Declare a producer

```jsonc
"queues": {
  "producers": [{ "binding": "FULFILLMENT", "queue": "fulfillment" }]
}
```

`queue` is the resource name (keep it on child wranglers — names must match the parent or Start fails). `binding` is `env.FULFILLMENT`. [Configure bindings](/build/wrangler).

Producer-only example: `dev/examples/queue`.

## 2. Send from the HTTP Worker

```js
export default {
  async fetch(request, env) {
    if (request.method === 'POST' && new URL(request.url).pathname === '/api/orders') {
      const order = await createOrder(env.DB, await request.json())
      await env.FULFILLMENT.send({
        type: 'order.placed',
        order_id: order.id,
        at: Date.now(),
      })
      return Response.json(order, { status: 201 })
    }
    return new Response('ok')
  },
}
```

## 3. Declare a consumer (separate deploy)

Deploy a **second** Worker artifact with only `queue()` — no `fetch()`:

```jsonc
"queues": {
  "consumers": [{ "queue": "fulfillment", "max_batch_size": 10 }]
}
```

```js
export default {
  async queue(batch, env) {
    for (const msg of batch.messages) {
      await handle(msg.body, env)
      msg.ack()
    }
  },
}
```

::: warning Producer vs consumer
celld allows **one consumer script per queue**, and that script **cannot** export `fetch()`. HTTP and consumption must be two Workers (or HTTP-only with peek/redrive from the operator API).
:::

## 4. What you see after deploy

- Dashboard → Storage → Queues: peek, pause, resume, redrive, purge
- Child versions **branch** the queue from the parent
- Root starts empty

[Handlers](/build/handlers)

## Operator API

Prefix: `/v1/projects/{project}/versions/{version}/`

```
GET  …/queues
GET  …/queues/{name}
GET  …/queues/{name}/peek?limit=
POST …/queues/{name}/pause
POST …/queues/{name}/resume
POST …/queues/{name}/redrive?limit=
POST …/queues/{name}/purge
```

## celld vs Cloudflare

- Messages are retained **four days** (not configurable).
- Pull consumers and the Cloudflare Queues HTTP API are not available.
- One writer per queue; one consumer script per queue.
- Confirm batching, retries, and dead-letter behavior in [celld compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#queues) before production billing pipelines depend on queues.
