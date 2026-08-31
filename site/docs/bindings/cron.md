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

Only while the version is **ready** and it is the project **production** version (`prod_version_id`). Preview ready versions still **list** cron expressions in bindings (from wrangler), but cellp does not arm schedules on them — only prod ticks. After **promote**, the new prod version is redeployed with crons enabled; the old prod (if still ready) stops scheduling.

Archive stops the process → no ticks. Cron **does not branch**.

Dashboard lists expressions; there is no “run now”.

Some Cloudflare cron syntax is rejected (e.g. descending ranges). Details: [celld cron notes](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

