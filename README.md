# cellp

**Private, versioned Workers — on your hardware.**

Every deploy versions **the app and its data**. Preview is a real fork of D1, KV, R2, and Queues. Production is an explicit promote. You keep Git, CI, and TLS; cellp is the control plane in the middle.

**Docs:** **[https://konghayao.github.io/cellp/](https://konghayao.github.io/cellp/)**

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp doctor
cellp dev
```

[Install](https://konghayao.github.io/cellp/guides/install) · [cellp dev](https://konghayao.github.io/cellp/guides/dev) · [Write a Worker](https://konghayao.github.io/cellp/build/)

---

cellp 是**私有化 Workers 平台控制面**：外部 CI 每次投递时同时 version 化 App + Data，经 Gateway 提供 preview / prod。100% 自建，不依赖 AWS / Cloudflare 账号。

面向使用者的文档以 GitHub Pages 为准；本 README 下面是仓库入口。贡献者 / Agent 仍读 [DESIGN.md](./DESIGN.md) 与 [docs/](./docs/README.md)。

## What it is / is not

| cellp does | cellp does not |
|------------|----------------|
| Version lifecycle + preview / prod URLs | User accounts, orgs, SSO |
| Branch D1 · KV · R2 · Queue on child versions | Git hosting, webhooks, PR bots |
| Promote saga + rollback-by-re-promote | DNS, CDN, TLS, WAF, global PoPs |
| Dashboard + REST on the same API | Next.js / Node serverless hosting |
| Docker / laptop self-host (RustFS + SQLite) | A hosted cellp cloud |

## Stack

| Component | Role |
|-----------|------|
| **cellpd** | Go — API, orchestrator, gateway, SQLite registry |
| **celld** | Rust ([submodule](./celld/)) — Workers + bindings runtime |
| **offshoot** | SQLite copy-on-write (App + Data) |
| **RustFS** | Private S3 — artifacts, offshoot, celld blobs |
| **web/** | Vite SPA Dashboard (REST only) |

## Quick start

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp dev
```

Binaries (macOS / Linux / Windows) ship on [GitHub Releases](https://github.com/KonghaYao/cellp/releases). Docker: `ghcr.io/konghayo/cellp`.

### From source (contributors)

```bash
git clone https://github.com/KonghaYao/cellp.git && cd cellp
git submodule update --init celld
# build celld: cd celld && cargo build -p celld --profile lab
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/seed-commerce-store.sh
curl -sf http://127.0.0.1:8787/commerce-store/v1/stats
```

Prereqs: Docker · Node 20+ · Go · `celld` · `offshoot` · `jq` · `esbuild`.  
Walkthrough: **[docs site → Get started](https://konghayao.github.io/cellp/get-started/)** · local scripts: [dev/README.md](./dev/README.md).

### Docker

```bash
git submodule update --init celld
docker compose up -d --build
curl -sf http://127.0.0.1:8790/v1/health
```

Image: `ghcr.io/konghayo/cellp:latest` · [Self-hosting](https://konghayao.github.io/cellp/guides/self-hosting) · [docker/README.md](./docker/README.md).

### Dashboard

```bash
./dev/scripts/up.sh
cd web && npm install && npm run dev
# http://127.0.0.1:5190
```

## Repository

```
cellp/     Go control plane
celld/     Rust runtime (submodule)
web/       Dashboard
dev/       Local stack + examples
e2e/       Integration gates
stress/    Load tests
docs/      Internal design · ADRs · test plans · evidence
site/      Public documentation (this GitHub Pages site)
```

Local ports: Gateway **8787** · API **8790** · celld **8792+** · Dashboard **5190** · RustFS **9000**.

## Contributor docs

| Doc | For |
|-----|-----|
| [DESIGN.md](./DESIGN.md) | Architecture (source of truth for implementers) |
| [docs/decisions.md](./docs/decisions.md) | AD-1…10 product boundary |
| [docs/test-plan.md](./docs/test-plan.md) | Acceptance gates |
| [AGENTS.md](./AGENTS.md) | How agents/contributors change the tree |
| [docs/README.md](./docs/README.md) | Internal doc index |

```bash
cd cellp && go test ./...
./e2e/scripts/run-all.sh
cd web && npm run test:e2e
```

## License

See each subtree. `celld/` is based on [KonghaYao/celld](https://github.com/KonghaYao/celld).
