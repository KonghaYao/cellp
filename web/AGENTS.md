# Agent 指令 — cellp Dashboard (`web/`)

你在 `web/` 开发 Dashboard 时，**只通过 cellpd REST API** 与平台交互。

**必读：** [DESIGN.md](../DESIGN.md) · [docs/decisions.md](../docs/decisions.md) · [../cellp/api/openapi.yaml](../cellp/api/openapi.yaml) · [docs/plans/phase-4-dashboard.md](../docs/plans/phase-4-dashboard.md)

## 技术栈

**Vite 6 + React 19 + React Router 7** — 纯静态 SPA。禁止引入 Next.js / SSR / Server Actions。

## 启动

```bash
# 终端 1：后端栈
./dev/scripts/up.sh && ./dev/scripts/health.sh

# 终端 2：Dashboard
cd web && npm install && npm run dev
# http://127.0.0.1:5173
```

API 默认 `http://127.0.0.1:8790`（见 `web/src/lib/cellp-api.ts`）。  
环境变量见 `web/.env.example`（`VITE_CELLP_*`）。

## 改代码后验证

```bash
cd web && npm run lint
cd web && npm run test:e2e    # Playwright smoke（TP-UI-5）
cd web && npm run build       # 产物 web/dist/
```

## 架构约束

| 规则 | 原因 |
|------|------|
| **禁止**直连 `:8792` celld | AD-1：运行时不对 Dashboard 暴露 |
| **禁止**调用 offshoot CLI / 读 offshoot store | 数据面经 orchestrator + API |
| **禁止**在 bundle 里嵌 SQLite 字节 | D1 契约冻结 |
| 所有数据经 `cellp-api.ts` | 单一 API 边界，便于 mock |

验收：`rg ':8792|offshoot' web/src/` 应无匹配（TP-UI-6）。

## 页面与路由

- `web/src/lib/routes.ts` — 路由常量
- `web/src/pages/` — 页面组件
- `web/src/components/layout/` — App shell · sidebar

完整路由表见 [phase-4-dashboard.md](../docs/plans/phase-4-dashboard.md)。

## 禁止

- 改 `cellp/` Go 代码（除非任务明确要求全栈）
- 引入 Next.js 或 `web/app/` App Router
- 引入直连 S3 / celld admin API

## 相关文档

- **[docs/test-plan.md §F](../docs/test-plan.md)** — TP-UI-* 验收项
- **[../dev/AGENTS.md](../dev/AGENTS.md)** — 本地栈
