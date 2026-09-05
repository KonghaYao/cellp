# Versions

A **version** is one deploy: one Worker artifact, one isolated runtime, one data prefix, one preview URL.

You choose the `id` (commit SHA, `pr-42-a1b2c3`, `v-2026-08-30`, …). cellp does not invent git-based names.

## Create (CI)

```bash
curl -sS -X POST "$CELLP_URL/v1/projects/my-shop/versions" \
  -H "Authorization: Bearer $DEPLOY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "pr-42-a1b2c3",
    "parent_version_id": "v-staging-seed",
    "git_ref": "pr-42",
    "git_sha": "a1b2c3d4",
    "artifact_digest": "sha256:…"
  }'
```

Only `DEPLOY_TOKEN` can create versions. The artifact must already be at `s3://cellp-artifacts/{project}/{version}/`.

| Field | Role |
|-------|------|
| `id` | Version identifier and URL segment |
| `parent_version_id` | If set, **branch** D1/KV/R2/Queue from that version |
| `git_ref` / `git_sha` | Labels for humans and Dashboard. They do **not** route traffic or auto-promote |
| `artifact_digest` | Optional integrity check against the uploaded object |
| `env` | Optional env overrides at create time |

Response is `202` with a poll URL. Wait until `status` is `ready`.

## Status machine

| Status | Meaning |
|--------|---------|
| `pending` → `fetching` → `branching` → `preparing` → `deploying` | Orchestrator at work |
| **`ready`** | Process up, preview URL live |
| **`archived`** | Process stopped, S3 kept. Preview is 503 until [wake](/concepts/archive) |
| `draining` | Leaving the serving set (e.g. during promote) |
| `destroyed` | Gone. Cannot roll back to it |
| `failed` | Inspect `error` on the version payload |

## Initial production

When a project has **no** `prod_version_id` yet, the orchestrator sets production to the **first** version that becomes `ready` (compare-and-set on the empty prod pointer). Later versions stay preview-only until [promote](/concepts/promote).

## Parent vs root

- **Root** (no parent): D1 import / empty bindings as configured for first seed.
- **Child** (has parent): data plane **fork** ([Data fork](/concepts/data-fork)). Worker code comes from **this** bundle.

Do not parent a PR at live production unless you know why. Use a staging seed. The API will 422 or scrub some prod-fork cases.

## One process per ready version

Today, each **`ready`** version runs **one celld process** on its own port. There is no small hard cap on how many ready versions you can have; idle previews are [archived](/concepts/archive) to reclaim processes.

Multi-replica **elastic serving** (scale-to-zero and `0..N` replicas per version) is on the roadmap and ships behind an operator feature flag—**off by default** in current releases. Until you enable that explicitly, plan capacity around one process per hot version.

See [Limits](/reference/limits).

## Pin

`POST …/versions/{id}/pin` keeps a ready version from idle-archive. Unpin when QA is done. See [Archive](/concepts/archive).

## Destroy

`DELETE …/versions/{id}` is irreversible. Prefer archive if you might need the data again.
