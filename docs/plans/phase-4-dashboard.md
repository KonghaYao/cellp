# Phase 4 — Dashboard（v1）

> **TP：** TP-UI-*  
> **Gate：** **M1（TP-VE-ALL）** — 未过不得开工  
> **栈：** **Vite 6 + React 19 + React Router 7**（纯静态 SPA，非 Next.js）

## Exit Criteria

- [x] `cd web && npm run dev` 可访问（`:5173`）
- [x] `cd web && npm run build` → `web/dist/` 纯静态
- [x] TP-UI-1..6 全绿
- [x] Playwright e2e 全绿（`cd web && npm run test:e2e`）
- [x] **M2 达成**（TP-V1–V7 + TP-UI 全绿）

## 技术选型

| 项 | 选型 |
|----|------|
| 构建 | Vite 6 |
| UI | React 19 · Tailwind CSS 4 · lucide-react |
| 路由 | React Router 7（Browser history） |
| API | `web/src/lib/cellp-api.ts` → cellpd `:8790/v1` |
| 环境变量 | `VITE_CELLP_API_URL` · `VITE_CELLP_ADMIN_TOKEN` · `VITE_CELLP_GATEWAY_URL` |

**不做：** Next.js · SSR · Server Actions · 直连 celld `:8792` · offshoot CLI

## 路由（v1）

| 路径 | 页面 |
|------|------|
| `/` | Projects 列表 |
| `/projects/:id` | Project 概览 |
| `/projects/:id/deployments` | Deployments 表格 |
| `/projects/:id/storage` | Storage 入口 |
| `/projects/:id/storage/:vid/browser` | D1 管理（Schema · Data · Query · Branches） |
| `/projects/:id/settings` | 项目设置 · **Worker env** |
| `/projects/:id/versions/:vid` | 版本详情 · Promote · Destroy · **Worker env** |

旧路径 `/projects/:id/versions/:vid/database` → 重定向到 storage browser。

## 目录结构

```
web/
├── package.json
├── vite.config.ts
├── playwright.config.ts
├── index.html
├── src/
│   ├── App.tsx
│   ├── pages/
│   ├── components/       # layout/ · database/ · ui/
│   └── lib/              # cellp-api.ts · routes.ts · format.ts
└── e2e/                  # Playwright + mock-api-server.mjs
```

## 数据流

```
Browser SPA (:5173)  ──fetch──▶  cellpd API (:8790/v1)
                         Bearer VITE_CELLP_ADMIN_TOKEN
```

本地 dev 跨域：cellpd API 已加 CORS middleware（允许 `Authorization` header）。

## 静态部署

```bash
cd web && npm run build
./dev/scripts/deploy-dashboard.sh v-ui1
```

产物 `web/dist/` 经 `simulate-cd.sh` 上传为静态 artifact。

## TP-UI 验证

| ID | 检查 | 命令 |
|----|------|------|
| TP-UI-1 | Project 列表 | Playwright |
| TP-UI-2 | Version / Deployments 列表 | Playwright |
| TP-UI-3 | Promote + Destroy | Playwright |
| TP-UI-4 | 仅 API 消费 | `web/src/lib/cellp-api.ts` |
| TP-UI-5 | Playwright smoke | `npm run test:e2e` |
| TP-UI-6 | 无直连运行时 | `rg ':8792\|offshoot' web/src/` 无匹配 |
| TP-UI-13 | Settings Worker env | Playwright `settings edits worker env` |

## Subagent prompt

```
Phase 4 Dashboard. Work only under web/. Vite SPA only — no Next.js.
Gate: TP-VE-ALL passed. Verify: cd web && npm run test:e2e
```
