# Environment variables

Env is **per version**, not “Preview / Production / Development” like Vercel.

## Sources (lowest → highest)

| Source | Meaning |
|--------|---------|
| wrangler `vars` | Shipped in the bundle |
| **overrides** | Set via API or Dashboard Settings |
| platform | `PROJECT_ID`, `VERSION_ID` — **read-only** |

`GET /v1/projects/{p}/versions/{v}/env` returns each key with `source`: `wrangler` | `override` | `platform`.

## Set overrides

```bash
curl -sS -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  "$CELLP_URL/v1/projects/my-shop/versions/v1/env" \
  -d '{"vars":{"API_ORIGIN":"https://api.example.com"}}'
```

On a **ready** version this restarts celld so the Worker sees the new `env`. You cannot PUT platform keys.

You can also pass `env` on `POST /versions` so CI injects secrets at deploy time.

## Inheritance

A child version **copies parent overrides** (not platform keys). Then you can change them independently.

## Secrets

The API stores override values as strings. There is no sealed vault inside cellp.

For production secrets:

- Inject from CI / an external vault at `POST /versions`
- Or put a secret manager **in front** of cellp and never persist high-value keys in the registry

Do not treat Dashboard env as a password manager.

## cellpd deploy policy

By default, deploy is **fail-closed**: if offshoot fork/checkpoint/export, D1 import/branch, or KV/R2/Queue branch fails, the version becomes `failed` and must not serve preview traffic. For local debugging only, set `CELLP_LENIENT_DEPLOY=1` on cellpd to warn and continue (not used in CI or e2e).

## Compared to Vercel

| Vercel | cellp |
|--------|--------|
| Project / Preview / Production env | One map per **version** |
| Encrypted Dashboard secrets | Plain strings + CI injection |
| Redeploy to apply | `PUT …/env` restarts that version |
