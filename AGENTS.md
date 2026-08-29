# Agent 指令 — cellp

你在本仓库开发 **cellp**（版本化 Serverless 运行时）。

## 必读（按顺序）

1. **[DESIGN.md](./DESIGN.md)** — 唯一顶层设计
2. **[docs/decisions.md](./docs/decisions.md)** — 当前有效架构决策（AD-1..5 · D1 · 存储 tier）
3. **[docs/test-plan.md](./docs/test-plan.md)** — 功能验收门禁
4. 任务相关子目录 AGENTS：
   - 后端 / 本地栈 → **[dev/AGENTS.md](./dev/AGENTS.md)**
   - Dashboard → **[web/AGENTS.md](./web/AGENTS.md)**

完整文档索引：**[docs/README.md](./docs/README.md)**

## 仓库地图

| 路径 | 是什么 | 验证命令 |
|------|--------|----------|
| `cellp/` | Go 控制面（API · Orchestrator · Gateway · Registry） | `cd cellp && go test ./...` |
| `celld/` | Rust Workers 运行时（**git submodule**） | `cargo build -p celld --profile lab`（在 `celld/`） |
| `web/` | Dashboard（Vite + React SPA） | `cd web && npm run test:e2e` |
| `dev/` | 本地 dev 栈（RustFS · cellpd · celld · offshoot） | `./dev/scripts/health.sh` |
| `e2e/` | 端口级集成测试（M1/M2 门禁） | `./e2e/scripts/run-all.sh` |
| `stress/` | 压测（phase5 生产 · phase6 扩展/D1 scale） | 见 `stress/README.md` |
| `docs/` | 计划 · 契约 · 证据 | [docs/README.md](./docs/README.md) |

## 核心决策（摘要）

- **AD-1：** 每个 ready version = 独立 celld 进程 + 独立 bucket；**本地 `CELPD_WATCH` 为临时页缓存，Stop 后删除；S3 为唯一持久层**
- **AD-4：** Dev 可用 local offshoot；prod offshoot RustFS 需 V0b（当前 **deferred**）
- **D1 import：** 根 version；`celld d1 import --file`；契约 [D1-IMPORT-RPC.md](./docs/plans/D1-IMPORT-RPC.md)
- **D1 branch：** 子 version（`parent_version_id`）；`celld d1 branch --parent-bucket`；契约 [D1-BRANCH-RPC.md](./docs/plans/D1-BRANCH-RPC.md)
- **冻结：** SQLite 字节 **不得** 放进 JSON RPC；celld **不** 读 offshoot store

## 改代码后的验证顺序

```bash
# 1. 本地栈健康
./dev/scripts/up.sh && ./dev/scripts/health.sh

# 2. Go 单元测试
cd cellp && go test ./...

# 3. 集成门禁
./e2e/scripts/run-all.sh

# 4. D1 专项（改 celld D1/LTX 或 orchestrator 后）
bash e2e/scripts/v1-d1-seed.sh
bash e2e/scripts/v1-d1-branch.sh
D1_IMPORT_SIZE_MB=8 bash stress/phase6/d1-branch-scale.sh
```

100 MB 多分支手动测试（**不在** `run-all.sh`）：

```bash
D1_BRANCH_MULTI_SIZE_MB=100 D1_BRANCH_MULTI_COUNT=3 \
  bash e2e/scripts/v1-d1-branch-multi-100mb.sh
```

改 `celld/` submodule 后：`cargo build -p celld --profile lab` 并确保 `~/.local/bin/celld` 指向新构建。

## 禁止

- 引入 **PostgreSQL**、**Caddy**、**Forgejo** 作为 cellp 依赖
- 使用 AWS S3 / Cloudflare R2 等外部云对象存储（统一 **RustFS**）
- 跳过 `celld diagnose` 存储探针
- 手改 `dev/data/` 里的 sqlite / 状态（用 `./dev/scripts/reset.sh`）
- 未经审查修改 **冻结契约**（`docs/plans/D1-*-RPC.md`）
- 改 `cellp/go.mod`（除非 track 明确为 deps owner）
- Dashboard（`web/`）直连 `:8792` celld 或 offshoot CLI
- 在 `web/` 引入 Next.js / SSR / Server Actions（Dashboard 是 **Vite SPA**）

## Subagent 派发

见 [docs/README.md § Subagent](./docs/README.md#subagent-派发约定)。

## 证据

运行测试前：`mkdir -p docs/evidence`  
产物写入 `docs/evidence/`；临时 `.log` / `.out` 已 gitignore，勿提交。
