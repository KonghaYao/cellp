# Cron

Cron is declared in wrangler and implemented as `scheduled` on the Worker. **celld** runs the handler when a schedule is **armed** on that version’s fleet. **cellp** decides which ready versions arm `triggers.crons` at deploy time so preview fleets do not multiply production side effects.

[Bindings overview](/concepts/bindings) · [Binding guides](./index)

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

## 3. cellp scheduling policy

Cron **does not branch** data—there is no forked “cron state.” What changes per version is whether cellp passes `triggers.crons` into **celld deploy** for that version.

| Project state | Version | `scheduled` runs? |
|---------------|---------|-------------------|
| **No production yet** (`prod_version_id` empty) | Any version that deploys **ready** | **Yes** — cellp arms crons on deploy for that version |
| **Production set** | Version id **equals** `prod_version_id` | **Yes** |
| **Production set** | Preview (any other ready version) | **No** — deploy uses a wrangler view **without** `triggers.crons`; artifact on disk is unchanged |
| Any | **Archived** or not `ready` | **No** — process stopped |

Implications:

- While **`prod_version_id` is empty**, any version that deploys **ready** may arm crons; the **first** `ready` version normally becomes production and sets that pointer. If several ready versions exist before then, more than one fleet may arm the same expression—keep one ready line or cut over promptly if you need a single scheduler. After production exists, **only** the prod version arms.
- `GET …/bindings` still shows cron expressions from the **artifact** wrangler even when preview deploy stripped them for celld.
- After **promote**, cellp **redeploys and restarts** the old and new production versions (when still `ready`) so only the new prod fleet arms crons.

Dashboard lists expressions; there is no “run now” in cellp.

## celld vs Cloudflare

- Rejects descending ranges (`SAT-SUN`, `NOV-FEB`) and `*` inside a list (`1,*`).
- After fleet downtime, celld runs the **most recent missed** occurrence once.
- One handler at a time per script; failures retry until the next tick unless the handler calls `noRetry()`.
- A **service-binding** target cannot run its own Cron Triggers.

Full notes: [celld cloudflare-compat — Cron Triggers](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#cron-triggers).
