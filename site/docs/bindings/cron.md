# Cron

Cron is declared in wrangler and implemented as `scheduled` on the Worker. celld fires it. cellp only **shows** the expression.

## 1. Declare it

```jsonc
"triggers": { "crons": ["0 * * * *"] }
```

## 2. Handle it

```js
export default {
  async fetch(request, env) { /* HTTP */ },

  async scheduled(controller, env, ctx) {
    await env.CACHE.put('cron:last-run', String(controller.scheduledTime))
  },
}
```

[Handlers](/build/handlers)

## 3. When it runs

Only while the version is **ready**. Archive stops the process → no ticks. Production cron is whatever the **promoted** version declared. Cron **does not branch**.

Dashboard lists expressions; there is no “run now”.

Some Cloudflare cron syntax is rejected (e.g. descending ranges). Details: [celld cron notes](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

