# Queues

Produce messages with `env.<binding>.send`. Consume with a **separate** script — not the same module as `fetch`.

## 1. Declare a producer

```jsonc
"queues": {
  "producers": [{ "binding": "FULFILLMENT", "queue": "fulfillment" }]
}
```

`queue` is the resource name (keep it on child wranglers). `binding` is `env.FULFILLMENT`. [Configure bindings](/build/wrangler).

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

::: warning Producer vs consumer
A consumer must export `queue()`. HTTP `fetch` + `queue()` on one script is not the supported cellp pattern. Keep a producer Worker for HTTP (`dev/examples/queue`) and a separate consumer if you need `queue()`.
:::

## 3. What you see after deploy

- Dashboard → Storage → Queues: peek, pause, resume, redrive, purge
- Child versions **branch** the queue from the parent
- Root starts empty

[Platform data](/build/data) · [Handlers](/build/handlers)

## Operator API

```
GET  …/queues
GET  …/queues/{name}
GET  …/queues/{name}/peek
POST …/queues/{name}/pause
POST …/queues/{name}/resume
POST …/queues/{name}/redrive
POST …/queues/{name}/purge
```

celld Queues are **partial** vs Cloudflare. Confirm batching, retries, and DLQ before a billing pipeline depends on them.
