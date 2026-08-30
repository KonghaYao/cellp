# KV

Workers KV on `env.<binding>`.

## 1. Declare it

```jsonc
"kv_namespaces": [
  {
    "binding": "CACHE",
    "id": "my-shop-cache"
  }
]
```

`id` is the namespace identity — keep it on child wranglers. [Configure bindings](/build/wrangler).

## 2. Use it in the Worker

```js
export default {
  async fetch(request, env) {
    const url = new URL(request.url)

    if (url.pathname === '/api/banner' && request.method === 'GET') {
      const value = (await env.CACHE.get('store:banner')) ?? ''
      return Response.json({ value })
    }

    if (url.pathname === '/api/banner' && request.method === 'PUT') {
      const { value } = await request.json()
      await env.CACHE.put('store:banner', String(value ?? ''))
      return Response.json({ ok: true })
    }

    return new Response('not found', { status: 404 })
  },
}
```

Commerce uses `CACHE` for cart + store banner (`dev/examples/commerce`).

## 3. Put keys in

| Method | When |
|--------|------|
| Worker `put` | App logic |
| Dashboard → Storage → KV | Flags, banners, one-off edits |
| Child version | **Branch** from parent (preview writes stay isolated) |

Root versions start **empty**. There is no “create namespace” button besides wrangler. [Platform data](/build/data).

## Operator API

```
GET    …/kv
GET    …/kv/{ns}                 # {ns} is the wrangler id
GET    …/kv/{ns}/keys
GET    …/kv/{ns}/keys/{key}
PUT    …/kv/{ns}/keys/{key}
DELETE …/kv/{ns}/keys/{key}
```

## Gaps

celld KV is **partial** vs Cloudflare. Check list/metadata/limits against [celld compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) if you depend on them.
