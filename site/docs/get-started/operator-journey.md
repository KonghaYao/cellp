# Operator journey (CLI → Dashboard → promote)

This page is the **closed loop** for a platform operator: deploy a Worker, inspect it in the Dashboard, exercise preview data, promote to production, and know where to roll back. It ties together docs that are otherwise scattered across [Quick start](/get-started/), [Dashboard](/get-started/dashboard), [Preview](/concepts/preview), and [Promote](/concepts/promote).

## What you need

| Piece | Command / URL |
|-------|----------------|
| cellp CLI + celld | [Install](/guides/install) · `cellp doctor` |
| Local platform | `cellp dev --no-deploy` or [Local stack](/get-started/local) (`./dev/scripts/up.sh`) |
| Admin token | Same as `CELLP_ADMIN_TOKEN` / `PLATFORM_TOKEN` in dev |
| Dashboard | From repo root: `pnpm install && pnpm --filter cellp-dashboard dev` → `http://127.0.0.1:5190` with `VITE_CELLP_*` |

There is **no login UI**. The Dashboard sends `Authorization: Bearer` on every API call.

## Operator checklist

Use this list while you run the loop locally. Check items off in your notes or editor; success criteria match Dashboard **Operator checklist** on the project Overview.

- [ ] **Stack & token ready**  
  **Success:** `cellp doctor` clean; `:8790/v1/health` and `:8787/health` OK; `CELLP_ADMIN_TOKEN` matches Dashboard `VITE_CELLP_ADMIN_TOKEN`.  
  **If it fails:** Run `cellp dev --no-deploy` or [Local stack](/get-started/local) (`./dev/scripts/up.sh`); export token from `dev/.env`.

- [ ] **Project exists**  
  **Success:** Project id appears under **Projects** (from **New project** or first deploy).  
  **If it fails:** Dashboard **New project** with valid id, or deploy once with `cellp dev --project <id>`.

- [ ] **Version deployed (CLI/CI)**  
  **Success:** Deployments shows a version; status becomes `ready`.  
  **If it fails:** Dashboard does not upload Workers—run `cellp dev --project <id>` from the app dir or follow [Deploy from CI](/guides/ci); check version `error` via API.

- [ ] **Preview on gateway**  
  **Success:** `curl -sf -H "Host: <preview-host>" http://127.0.0.1:8787/` returns 200 (host from `preview_url`; see [Preview](/concepts/preview)).  
  **If it fails:** Wait for `ready`; use **http** + **:8787** in browser; Clash users need [dev/clash](https://github.com/KonghaYao/cellp/blob/main/dev/clash/README.md) DIRECT for nip.io.

- [ ] **Dashboard walk-through**  
  **Success:** Overview → Deployments → Version detail → Storage (ready version) → Platform (jobs/routes/health).  
  **If it fails:** 401 → fix Bearer token; empty Storage → pick a **ready** version; API down → restart stack.

- [ ] **Promote to production** (skip if this is still the only / first `ready` version — it is already prod)
  **Success:** `prod_version_id` matches the version you want live; **prod Host** (`prod_url`) serves it.
  **If it fails:** Only **ready** versions promote; on preview branches read [What promote does not do](/concepts/promote#what-promote-does-not-do) (pointer switch, not merge of post-fork prod writes).

- [ ] **Rollback (when needed)**  
  **Success:** Promote a previous **ready** version again; prod URL reflects the rollback target.  
  **If it fails:** Version must stay `ready` (not archived/destroyed); see [Rollback](/guides/rollback).

## 1. Register a project (optional)

Projects appear automatically on first deploy, or you can create an empty shell:

- **Dashboard:** Projects → **New project** (id only; optional git remote metadata)
- **API:** `POST /v1/projects` with `{ "id": "my-shop" }`

Neither path uploads a Worker. You still deploy from your app directory or CI.

## 2. Deploy a version (CLI or CI)

From your Worker repo (wrangler manifest + entry):

```bash
cd my-shop
cellp dev --project my-shop
```

Or use CI: build artifact → `POST /v1/projects/my-shop/versions` → poll until `ready`. See [Deploy from CI](/guides/ci).

**Mental model:** a version is **code artifact + isolated data plane** (D1/KV/R2/Queue branch rules in [Platform data](/build/data)).

## 3. Confirm preview on the gateway

```bash
# Preview Host from GET …/versions/<id> → preview_url (not the prod Host)
curl -sf -H "Host: <version-id>.my-shop.ingress.local" http://127.0.0.1:8787/health

# Prod Host: my-shop.ingress.local (see GET /v1/projects/my-shop → prod_url)
curl -sf -H "Host: my-shop.ingress.local" http://127.0.0.1:8787/health
```

**Prod Host** (`GET /projects/{id}` → `prod_url`) serves the current `prod_version_id`. The **first** `ready` version on a project sets that pointer automatically; **later** cutovers require [promote](/concepts/promote). Path URLs `/{project}/{version}/` are deprecated.

## 4. Walk the Dashboard

Suggested click path:

1. **Projects** — find `my-shop`
2. **Overview** — prod pointer, links to prod/preview URLs
3. **Deployments** — status, parent, pin, archive, promote entry
4. **Version detail** — Promote / Destroy, **runtime inspection** strip, Worker env, [preview snapshot notice](/concepts/preview#data-snapshot-timeline) on branch versions
5. **Inspect** (project sidebar) — fleet counts, unhealthy routes, prod bindings summary
6. **Storage** — bindings hub for a **ready** version → D1 browser, KV, Queues, Workflow list
7. **Platform** — pending jobs, active routes (filter by project), gateway error counters

Read-only inspection is enough for most PR reviews. Writes in Storage (SQL, KV put) affect **that version’s bucket only**.

## 5. Promote (when preview is good)

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://127.0.0.1:8790/v1/projects/my-shop/versions/<version-id>/promote"
```

Or **Promote to prod** on the version page.

**Reminder:** promote **switches** `prod_version_id` to the promoted version’s existing bucket. It does **not** merge prod writes that happened **after** the preview was forked. See [What promote does not do](/concepts/promote#what-promote-does-not-do).

PR pipelines should stop before promote; only `main` (or your release branch) should call promote.

## 6. Roll back

Promote a previous **ready** version again, or follow [Rollback](/guides/rollback). Old prod versions remain until archived/destroyed.

## Related

- [Dashboard reference](/get-started/dashboard)
- [Versions & parent_version_id](/concepts/versions)
- [Cron on prod only](/bindings/cron)
- [Observability](/guides/observability) (no built-in tail UI)
