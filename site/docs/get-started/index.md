# Quick start

Install the CLI, start a local platform (**no Docker**), deploy a Worker.

## 1. Install

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp doctor
```

Details: [Install](/guides/install).

## 2. Run the platform

```bash
cellp dev --no-deploy
```

Leaves API on `:8790` and gateway on `:8787`. Stop with Ctrl-C. Full notes: [cellp dev](/guides/dev).

## 3. Write a Worker (or use the example)

Minimal app:

```text
my-shop/
  wrangler.jsonc
  index.js
```

See [Write a Worker](/build/). From that directory:

```bash
cellp dev
```

Opens `http://127.0.0.1:8787/<name>/dev/` after the version is ready.

## Commerce example (from this repo)

```bash
git clone https://github.com/KonghaYao/cellp.git && cd cellp
# after install.sh so celld is on PATH
cd dev/examples/commerce
cellp dev --project commerce-store
```

Or the contributor stack with Docker RustFS: [Local stack](/get-started/local).

## Docker (self-host)

```bash
docker compose up -d
curl -sf http://127.0.0.1:8790/v1/health
```

Image `ghcr.io/konghayo/cellp`. [Self-hosting](/guides/self-hosting).

## Next

- [cellp dev](/guides/dev)
- [Write a Worker](/build/)
- [Configure bindings](/build/wrangler)
- [Deploy from CI](/guides/ci)
