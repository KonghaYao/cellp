# Auth & tokens

cellp has **no users**. Two bearer tokens cover the product.

## Tokens

| Name | Env | Purpose |
|------|-----|---------|
| **Deploy** | `CELLP_DEPLOY_TOKEN` | Create versions (`POST /versions`). CI. |
| **Admin** | `CELLP_ADMIN_TOKEN` | Projects, promote, archive, bindings, env, Dashboard |

Local defaults are `dev-local-token`. **Change them** before anything is reachable beyond localhost.

## How to send

```http
Authorization: Bearer <token>
```

There is no OAuth, API keys per human, or scoped PATs. If you need that, terminate at **your** reverse proxy (SSO, mTLS, IP allowlist) and inject the admin token toward cellpd.

## Dashboard

The Dashboard stores or sends the **admin** token to `:8790`. Treat the Dashboard as an admin surface. Do not hang it on the public internet with only the default token.

## Git identity

`git_sha` / `git_ref` on a version are labels. They are not authentication and not an audit log of “who clicked deploy.” Your CI system is the system of record for who pushed.

## Multi-tenant

Not a cellp feature. Run separate cellp deployments, or put a tenant-aware layer in front that maps to projects. Do not expect orgs in the registry.
