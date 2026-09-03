# 前端框架：Cloudflare vs cellp

> **日期：** 2026-09-02  
> **架构决策：** [decisions.md §18 AD-13](./decisions.md#18-ad-13--前端框架一等公民与-nextjs-边界)  
> **CF 官方索引：** [Framework guides](https://developers.cloudflare.com/workers/framework-guides/) · [Static assets](https://developers.cloudflare.com/workers/static-assets/)  
> **cellp 口径：** 仅 **支持 / 不支持**（见 [support-matrix.md](./support-matrix.md)）

---

## 0. 一等公民（cellp AD-13）

与 Cloudflare Workers 文档对齐、cellp **产品承诺 + S22–S25 验证轨** 的框架：

| 等级 | 框架 | 说明 |
|:----:|------|------|
| **一等** | **React + Vite SPA**、**Vue + Vite SPA** | 默认推荐；S15/S17/S20 等已验 |
| **一等** | **Astro** | `@astrojs/cloudflare`；**S22** |
| **一等** | **SvelteKit** | `adapter-cloudflare` **单 Worker**；**S23**（≠ cloudflarebase 多 service） |
| **一等** | **Remix** | `@remix-run/cloudflare`；**S24** |
| **一等** | **Nuxt** | Nitro `cloudflare` preset；**S25** |
| **非一等** | **Next.js** | CF 上靠 OpenNext/vinext；cellp **不追一等** → [plans/NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) |
| **非一等** | Solid / Waku / 等 | 有 CF guide，无 S 槽位；按需社区验证 |

**公开推荐（新应用）：** **Vite SPA + 单 Worker API** + wrangler `assets`（与 CF Static assets 模型一致）。

---

## 1. Cloudflare 在推什么（2025–2026）

| 方向 | 说明 |
|------|------|
| **Workers + Static Assets** | 一个部署单元：`main` Worker + `[assets]` 目录；SPA 用 `not_found_handling = "single-page-application"` |
| **Pages → Workers** | 新功能投在 **Workers**；Pages 仍维护，等价能力迁移到 **assets + Worker** |
| **框架** | 官方 guide 覆盖（llms.txt 摘要）：**React+Vite、Vue、SvelteKit、Astro、Nuxt、Solid、Waku、Next（vinext / OpenNext）、Remix** 等 |
| **路由** | 默认静态优先；API 用 `run_worker_first` 或 Worker 内 `env.ASSETS.fetch()` |

**结论：** CF 侧「热门」= **Vite SPA + 薄 Worker**、**SvelteKit/Astro adapter**、**Next 经 OpenNext/vinext**；不是「任意 wrangler 二次 esbuild」。

---

## 2. cellp 实际能托住哪类（从 S01–S21 反推）

| 框架 / 形态 | CF 典型部署 | cellp 证据 | **支持？** |
|-------------|-------------|------------|:----------:|
| **纯静态 + 薄 Worker**（`index.html` + `/api/*`） | assets + `run_worker_first` | S01 Relay、S06 Memos | **支持** |
| **Vite SPA + Worker**（build 出 `dist`，Worker 单入口） | assets + `no_bundle` 或 celld bundle | S15、S17 Vue、S20 R2-Explorer | **支持** |
| **Vite SPA + Pages Functions 形态** | `functions/` + `dist` | S21 FileWorker（pages build + bundle） | **支持** |
| **React 全栈 monorepo（pnpm）** | wrangler dry-run bundle | S05 FlareMo | **支持** |
| **Bun 自建 worker bundle** | 单文件 `.cellp-bundle` | S08 EdgeEver | **支持** |
| **Headless CMS Admin（Vite）** | assets + API Worker | S09 SonicJS | **支持** |
| **博客前后端分离**（`server/` wrangler） | 多目录 + assets | S07 Monolith | **支持** |
| **Alpine/HTML 单页工具** | 静态为主 | S18 webhookflare | **支持** |
| **Hono + 预 bundle**（无 .md/Tailwind 在 Worker 图里） | `no_bundle` | S19（根路径无路由 → 矩阵 **不支持**） | **不支持**（产品入口） |
| **Worker 内 React SSR + Tailwind + import `.md`** | wrangler 全规则 | S16 pastebin | **不支持** |
| **SvelteKit adapter（单 Worker 静态+_worker.js）** | adapter-cloudflare | S14 **能 slim 部署**，但 **多 service** → 矩阵 **不支持** |
| **多 Worker `[[services]]` 控制台** | 主站 + auth/db agents | S14 cloudflarebase | **不支持**（平台） |
| **Next.js / OpenNext** | OpenNext / vinext | **实验**；不进 support 矩阵 | **非一等**（见 OpenNext 计划） |

---

## 3. 与 CF 官方框架列表的对照（cellp 预测）

| 框架 | CF 支持 | cellp | 验证 |
|------|---------|-------|------|
| **React + Vite SPA** | ✅ 一等公民 | **一等 · 应支持** | S15/S17/S20 |
| **Vue + Vite** | ✅ | **一等 · 应支持** | S17 |
| **Astro** | ✅ static / server | **一等 · 待 S22** | `cloudflare/templates` astro-blog-starter |
| **SvelteKit** | ✅ adapter-cloudflare | **一等 · S23 支持** | C3 `templates/svelte` + `dev/examples/support-sveltekit/` |
| **Nuxt** | ✅ Nitro cloudflare | **一等 · S25 支持** | C3 `templates/nuxt` + `dev/examples/support-nuxt/`（PD-20260902-06 已修：`node:timers` `setImmediate`） |
| **Remix** | ✅ | **一等 · 待 S24** | `remix-starter-template` |
| **SolidStart** | ✅ C3 | **不支持（S27）** | `create-solid` 非交互 scaffold 失败 |
| **Qwik City** | ✅ C3 workers | **不支持（S28）** | `prepare-artifact` OK · prod celld health timeout |
| **Waku** | ✅ C3（`create-waku`） | **支持（S29 v9）** | 同上 · `nodejs_als` + slim wrangler · celld sibling ESM load |
| **Next.js** | ✅ OpenNext / vinext | **非一等 · 实验** | [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) |
| **Worker 内 SSR + CSS 管线** | ✅（wrangler 构建） | **易挂**（S16） | 预构建 + `no_bundle` |

---

## 4. 框架验证队列 S22–S25（AD-13）

| ID | 框架 | 仓库 / 路径 | 部署 |
|----|------|-------------|------|
| **S22** | Astro | `cloudflare/templates` → `astro-blog-starter-template` | `./dev/scripts/deploy-support-app.sh S22` |
| **S23** | SvelteKit | `cloudflare/workers-sdk` → `packages/create-cloudflare/templates/svelte` | `./dev/scripts/deploy-support-app.sh S23` |
| **S24** | Remix | `cloudflare/templates` → `remix-starter-template` | S24 |
| **S25** | Nuxt | `cloudflare/workers-sdk` → `packages/create-cloudflare/templates/nuxt` | S25 |

每个项目验证后 **仅**写入 [support-matrix.md](./support-matrix.md)：**支持** 或 **不支持**。套路优先：**`prepare-artifact.sh` + `no_bundle` + assets**。

---

## 5. cellp 与 CF 的差距（框架维度）

| 能力 | CF（wrangler） | cellp（today） |
|------|----------------|----------------|
| 完整 wrangler 构建（rules、loader、CSS） | ✅ | ❌ 常需 **预 bundle** |
| Static assets + SPA fallback | ✅ | ✅（overlay `assets`） |
| `run_worker_first` | ✅ | ⚠️ 注意 JSONC strip（S05 `/*` 坑） |
| 多 Worker services | ✅ | **不支持** |
| Framework CI 模板 | Pages/Workers Builds | **`deploy-support-app.sh` + overlay** |

---

## 6. 三项决策（已写入 AD-13）

1. **S22–S25** — Astro / SvelteKit / Remix / Nuxt 一等公民验证轨（见 §4）。  
2. **公开推荐** — 默认 **Vite SPA + Worker**；站点 [migrate/stacks.md](../site/docs/migrate/stacks.md)、[migrate/frameworks.md](../site/docs/migrate/frameworks.md)。  
3. **Next.js** — **非一等**；OpenNext 仅实验路径，优化见 [plans/NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md)。

**参考：** [CF framework guides](https://developers.cloudflare.com/workers/framework-guides/) · [migrate Pages → Workers](https://developers.cloudflare.com/workers/static-assets/migration-guides/migrate-from-pages/)
