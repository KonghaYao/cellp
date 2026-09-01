# cellp — 本地 Dev 快速上手

产品文档（如何用、CI、Dashboard）：**[https://konghayao.github.io/cellp/](https://konghayao.github.io/cellp/)**

使用者本机无 Docker：`curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh` 然后 `cellp dev`。下面是**贡献者**本地栈（Docker RustFS）。

**Preview/Prod Host 统一配置：** [INGRESS-HOST.md](./INGRESS-HOST.md) · `./dev/scripts/ingress-host-init.sh` · Clash [clash/README.md](./clash/README.md)

```bash
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/seed-commerce-store.sh   # commerce-store v1 + D1 seed
./dev/scripts/health.sh
# AD-12 Host — 见 INGRESS-HOST.md（lvh.me）
curl -H "Host: commerce-store.lvh.me" http://127.0.0.1:8787/stats
```

完整设计见 **[DESIGN.md §11](../DESIGN.md#11-本地单机-devagent-闭环)** · 决策 **[AD-12](../docs/decisions.md#17-ad-12--hostname--port-ingress废弃-path-选-version)** · Ingress **[INGRESS-ROUTING.md](../docs/plans/INGRESS-ROUTING.md)** · 验证 **[docs/test-plan.md](../docs/test-plan.md)**。

## Ingress（AD-12，摘要）

| 角色 | Host 形态 |
|------|-----------|
| Preview | `{version}.{project}.lvh.me` |
| Prod | `{project}.lvh.me` |

Gateway **:8787** 按 **Host** 选 version。`*.lvh.me` → `127.0.0.1`。`./dev/scripts/ingress-host-init.sh`。

## 端口

| 服务 | 端口 | 说明 |
|---|---|---|
| **cellpd Gateway** | 8787 | 内置 reverse proxy（dev 由 mock 模拟） |
| **cellpd API** | 8790 | REST · Registry |
| celld | 8792 | Workers 运行时（含 Workers KV） |
| RustFS S3 | 9000 | 对象存储 |
| RustFS Console | 9001 | 管理 UI |

## cellp 实际依赖 vs 外部边界

| **cellp 组件** | **不是 cellp 依赖** |
|---|---|
| RustFS · celld · offshoot | 任意 Git 托管 |
| **SQLite** Registry（`cellp-registry.sqlite`） | 任意 CI 引擎 |
| cellpd（API + Orchestrator + **内置 Gateway**） | Caddy / Nginx（用户自有 TLS/LB 反代即可） |

## 前置

- Docker（仅 RustFS compose；Gateway/API 跑在宿主机）
- Node 20+
- `celld` — 本仓库 submodule [`celld/`](https://github.com/KonghaYao/celld)（`git submodule update --init` 后 `cargo build -p celld --profile lab`）
- `esbuild` — `npm i -g esbuild`
- `offshoot` — `go install github.com/sricola/offshoot/cmd/offshoot@latest`
- `jq`

## 脚本

| 脚本 | 用途 |
|---|---|
| `up.sh` | 起 RustFS compose + offshoot + mock（API+Gateway）+ celld |
| `down.sh` | 停栈 |
| `health.sh` | 探活（agent CI 用） |
| `reset.sh` | 清空 `dev/data/` |
| `simulate-cd.sh` | 本地 CD：`simulate-cd.sh <project> <version>` |
| `seed-commerce-store.sh` | **默认验收**：`commerce-store` v1，D1 电商假数据 + Dashboard 链接 |
| `seed-demo.sh` | Bindings 演示：`demo-app` v1/v2，D1/KV/Queue/Workflow 假数据 |
| `ingress-host-init.sh` | 设置 **lvh.me** ingress |
| `ingress-repromote-support.sh` | 换 base 后重绑 support 项目 prod Host |
| `logs.sh` | 看日志 |
| `gc.sh` | 一次性 Registry GC（jobs + destroyed versions） |

## Registry GC（Phase 6A-T3）

cellpd 启动后按 `CELLP_GC_INTERVAL` 后台清理：

| 变量 | 默认 | 说明 |
|---|---|---|
| `CELLP_GC_INTERVAL` | `1h` | 后台 tick；`0` 关闭 |
| `CELLP_GC_RETENTION_DAYS` | `7` | completed/failed jobs 与 destroyed versions 保留天数 |

手动执行：`./dev/scripts/gc.sh`

## Orchestrator worker pool（Phase 6C-T1）

| 变量 | 默认 | 说明 |
|---|---|---|
| `CELLP_ORCH_WORKERS` | `1` | Orchestrator 并行 worker goroutine 数（SQLite dev 仍为单写；PostgreSQL prod 配合 `SKIP LOCKED`） |
| `CELLP_QUEUE_MAX` | `10000` | 部署队列 pending jobs 上限；超过后 `POST /versions` 返回 **503** |
| `CELLP_LENIENT_DEPLOY` | （未设） | 设为 `1` 时 offshoot / D1 seed&branch 失败仅 warn 仍尝试 ready；**默认关闭（fail-closed）**。`CELLP_STRICT_OFFSHOOT_FORK=1` 已废弃，与默认等价 |

队列深度探针：`GET /v1/health/deep`（返回 `pending_jobs` · `queue_max`）。
