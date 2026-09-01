# cellp dev

`cellp dev` is the wrangler-style local loop: **one command, no Docker**. It starts an in-process S3, then cellpd (API + gateway). If the current directory has `wrangler.jsonc`, it deploys that Worker as version `dev`.

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"

cd my-shop          # folder with wrangler.jsonc + index.js
cellp dev
```

Then:

- API `http://127.0.0.1:8790`
- Gateway `http://127.0.0.1:8787` with **Host** from `preview_url` / `prod_url` ([Preview](/concepts/preview))
- Data `~/.cellp/data` (override with `--home` or `CELLP_HOME`)

Each `cellp dev` that finds `wrangler.jsonc` deploys a version. The first id is `dev`. If `dev` already exists in `~/.cellp`, a new id (`dev-<unix>`) is created and **forked from production** so D1/KV/R2 stay. There is no file watcher — edit, Ctrl-C, run `cellp dev` again.

`--no-deploy` only starts the platform.

## Flags

| Flag | Meaning |
|------|---------|
| `--home DIR` | Data directory (default `~/.cellp`) |
| `--project ID` | Project id (default: `name` in wrangler.jsonc) |
| `--no-deploy` | Only start the platform; do not upload cwd |

## Compared with `wrangler dev`

| wrangler | cellp |
|----------|--------|
| Talks to Cloudflare or a local isolate | Talks to **your** cellpd on localhost |
| One Worker process | Full platform: versions, bindings, gateway |
| No Docker | No Docker (`cellp dev` uses embedded S3) |
| Account login | Two tokens (defaults `dev-local-token`) |

You still write the same Worker + `wrangler.jsonc`. [Write a Worker](/build/).

## What is not Docker

`cellp dev` does **not** start RustFS. Object storage is a local Bolt-backed S3 on `:19000`. Offshoot uses a **directory** under `~/.cellp/data`. That is the laptop path.

For a production-like disk (RustFS), use [Docker Compose](/guides/self-hosting).

## Doctor

```bash
cellp doctor
```

Checks `celld`, `offshoot`, `esbuild`, and ports `8787` / `8790` / `19000`. Release tarballs put the binaries in the same folder so `cellp` finds siblings on `PATH`.

## Tokens

Local defaults match the rest of the docs: deploy and admin are `dev-local-token`. [Auth](/reference/auth).
