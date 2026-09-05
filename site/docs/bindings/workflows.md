# Workflows

Long-running jobs. You **write a class**, declare it in wrangler, then `create()` from `fetch`. **celld** executes steps; **cellp** does not branch workflow **instances** across preview versions and exposes **read-only** operator APIs.

[Bindings overview](/concepts/bindings) · [Platform data](/build/data) · [Binding guides](./index)

## 1. Declare it

```jsonc
"workflows": [
  {
    "binding": "REPORTS",
    "name": "order-report",
    "class_name": "OrderReport"
  }
]
```

## 2. Implement and start

See the copy-paste in [Handlers](/build/handlers). `class_name` must match the exported class. `this.env` inside the workflow is the same binding set as the Worker.

## 3. cellp: instances and previews

**Instances do not branch.** A child preview version starts with **no** in-flight workflow cells copied from the parent. That prevents a PR preview from resuming production’s half-finished jobs.

The Worker **script** and wrangler workflow binding are whatever you uploaded for **this** version—only **runtime instance state** is empty at fork time.

Dashboard → Storage → Workflows lists instances (**read-only**). cellp does not expose pause, resume, restart, or delete through the API or Dashboard—even when celld’s Worker API supports some lifecycle calls from code.

## Operator API (cellp)

Prefix: `/v1/projects/{project}/versions/{version}/`

```
GET …/workflows
GET …/workflows/{name}/instances
```

There are no write endpoints—cellp wraps `celld cell list` for workflow cells only.

## celld vs Cloudflare

- `create()` can replace a terminal instance with the same ID (Cloudflare rejects duplicates).
- `run()` replays from the start; side effects outside steps may run again after crashes.
- `pause()` / `resume()` / `restart()` exist in celld with documented semantics; `delete()`, `deleteBatch()`, and rollback are **not** available.
- `retention`, `locationHint`, and some step result types are not available.
- Step / parameter payloads are capped at **1 MiB**; non-step work cannot pend more than **60 seconds**.

Details: [celld cloudflare-compat — Workflows](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#workflows).
