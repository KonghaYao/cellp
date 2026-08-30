# Quick start

Get a live Worker with real bindings on your laptop. No Cloudflare account.

## What you will have

- Gateway at `http://127.0.0.1:8787`
- API at `http://127.0.0.1:8790`
- An example store at `/commerce-store/v1/`
- Dashboard at `http://127.0.0.1:5190` (after `npm run dev` in `web/`)

## Prerequisites

- Docker (for RustFS)
- Node 20+
- Go
- Rust/cargo (to build **celld**)
- `jq`, `esbuild` (`npm i -g esbuild`)
- [offshoot](https://github.com/sricola/offshoot): `go install github.com/sricola/offshoot/cmd/offshoot@latest`

## 1. Clone

```bash
git clone https://github.com/KonghaYao/cellp.git
cd cellp
git submodule update --init celld
```

## 2. Build the runtime

```bash
cd celld && cargo build -p celld --profile lab
# put celld/target/lab/celld on your PATH, e.g. ~/.local/bin/celld
```

## 3. Start the stack

```bash
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/health.sh
```

`health.sh` must exit 0. If it does not, run `./dev/scripts/logs.sh`.

## 4. Seed the example

```bash
./dev/scripts/seed-commerce-store.sh
curl -sf http://127.0.0.1:8787/commerce-store/v1/stats
```

Open the storefront: [http://127.0.0.1:8787/commerce-store/v1/](http://127.0.0.1:8787/commerce-store/v1/)

## 5. Deploy another version (optional)

```bash
./dev/scripts/simulate-cd.sh commerce-store v-dev2
curl -sf http://127.0.0.1:8787/commerce-store/v-dev2/
```

You now have two isolated versions of the same project. Production is whichever version you [promote](/concepts/promote).

## Docker (single machine)

If you prefer Compose instead of the laptop toolchain:

```bash
git submodule update --init celld
docker compose up -d --build
curl -sf http://127.0.0.1:8790/v1/health
```

Image: `ghcr.io/konghayo/cellp:latest`. Full notes: [Self-hosting](/guides/self-hosting).

## Next

- **[Write a Worker](/build/)** — `index.js` + `wrangler.jsonc` + first version URL
- **[Configure bindings](/build/wrangler)** — how D1 / KV / R2 / queues get onto `env`
- **[Platform data](/build/data)** — seed D1, edit KV, fork preview data
- [Local stack in more detail](/get-started/local)
- [Example app (commerce)](/get-started/example)
- [Dashboard](/get-started/dashboard)
- [Deploy from CI](/guides/ci)
