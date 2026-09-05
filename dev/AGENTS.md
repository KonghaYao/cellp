# Agent 指令 — cellp 本地 Dev

你在本仓库开发 cellp 后端 / celld 集成 / e2e 时，**必须**使用本地 dev 栈验证，再 commit。

**必读：** [DESIGN.md](../DESIGN.md) · [docs/decisions.md](../docs/decisions.md) · [docs/test-plan.md](../docs/test-plan.md)

## 启动

```bash
cp dev/.env.example dev/.env   # 首次

# 首次或依赖变更
./dev/scripts/up.sh
./dev/scripts/health.sh        # 完整运行时健康检查，必须 exit 0

# 日常 cellpd 编辑循环
./dev/scripts/up.sh --fast      # 不碰 RustFS/celld/offshoot
./dev/scripts/health.sh --quick # 仅 API + Gateway readiness
```

`--fast` 不会启动缺失的 RustFS/celld，也不会运行 `celld diagnose`；它只能复用存储探针已经成功的本地栈。存储、celld 或 offshoot 变更后必须重新跑完整路径，并执行 `RUN_GATES=1 ./e2e/scripts/run-all.sh`（或至少目标 `v0a-celld-diagnose`）。

## 改代码后验证

```bash
# 部署示例 app 的新版本
./dev/scripts/simulate-cd.sh demo-app v-$(date +%s | tail -c 6)

# 或指定版本 id
./dev/scripts/simulate-cd.sh demo-app v-test1

# 自检
./dev/scripts/health.sh
curl -sf http://127.0.0.1:8790/v1/projects/demo-app/versions/v-test1 | jq .status
# 期望: "ready"

curl -sf -H "Host: demo-app.ingress.local" http://127.0.0.1:8787/health
# Host / ingress: dev/INGRESS-HOST.md
```

### Go 控制面

```bash
cd cellp && go test ./...
make -C cellp openapi-check   # 改 API 后
```

### D1 数据面（改 celld / orchestrator / runtime 后）

```bash
export PATH="$HOME/.local/bin:$HOME/go/bin:$PATH"
bash e2e/scripts/v1-d1-seed.sh
bash e2e/scripts/v1-d1-branch.sh
D1_IMPORT_SIZE_MB=8 bash stress/phase6/d1-branch-scale.sh
```

改 `celld/` submodule 后：

```bash
cd celld && cargo build -p celld --profile lab
# 确保 ~/.local/bin/celld 指向 celld/target/lab/celld
```

### 目标 E2E（编辑循环）

```bash
# 名称可省略 .sh；多个名称用逗号或重复 --only
./e2e/scripts/run-all.sh --only v1-d1-seed,v1-d1-branch
./e2e/scripts/run-all.sh --only health-all --skip-cleanup
```

`--only` 是开发加速入口，**不代表 M1/M2 全量门禁**。`--skip-cleanup` 会让目标脚本内部的 `cleanup_e2e_versions` 也跳过，仅在确认旧 `v-e2e-*` 不会污染结果时使用。

### 全量验证

```bash
./e2e/scripts/run-all.sh  # 完整 MANIFEST（沿用既有默认，不含 Phase 0）
# 存储准入/发布验证（包含 celld diagnose）
RUN_GATES=1 ./e2e/scripts/run-all.sh
```

## Exit code 约定

| 脚本 | 0 | 非 0 |
|---|---|---|
| `health.sh` | 全组件健康 | 列出失败组件 |
| `simulate-cd.sh` | version ready | 打印 orchestrator 错误 |
| `up.sh` | 栈已启动 | 缺二进制或 docker 失败 |

## 禁止

- 不要引入 AWS S3 / Cloudflare R2 / Azurite 等外部云对象存储（统一 **RustFS**）
- 不要把 **Caddy / Forgejo / PostgreSQL** 当作 cellp 依赖
- 不要跳过 `celld diagnose` 存储探针（私有化准入门槛）
- 不要跳过 `health.sh` 直接 claim 完成
- 不要手改 `dev/data/` 里的 sqlite/状态（用 `reset.sh`）
- 不要用 `DATABASE_PATH` 模式集成 celld（用 D1 + offshoot export seed）
- 不要让 celld 直接读 offshoot store（D1 走 import/branch RPC，见 [decisions.md](../docs/decisions.md)）
- 不要在 JSON API 里传 SQLite 字节（冻结契约）

## 卡住时

```bash
./dev/scripts/logs.sh
./dev/scripts/down.sh
./dev/scripts/reset.sh
./dev/scripts/up.sh
```

## 相关文档

- **[DESIGN.md §11](../DESIGN.md#11-本地单机-devagent-闭环)** — 本地 Dev 设计
- **[docs/decisions.md](../docs/decisions.md)** — AD-1..5 · D1 · 存储 tier
- **[e2e/README.md](../e2e/README.md)** — 端口级验收脚本
- **[VALIDATION.md](../VALIDATION.md)** — V* 历史编号索引
