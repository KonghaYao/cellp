# Auth & tokens

cellp has **no users**. Two bearer tokens cover the product.

## Tokens

| Name | Env | Purpose |
|------|-----|---------|
| **Deploy** | `CELLP_DEPLOY_TOKEN` | `POST /v1/projects/{project}/versions` **only** |
| **Admin** | `CELLP_ADMIN_TOKEN` | All other `/v1/*` routes (projects, GET versions, promote, bindings, env, Dashboard) |

**Legacy alias:** if `CELLP_DEPLOY_TOKEN` or `CELLP_ADMIN_TOKEN` is unset, cellpd falls back to `PLATFORM_TOKEN` for that role (common in older Compose examples).

Local defaults are `dev-local-token` for both. **Change them** before anything is reachable beyond localhost.

## Strict separation (recommended for CI)

When deploy and admin secrets **differ**:

| Credential | Allowed | Forbidden |
|------------|---------|-----------|
| Deploy token | `POST …/versions` | `GET …/versions/{id}`, promote, bindings → **403** |
| Admin token | All routes **except** `POST …/versions` when deploy ≠ admin | `POST …/versions` → **403** |

When both env vars are the **same** value (typical local dev), one token can create and poll.

CI should use **deploy** for upload triggers and **admin** (or a separate read-only automation secret mapped to admin at your proxy) for polling `GET …/versions/{id}`.

## How to send

```http
Authorization: Bearer <token>
```

There is no OAuth, API keys per human, or scoped PATs. If you need that, terminate at **your** reverse proxy (SSO, mTLS, IP allowlist) and inject the admin token toward cellpd.

## Dashboard

Build-time (`web/.env`):

| Variable | Purpose |
|----------|---------|
| `VITE_CELLP_API_URL` | cellpd origin **without** `/v1` (e.g. `http://127.0.0.1:8790`) |
| `VITE_CELLP_ADMIN_TOKEN` | Admin bearer token (required for normal Dashboard API calls) |
| `VITE_CELLP_DEPLOY_TOKEN` | Optional. Used **only** when the Dashboard calls `POST …/versions` (e.g. create/branch version from the UI). If unset, that call falls back to the admin token (fine when deploy and admin are the same). When tokens are **separated**, set this only if you use that UI path—same privilege class as `CELLP_DEPLOY_TOKEN` in CI. **Do not** ship a deploy token in a browser build you expose on the public internet; prefer CLI/CI for creates and keep the Dashboard admin-only. |
| `VITE_CELLP_GATEWAY_URL` | Optional public gateway origin for links |
| `VITE_CELLP_INGRESS_BASE_DOMAIN` | Must match cellpd `CELLP_INGRESS_BASE_DOMAIN` |

The Dashboard sends the **admin** token on almost every `:8790` request. Treat it as an admin surface. Do not expose it on the public internet with only the default token.

## Git identity

`git_sha` / `git_ref` on a version are labels. They are not authentication and not an audit log of “who clicked deploy.” Your CI system is the system of record for who pushed.

## Multi-tenant

Not a cellp feature. Run separate cellp deployments, or put a tenant-aware layer in front that maps to projects. Do not expect orgs in the registry.
