# Deploy from CI

You already have a Worker folder (`wrangler.jsonc` + `main`). cellp never watches git. CI is: **put that folder in object storage → POST version → poll → optional promote**.

If you have not written the app yet, start at [Write a Worker](/build/) and [Configure bindings](/build/wrangler).

API field reference: [REST API](/reference/api). Machine-readable contract: [`cellp/api/openapi.yaml`](https://github.com/KonghaYao/cellp/blob/main/cellp/api/openapi.yaml).

## Artifact layout

Upload the wrangler bundle so cellpd can fetch it:

```text
s3://cellp-artifacts/{project}/{version}/
```

Use path-style S3 against **RustFS** (`AWS_ENDPOINT` / `--endpoint-url`). Not AWS S3, not Cloudflare R2.

Upload **before** calling create-version, or finish upload immediately after — the orchestrator fetches from this prefix.

## Create the version

`POST /v1/projects/{project}/versions` with **`CELLP_DEPLOY_TOKEN`** (not admin when tokens are split).

Child (PR) example body:

```json
{
  "id": "pr-42-a1b2c3d",
  "parent_version_id": "v-staging-seed",
  "git_ref": "pr-42",
  "git_sha": "a1b2c3d4e5f6",
  "artifact_digest": "sha256:…"
}
```

**202 Accepted** body includes `poll_url`. Poll `GET` on that version until `status` is `ready` or `failed`. Use **`CELLP_ADMIN_TOKEN`** for GET (deploy token returns **403** when deploy ≠ admin). Locally both are often `dev-local-token`.

If the queue is saturated, POST returns **503** `queue_full` — back off and retry.

## GitHub Actions (PR preview)

A full workflow lives in the repo: [`dev/examples/ci-pr-preview.example.yml`](https://github.com/KonghaYao/cellp/blob/main/dev/examples/ci-pr-preview.example.yml).

Sketch:

```yaml
# build job: pnpm install --frozen-lockfile && pnpm run build, aws s3 cp bundle to
# s3://cellp-artifacts/$PROJECT/$VERSION/

# preview job:
- name: Deploy preview
  env:
    CELLP_DEPLOY_TOKEN: ${{ secrets.CELLP_DEPLOY_TOKEN }}
  run: |
    curl -sf -X POST "$CELLP_URL/projects/$PROJECT/versions" \
      -H "Authorization: Bearer $CELLP_DEPLOY_TOKEN" \
      -H "Content-Type: application/json" \
      -d "$BODY"
    # poll until ready with CELLP_ADMIN_TOKEN, print preview_url
```

Secrets you need:

| Secret / var | Purpose |
|--------------|---------|
| `CELLP_URL` | API origin **including `/v1`**, e.g. `https://cellp.internal/v1` |
| `CELLP_DEPLOY_TOKEN` | `POST /versions` only |
| `CELLP_ADMIN_TOKEN` | Poll GET, promote, Dashboard |
| RustFS endpoint + keys | Artifact upload |

Commenting the preview URL on the PR is **your** GitHub token / bot. cellp will not.

## Production pipeline

Same upload + `POST /versions` **without** treating it as a throwaway PR id. After smoke tests:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/projects/$PROJECT/versions/$VERSION/promote"
```

Keep `ADMIN_TOKEN` out of PR workflows from forks.

## Parent choice

| Pipeline | `parent_version_id` |
|----------|---------------------|
| First seed | omit (root) + D1 import / seed scripts |
| PR | a **pinned staging** version |
| Prod candidate | staging or last good prod — team policy |

Avoid parenting every PR at live production.

PR previews inherit the parent’s data **at fork time** only. Orders or config written to **production after** the preview was created are **not** merged in when you later promote that preview — see [Preview data timeline](/concepts/preview#data-snapshot-timeline).

## Local stand-in

Copy artifacts into your dev layout and call the same `POST /versions` with curl, or use `cellp dev` in a Worker directory for a laptop loop. For a scripted CD rehearsal, see `dev/scripts/simulate-cd.sh` in the repository (creates a **root** version from an example Worker).
