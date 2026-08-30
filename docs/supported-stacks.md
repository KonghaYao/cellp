# 支持的技术栈（cellp v1）

> **运行时：** celld 0.4.x（Workers 语义）· **不是** Node / Next.js 托管平台。

---

## 支持

| 类别 | 说明 |
|------|------|
| **Cloudflare Workers** | `export default { fetch() }` 形态 |
| **wrangler.jsonc / json** | bundle 内由 celld deploy 解析；**无** `wrangler.toml` 生命周期 |
| **D1** | import / branch · Dashboard D1 browser |
| **KV / R2 / Queue** | operator API + Dashboard（R2 无对象浏览器） |
| **Workflow / Cron** | Workflow 只读 list；Cron 展示 only |
| **静态 assets** | wrangler `assets` / Workers Sites 模式（celld 支持范围内） |
| **外部 CI** | 任意 Git host + `POST /versions` |

---

## 不支持 / 非目标

| 类别 | 说明 |
|------|------|
| **Next.js SSR / App Router on Node** | 用 Vercel / Node 平台；cellp 不跑 Node serverless |
| **Edge Middleware（Next）** | 非 Workers bundle |
| **Pages Functions（非 Workers 形态）** | 需改为 Workers 部署流 |
| **Workers AI / Vectorize / Hyperdrive** | celld 未实现 — 见 celld compat |
| **npm 任意 Node 依赖** | 仅 Workers 兼容包 |

---

## 迁移

- [cloudflare-migration.md](./cloudflare-migration.md)
- [vercel-migration.md](./vercel-migration.md)
