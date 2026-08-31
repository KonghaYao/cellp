# Operator journey (CLI → Dashboard → promote)

This page is the **closed loop** for a platform operator: deploy a Worker, inspect it in the Dashboard, exercise preview data, promote to production, and know where to roll back. It ties together docs that are otherwise scattered across [Quick start](/get-started/), [Dashboard](/get-started/dashboard), [Preview](/concepts/preview), and [Promote](/concepts/promote).

## What you need

| Piece | Command / URL |
|-------|----------------|
| cellp CLI + celld | [Install](/guides/install) · `cellp doctor` |
| Local platform | `cellp dev --no-deploy` or [Local stack](/get-started/local) (`./dev/scripts/up.sh`) |
| Admin token | Same as `CELLP_ADMIN_TOKEN` / `PLATFORM_TOKEN` in dev |
| Dashboard | `cd web && npm run dev` → `http://127.0.0.1:5190` with `VITE_CELLP_*` |

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
  **Success:** `curl -sf "http://127.0.0.1:8787/<project>/<version-id>/"` returns 200 (or your app’s expected body).  
  **If it fails:** Wait for `ready`; confirm gateway URL shape `/{project}/{version}/` (not prod path until promote).

- [ ] **Dashboard walk-through**  
  **Success:** Overview → Deployments → Version detail → Storage (ready version) → Platform (jobs/routes/health).  
  **If it fails:** 401 → fix Bearer token; empty Storage → pick a **ready** version; API down → restart stack.

- [ ] **Promote to production**  
  **Success:** `prod_version_id` matches promoted version; `/{project}/` serves prod.  
  **If it fails:** Only **ready** versions promote; on preview branches read [What promote does not do](/concepts/promote#what-promote-does-not-do) (pointer switch, not merge of post-fork prod writes).

- [ ] **Rollback (when needed)**  
  **Success:** Promote a previous **ready** version again; prod URL reflects the rollback target.  
  **If it fails:** Version must stay `ready` (not archived/destroyed); see [Rollback](/guides/rollback).

- [ ] **Contributor verify**  
  **Success:** `cd web && npm run test` green; optional `./dev/scripts/up.sh` then `cd web && npm run test:e2e:live`.  
  **If it fails:** Vitest logs under `web/`; live skip OK if stack down—use `web/scripts/verify-user-loop.sh` for the standard gate + log in `docs/evidence/`.

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
curl -sf "http://127.0.0.1:8787/my-shop/<version-id>/"
```

Preview URL shape: `/{project}/{version}/`. Production is `/{project}/` only after promote.

## 4. Walk the Dashboard

Suggested click path (matches live E2E `web/e2e/live/operator-loop.spec.ts`):

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

## 7. Verify (contributors)

**Fast (Vitest, mock API):**

```bash
cd web && npm run test
```

Covers create-project, preview snapshot copy, promote confirm, and navigation helpers under `web/src/flows/`.

**Browser mock (Playwright):**

```bash
cd web && npm run test:e2e
```

**Live cellpd:**

```bash
cd web && npm run test:e2e:live
```

Uses real `:8790` (not the Playwright mock). Override project id: `CELLP_LIVE_PROJECT=commerce-store`.

## Related

- [Dashboard reference](/get-started/dashboard)
- [Versions & parent_version_id](/concepts/versions)
- [Cron on prod only](/bindings/cron)
- [Observability](/guides/observability) (no built-in tail UI)
