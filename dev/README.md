# cellp — 本地 Dev 快速上手

```bash
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/seed-commerce-store.sh   # commerce-store v1 + D1 seed
./dev/scripts/health.sh
curl http://127.0.0.1:8787/commerce-store/v1/stats
```

完整设计见 **[DESIGN.md §11](../DESIGN.md#11-本地单机-devagent-闭环)** · 决策见 **[docs/decisions.md](../docs/decisions.md)** · 验证项见 **[docs/test-plan.md](../docs/test-plan.md)**。

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

队列深度探针：`GET /v1/health/deep`（返回 `pending_jobs` · `queue_max`）。
