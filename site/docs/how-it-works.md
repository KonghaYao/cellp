# How it works

You do not push git to cellp. You **push a version**. On a laptop that is `cellp dev`. In CI it is upload + `POST /versions`.

## The loop

<Flow :steps="[
  'Write a Worker + wrangler.jsonc. Locally: cellp dev. In CI: upload the folder to RustFS',
  'POST /v1/projects/{project}/versions with id, optional parent_version_id, and artifact digest',
  'Poll GET …/versions/{id} until status is ready (or failed)',
  'Open preview_url on the gateway (HTTP Host, e.g. v1.my-shop.ingress.local:8787)',
  'When it looks right, POST …/versions/{id}/promote — prod Host points at that version (skip promote for the first version on a new project; it becomes prod when ready)'
]" />

That is the entire CD contract. GitHub Actions, GitLab CI, or a laptop script are interchangeable.

## cellp vs celld

| | **cellp** (control plane) | **celld** (runtime) |
|--|---------------------------|---------------------|
| **You install** | `cellp` / `cellpd`, Dashboard, RustFS | Bundled with cellp releases (or built from `celld/` in the repo) |
| **Responsibility** | Versions, registry, gateway Host routing, promote saga, data fork orchestration | Execute Worker `fetch`, bindings, Cron, Workflows inside the isolate |
| **Process model** | One `cellpd` per deployment | One **celld** child process per **ready** version |

Application code never imports cellp. It only sees the Workers APIs celld provides. Platform automation talks to **`:8790`**; browsers and CI smoke tests use **`:8787`** with the correct **Host** ([Gateway routing](/concepts/routing)).

## Isolation model

Every **ready** version is:

- Its own **celld process** (own port)
- Its own **S3 prefix** (`s3://cellp-celld/{project}/{version}`)
- Its own gateway route

Two versions of the same project never share a runtime. That is how preview stays safe next to production.

Local watch directories are **throwaway page cache**. Object storage is the source of truth. If a version is stopped, the cache goes away; S3 still has the data.

## Parent versions (the fork)

If you omit `parent_version_id`, you get a **root** version: empty-ish bindings, D1 imported if you seeded it.

If you set `parent_version_id`, cellp treats this deploy as a **child**:

| Binding | Child version |
|---------|----------------|
| Worker script | From **this** artifact (not a diff of the parent) |
| D1 | Branch from parent (copy-on-write LTX) |
| KV / R2 / Queue | Branch from parent |
| Workflow instances / Cron | Do **not** branch — start empty / from this script |

Typical PR preview: parent is a **staging seed**, not live production. Forking prod is rejected or scrubbed on purpose. See [Data fork](/concepts/data-fork).

## URLs

The gateway is a reverse proxy inside cellpd ([Host-based routing](/concepts/routing)):

| URL | Meaning |
|-----|---------|
| `http://{version}.{project}.{base}:8787/` | This version (preview) |
| `http://{project}.{base}:8787/` | Current production version |
| `POST /v1/.../promote` | Atomically point production at a ready version |

Path selectors `/{project}/{version}/` and `/{project}/` are **deprecated** — see [Gateway routing](/concepts/routing). Dev hosts: [INGRESS-HOST.md](https://github.com/KonghaYao/cellp/blob/main/dev/INGRESS-HOST.md).

TLS and custom domains sit **in front** of the gateway. cellp speaks HTTP on `:8787` in dev.

## Promote, without magic

Promote is a saga, not a DNS flip:

1. Validate the version is ready
2. Drain the old production route
3. Promote data-plane pointers
4. CAS-update `prod_version_id`
5. Activate the new production route

If a step fails, the orchestrator compensates in reverse. Window target is a couple of seconds, not a blue-green cluster dance.

Promote **only** moves the production pointer to the promoted version’s bucket. It does **not** merge data from the old prod line into that bucket after a fork. See [What promote does not do](/concepts/promote#what-promote-does-not-do).

Rollback is the same API: promote the previous version (wake it first if it was archived). See [Rollback](/guides/rollback).

## Idle versions

Ready versions consume a process. Idle previews are **archived**: process stopped, S3 kept, preview returns `503`. `POST …/wake` brings it back. Pin a version if QA needs it to stay hot. Production is never auto-archived.

## Tokens

| Token | Used for |
|-------|----------|
| `DEPLOY_TOKEN` | `POST /versions` only (CI) |
| `ADMIN_TOKEN` | Everything else + Dashboard |

There is no user table. See [Auth](/reference/auth).

## Where to go next

- [Gateway routing](/concepts/routing) · [Data fork](/concepts/data-fork)
- [Concepts: Versions](/concepts/versions)
- [Deploy from CI](/guides/ci)
- [Bindings](/concepts/bindings)
