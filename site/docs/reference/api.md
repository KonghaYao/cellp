# REST API

Base URL: `http://<cellpd>:8790/v1`

Full machine contract: [`cellp/api/openapi.yaml`](https://github.com/KonghaYao/cellp/blob/main/cellp/api/openapi.yaml) in the repo. This page is the operator map.

## Auth

```http
Authorization: Bearer <token>
```

| Token | Endpoints |
|-------|-----------|
| `CELLP_DEPLOY_TOKEN` | `POST /projects/{id}/versions` only |
| `CELLP_ADMIN_TOKEN` | Everything else — list/get versions, poll deploy status, promote, bindings, Dashboard |

If deploy and admin tokens differ, CI must use **admin** (or a second credential) to `GET …/versions/{id}` while polling; deploy token alone cannot read version status.

Details: [Auth](/reference/auth).

## Health & runtime

REST paths below are under **`/v1`**. Prometheus metrics are **not** under `/v1`:

| Method | Path | Auth | Notes |
|--------|------|------|--------|
| `GET` | `/v1/health` | none | Liveness |
| `GET` | `/v1/health/deep` | none | Registry, store, queue; `503` if overloaded (`queue_full`) |
| `GET` | `http://<cellpd>:8790/metrics` | none | Prometheus (cellpd **root**, no `/v1` prefix) |
| `GET` | `/v1/runtime/routes` | admin | Active upstream summary |

## Projects

| Method | Path | Auth |
|--------|------|------|
| `GET` | `/projects` | admin |
| `POST` | `/projects` | admin |
| `GET` | `/projects/{projectID}` | admin |

Query `GET /projects`: `limit` (default 50, max 200), `cursor` for pagination.

`GET /projects/{projectID}`: optional `include=versions` with `limit` for an embedded first page.

`POST` body: `{ "id": "my-shop", "git_remote": "optional" }`.

Response includes `prod_version_id`, `prod_url` when production is set.

## Versions

| Method | Path | Auth | Notes |
|--------|------|------|--------|
| `GET` | `/projects/{p}/versions` | admin | `limit`, `cursor`, optional `status` filter |
| `POST` | `/projects/{p}/versions` | **deploy** | **202** — async deploy |
| `GET` | `/projects/{p}/versions/{v}` | admin | Status, `preview_url`, `prod_url`, parent, … |
| `DELETE` | `/projects/{p}/versions/{v}` | admin | Destroy (irreversible) |
| `GET` `PUT` | `/projects/{p}/versions/{v}/env` | admin | Worker env overrides |
| `POST` | `…/promote` | admin | Production cutover (**202**) |
| `POST` | `…/archive` `…/wake` | admin | Process lifecycle |
| `POST` | `…/pin` `…/unpin` | admin | Idle protection |

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

- `id` optional — server generates `v-YYYYMMDDHHMMSS` if omitted.
- `artifact_uri` in client JSON is **ignored**; cellpd builds `s3://{CELLP_ARTIFACTS_BUCKET}/{project}/{version}/`.
- Upload artifacts to that prefix **before** or immediately after POST (orchestrator fetches from object storage).

### POST /versions responses

| Code | Meaning |
|------|---------|
| **202** | Accepted — body includes `id`, `status`, `preview_url`, `poll_url` |
| **422** | Invalid fork (e.g. parenting live production for a PR) |
| **503** | Deploy queue full (`queue_full`, `pending_jobs`, `queue_max`) |

Poll `GET …/versions/{id}` until `status` is `ready` or `failed` (you may see `deploy_ready` transiently when elastic runtime is on—keep polling until `ready`; see table above).

### Version `status` values

| Status | Meaning |
|--------|---------|
| `pending` | Created, queued |
| `fetching` | Pulling artifact |
| `branching` | offshoot / D1 / KV / R2 / Queue branch |
| `preparing` | Import / seed |
| `deploying` | celld deploy |
| `deploy_ready` | Artifact and bindings prepared; may appear only when [elastic serving](/reference/limits) (`CELLP_ELASTIC_RUNTIME`) is enabled—qualification before `ready`. **Poll until `ready`** for deploy success; do not treat `deploy_ready` as the public CD terminal state. |
| `ready` | Serves preview Host (external deploy success terminal state) |
| `failed` | Deploy error — see `error` field |
| `archived` | Process stopped; data retained |
| `draining` | Transient during promote |
| `destroyed` | Removed |

### GET /versions/{v} fields (snapshot semantics)

| Field | Meaning |
|-------|---------|
| `parent_version_id` | When set, this version **branched data** (D1/KV/R2/Queue) from that parent at deploy time. The child does not see parent writes after the fork cut. `null` = root version. |
| `ready_at` | When the version became `ready`. |
| `preview_url` | Outward Gateway URL for preview Host (scheme/host from ingress config). |

Concepts: [Preview](/concepts/preview) · [Promote](/concepts/promote).

## Gateway (not `/v1`)

User traffic hits **cellpd Gateway** on port **8787** (default). Routing is by **HTTP Host** ([Preview & production](/concepts/preview)):

| Role | Host pattern |
|------|----------------|
| Preview | `{version}.{project}.{baseDomain}` |
| Production | `{project}.{baseDomain}` |

Configure `CELLP_INGRESS_BASE_DOMAIN`, `GATEWAY_URL`, and public schemes on cellpd. Dev setup: [repo `dev/INGRESS-HOST.md`](https://github.com/KonghaYao/cellp/blob/main/dev/INGRESS-HOST.md). Path selectors `http://gateway:8787/{project}/` and `/{project}/{version}/` are **deprecated**.

**WebSockets:** The gateway forwards `Upgrade: websocket` to celld (RFC 6455). Use the same Host rules as HTTP. TLS terminates at your outer proxy, not inside cellpd.

```bash
curl -H "Host: demo-app.ingress.local" http://127.0.0.1:8787/health
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/projects/demo-app/versions/v1" | jq .preview_url
```

## Bindings & data

Prefix: `/projects/{p}/versions/{v}` — all **admin**.

| Area | Paths |
|------|--------|
| Manifest | `GET /bindings` |
| D1 | `GET /database`, `/database/tables`, `/database/tables/{table}/rows`, `POST /database/query` |
| KV | `GET /kv`, `/kv/{ns}`, `/kv/{ns}/keys`, `GET|PUT|DELETE /kv/{ns}/keys/{key}` |
| Queues | `/queues`, `/queues/{name}`, `GET /peek`, `POST /pause` `/resume` `/redrive` `/purge` |
| Workflows | `GET /workflows`, `GET /workflows/{name}/instances` (read-only) |

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
