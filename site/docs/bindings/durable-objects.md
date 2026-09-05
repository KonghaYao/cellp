# Durable Objects

Durable Objects are **optional** on cellp. **celld** can run SQLite-backed classes from wrangler, but support is **partial** compared to Cloudflare. **cellp** does not branch DO state from a parent version like D1/KV/R2/Queue—each ready version has its own celld fleet and storage.

Treat DO as experimental until you have read the runtime notes below.

[Bindings](/concepts/bindings) · [Binding guides](./index) · Wrangler: [Configure bindings](/build/wrangler#durable-objects-optional)

## 1. Declare it

```jsonc
"durable_objects": {
  "bindings": [{ "name": "COUNTER", "class_name": "Counter" }]
},
"migrations": [{ "tag": "v1", "new_sqlite_classes": ["Counter"] }]
```

Export the class from the same module as your Worker (`class_name` must match). Example in the repo: `dev/examples/counter`.

## 2. Use it in the Worker

```js
export class Counter {
  constructor(state, env) {
    this.state = state
  }
  async fetch(request) {
    const n = (await this.state.storage.get('n')) ?? 0
    await this.state.storage.put('n', n + 1)
    return new Response(String(n + 1))
  }
}

export default {
  async fetch(request, env) {
    const id = env.COUNTER.idFromName('global')
    const stub = env.COUNTER.get(id)
    return stub.fetch(request)
  },
}
```

## 3. Operator surfaces

Durable Objects **do not** appear on `GET …/bindings` or in Dashboard Storage. cellp’s binding parser only lists D1, KV, R2, Queues, Workflows, and Cron from wrangler. There is no cellp operator API for DO state — debug through Worker logs and your own metrics.

## 4. Preview vs production

Each **version** runs its own celld process and object bucket. Durable Object storage for a preview version is **not** branched from the parent like D1/KV/R2/Queue. A child version starts with **empty** DO state for that version’s runtime. Plan migrations and IDs accordingly.

## Runtime gaps (celld)

Before shipping production traffic on DO:

- RPC stubs cannot cross isolate boundaries (see celld [RPC notes](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#rpc)).
- Outbound WebSockets do not survive object migration.
- Invalid UTF-8 in SQLite `TEXT` is rejected — store arbitrary bytes in `BLOB`.

Full matrix: [celld cloudflare-compat — Durable Objects](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#durable-objects).

## Related

[Handlers](/build/handlers) · [Supported stacks](/migrate/stacks) · [Images](/bindings/images) (separate partial binding)
