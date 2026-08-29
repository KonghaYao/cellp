# cellp

**cellp** — 版本化 Serverless 应用运行时。在每次 CD 时同时 version 化 **App + Data**，通过稳定 Gateway URL 提供完整可访问环境。100% 私有化部署。

| 文档 | 说明 |
|------|------|
| **[DESIGN.md](./DESIGN.md)** | 顶层设计（唯一设计入口） |
| **[docs/decisions.md](./docs/decisions.md)** | 架构决策与冻结约束 |
| **[docs/README.md](./docs/README.md)** | 文档库索引 |
| **[docs/test-plan.md](./docs/test-plan.md)** | 功能验收门禁 |
| **[AGENTS.md](./AGENTS.md)** | Agent / 贡献者总则 |
| **[dev/README.md](./dev/README.md)** | 本地开发快速上手 |

## 技术栈

| 组件 | 选型 | 职责 |
|------|------|------|
| **cellpd** | Go | API · Orchestrator · Gateway · SQLite Registry |
| **celld** | Rust（[submodule](./celld/)） | Workers + D1 + LTX 运行时 |
| **offshoot** | Go CLI | SQLite CoW 分支（App+Data versioning） |
| **RustFS** | 自建 S3 | artifact · offshoot · celld blob |
| **web/** | Vite + React SPA | Dashboard（仅消费 REST API） |

**外部边界（非 cellp 组件）：** 任意 Git 托管 · 任意 CI 引擎。

## v1 交付范围（2026-08-29）

| 能力 | 状态 | 验收 |
|------|------|------|
| CD + Version 生命周期 | ✅ | `e2e/scripts/ve-cd-loop.sh` |
| Gateway 多 version 路由（AD-1） | ✅ | `v3-dual-route.sh` |
| Promote saga（AD-5） | ✅ | `v4-promote-cutover.sh` |
| offshoot → D1 import | ✅ | `v1-d1-seed.sh` |
| D1 branch（子 version 共享父 LTX） | ✅ | `v1-d1-branch.sh` |
| Dashboard（项目 · 部署 · 存储 · D1 管理） | ✅ | `cd web && npm run test:e2e` |
| offshoot prod × RustFS（V0b） | ✅ | `e2e/scripts/v0b-offshoot-rustfs.sh` · [v0b-pass-report.md](./docs/evidence/v0b-pass-report.md) |
| Phase 6 扩展（6A · SQLite scope） | ✅ 6A 实现完成 | [test-plan-phase6.md](./docs/test-plan-phase6.md) · [v1-v0b-phase6-plan.md](./docs/plans/v1-v0b-phase6-plan.md)（6B–6F OUT OF SCOPE） |

## 快速开始

### 克隆与子模块

```bash
git clone <repo> cellp && cd cellp
git submodule update --init celld
```

### 本地 dev 栈

```bash
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/simulate-cd.sh demo-app v-dev1
./dev/scripts/health.sh
curl -sf http://127.0.0.1:8787/demo-app/v-dev1/
```

前置：Docker（RustFS）· Node 20+ · Go · `celld` · `offshoot` · `jq` · `esbuild`  
详见 **[dev/README.md](./dev/README.md)**。

### 构建 celld

```bash
cd celld && cargo build -p celld --profile lab
# 产物: celld/target/lab/celld
```

### 集成验收

```bash
./e2e/scripts/run-all.sh
cd cellp && go test ./...
cd web && npm run test:e2e
```

### Dashboard 开发

```bash
./dev/scripts/up.sh
cd web && npm install && npm run dev
# http://127.0.0.1:5173
```

## 仓库结构

```
cellp/          # Go 控制面（module root）
celld/          # Rust 运行时（git submodule）
web/            # Dashboard（Vite SPA）
dev/            # 本地栈脚本与示例
e2e/            # 端口级集成测试
stress/         # 压测 harness
docs/           # 计划 · 契约 · 证据
```

## 端口（本地 dev）

| 端口 | 服务 |
|------|------|
| 8787 | cellpd Gateway |
| 8790 | cellpd API |
| 8792+ | celld（每 version 递增） |
| 5173 | Dashboard dev server |
| 9000 | RustFS S3 |

## 许可

cellp 组件许可见各子目录。`celld/` submodule 基于 [KonghaYao/celld](https://github.com/KonghaYao/celld)（含 D1 import/branch 扩展）。
