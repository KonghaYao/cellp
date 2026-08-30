# Deploy from CI

You already have a Worker folder (`wrangler.jsonc` + `main`). cellp never watches git. CI is: **put that folder in object storage → POST version → poll → optional promote**.

If you have not written the app yet, start at [Write a Worker](/build/) and [Configure bindings](/build/wrangler).

## Artifact layout

Upload the wrangler bundle so cellpd can fetch it:

```text
s3://cellp-artifacts/{project}/{version}/
```

Use path-style S3 against **RustFS** (`AWS_ENDPOINT` / `--endpoint-url`). Not AWS S3, not Cloudflare R2.

## Create the version

`POST /v1/projects/{project}/versions` with `DEPLOY_TOKEN`.

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

Poll `GET` on the version (or the `poll_url` from the 202) until `status` is `ready` or `failed`.

## GitHub Actions (PR preview)

A full workflow lives in the repo: [`dev/examples/ci-pr-preview.example.yml`](https://github.com/KonghaYao/cellp/blob/main/dev/examples/ci-pr-preview.example.yml).

Sketch:

```yaml
# build job: npm ci && npm run build, aws s3 cp bundle to
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
    # poll until ready, print preview_url
```

Secrets you need:

| Secret / var | Purpose |
|--------------|---------|
| `CELLP_URL` | API origin, e.g. `https://cellp.internal/v1` |
| `CELLP_DEPLOY_TOKEN` | CI-only token |
| RustFS endpoint + keys | Artifact upload |

Commenting the preview URL on the PR is **your** GitHub token / bot. cellp will not.

## Production pipeline

Same upload + `POST /versions` **without** treating it as a throwaway PR id. After smoke tests:

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$VERSION/promote"
```

Keep `ADMIN_TOKEN` out of PR workflows from forks.

## Parent choice

| Pipeline | `parent_version_id` |
|----------|---------------------|
| First seed | omit (root) + D1 import / seed scripts |
| PR | a **pinned staging** version |
| Prod candidate | staging or last good prod — team policy |

Avoid parenting every PR at live production.

## Local stand-in

```bash
./dev/scripts/simulate-cd.sh my-shop v-local1
```

Same orchestrator path, no GitHub required.
