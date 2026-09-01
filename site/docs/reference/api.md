# REST API

Base URL: `http://<cellpd>:8790/v1`

Full machine contract: [`cellp/api/openapi.yaml`](https://github.com/KonghaYao/cellp/blob/main/cellp/api/openapi.yaml) in the repo. This page is the operator map.

## Auth

```http
Authorization: Bearer <token>
```

| Token | Endpoints |
|-------|-----------|
| `DEPLOY_TOKEN` | `POST /projects/{id}/versions` |
| `ADMIN_TOKEN` | Everything else (including GET version for polling if you choose admin; CI often uses deploy token only on POST — poll with whichever your deployment grants) |

Details: [Auth](/reference/auth).

## Health & runtime

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/health` | Liveness |
| `GET` | `/health/deep` | Registry, store, queue; `503` if overloaded |
| `GET` | `/metrics` | Prometheus |
| `GET` | `/runtime/routes` | Upstream summary (admin) |

## Projects

| Method | Path |
|--------|------|
| `GET` | `/projects` |
| `POST` | `/projects` |
| `GET` | `/projects/{projectID}` |

`POST` body: `{ "id": "my-shop", "git_remote": "optional" }`.

## Versions

| Method | Path | Notes |
|--------|------|--------|
| `GET` | `/projects/{p}/versions` | Cursor pagination |
| `POST` | `/projects/{p}/versions` | **202** — CD entry (`DEPLOY_TOKEN`) |
| `GET` | `/projects/{p}/versions/{v}` | Status, `preview_url`, … |
| `DELETE` | `/projects/{p}/versions/{v}` | Destroy |
| `GET` `PUT` | `/projects/{p}/versions/{v}/env` | Overrides |
| `POST` | `…/promote` | Production cutover |
| `POST` | `…/archive` `…/wake` | Process lifecycle |
| `POST` | `…/pin` `…/unpin` | Idle protection |

### POST /versions body

```json
{
  "id": "pr-42-abc",
  "parent_version_id": "v-staging-seed",
  "git_ref": "pr-42",
  "git_sha": "abc123",
  "artifact_digest": "sha256:…",
  "env": { "FEATURE": "1" }
}
```

`422` example: illegal prod fork.

Poll until `status` is `ready` or `failed`.

### GET /versions/{v} fields (snapshot semantics)

| Field | Meaning |
|-------|---------|
| `parent_version_id` | When set, this version **branched data** (D1/KV/R2/Queue) from that parent at deploy time. The child does not see parent writes after the fork cut (`fork_txid` in D1; not exposed in JSON). `null` = root version. |
| `ready_at` | When the version became `ready` (deploy + branch pipeline finished). |
| `preview_url` | Outward Gateway URL for preview Host (`http://{version}.{project}.{base}:8787/` in dev). |

Concepts: [Preview](/concepts/preview) · [Promote](/concepts/promote).

## Gateway (not `/v1`)

User traffic hits **cellpd Gateway** on port **8787** (default). Routing is by **HTTP Host** ([AD-12](/concepts/preview)):

| Role | Host pattern |
|------|----------------|
| Preview | `{version}.{project}.{baseDomain}` |
| Production | `{project}.{baseDomain}` |

Dev setup: [repo `dev/INGRESS-HOST.md`](https://github.com/KonghaYao/cellp/blob/main/dev/INGRESS-HOST.md). Path selectors `http://gateway:8787/{project}/` and `/{project}/{version}/` are **deprecated**.

```bash
curl -H "Host: demo-app.ingress.local" http://127.0.0.1:8787/health
curl -sS -H "Authorization: Bearer $TOKEN" \
  "$CELLP_URL/projects/demo-app/versions/v1" | jq .preview_url
# open preview_url in browser (http + :8787 in dev)
```

## Bindings & data

Prefix: `/projects/{p}/versions/{v}`

| Area | Paths |
|------|--------|
| Manifest | `GET /bindings` |
| D1 | `GET /database`, `/database/tables`, `/database/tables/{table}/rows`, `POST /database/query` |
| KV | `/kv`, `/kv/{ns}`, `/kv/{ns}/keys`, `/kv/{ns}/keys/{key}` |
| Queues | `/queues`, `/queues/{name}`, `peek` `pause` `resume` `redrive` `purge` |
| Workflows | `/workflows`, `/workflows/{name}/instances` |

## curl: first version smoke

```bash
export CELLP_URL=http://127.0.0.1:8790/v1
export TOKEN=dev-local-token

curl -sS "$CELLP_URL/health"
curl -sS -H "Authorization: Bearer $TOKEN" "$CELLP_URL/projects"
curl -sS -H "Authorization: Bearer $TOKEN" \
  "$CELLP_URL/projects/commerce-store/versions/v1"
```

Use `preview_url` / `prod_url` from the API for Gateway checks (Host routing, not `/v1`).
