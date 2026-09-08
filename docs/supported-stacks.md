# 支持的技术栈（cellp v1）

> **运行时：** celld 0.4.x（Workers 语义）· **不是** Node / Next.js 托管平台。  
> **框架 verdict（社区验证）：** [support-matrix.md](./support-matrix.md) · **AD-13** [framework-coverage-cellp.md](./framework-coverage-cellp.md)

---

## 支持

| 类别 | 说明 |
|------|------|
| **Cloudflare Workers** | `export default { fetch() }` 形态 |
| **wrangler.jsonc / json** | bundle 内由 celld deploy 解析；**无** `wrangler.toml` 生命周期 |
| **D1** | import / branch · Dashboard D1 browser |
| **KV / R2 / Queue** | operator API + Dashboard（R2 无对象浏览器） |
| **Workflow / Cron** | Workflow 只读 list；Cron 展示 only（**武装仅 prod**，AD-11） |
| **静态 assets** | wrangler `assets` / Workers Sites 模式（celld 支持范围内） |
| **外部 CI** | 任意 Git host + `POST /versions` |
| **Gateway** | **Host ingress**（AD-12）；preview / prod Host；path 选 version **已废弃** |
| **Native Component（实验）** | **`native-http-v1`**（AD-16）· Wasmtime · KV + config WIT；见 [Native 文档](https://konghayao.github.io/cellp/build/native-component.html) |

### AD-13 一等公民框架（产品承诺 + S22–S25）

| 框架 | 典型部署 |
|------|----------|
| **React / Vue + Vite SPA** | `dist` + 薄 Worker + `assets` |
| **Astro** | `@astrojs/cloudflare` · **S22** |
| **SvelteKit** | `adapter-cloudflare` **单 Worker** · **S23** |
| **Remix** | `@remix-run/cloudflare` · **S24** |
| **Nuxt** | Nitro `cloudflare_module` · **S25** |

社区已额外验证（**非 AD-13 一等**，见矩阵）：Hono、SolidStart、Qwik、Waku（**S26–S29**）等。

---

## 不支持 / 非目标

| 类别 | 说明 |
|------|------|
| **Next.js 一等公民 / OpenNext 全量** | **S30/S40 实验**；矩阵仍为 **不支持** — [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) |
| **Next.js SSR / App Router on Node** | 用 Vercel / Node 平台；cellp 不跑 Node serverless |
| **多 `[[services]]` Worker 编排** | cellp 不编排 service binding — [MULTI-WORKER-DEPLOY.md](./plans/MULTI-WORKER-DEPLOY.md) |
| **Edge Middleware（Next）** | 非 Workers bundle |
| **Pages Functions（非 Workers 形态）** | 需改为 Workers 部署流 |
| **Workers AI / Vectorize / Hyperdrive / Analytics Engine** | celld 未实现或未对齐 — 见 celld compat 与 support 矩阵 |
| **npm 任意 Node 依赖** | 仅 Workers 兼容包 |
| **账号 / Git 托管 / DNS / CDN / TLS / WAF** | AD-10 边界 |

---

## 迁移

- 对外：[Pages — Migrate](https://konghayao.github.io/cellp/migrate/cloudflare)
- 内部：[cloudflare-migration.md](./cloudflare-migration.md) · [vercel-migration.md](./vercel-migration.md)
