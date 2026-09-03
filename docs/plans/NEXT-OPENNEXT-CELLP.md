# Next.js / OpenNext on cellp（实验路径）

> **状态：** 非一等公民 · **不**纳入 [support-matrix.md](../support-matrix.md) 门禁  
> **决策：** [decisions.md §18 AD-13](../decisions.md#18-ad-13--前端框架一等公民与-nextjs-边界)  
> **对照：** [framework-coverage-cellp.md](../framework-coverage-cellp.md)

---

## 1. 产品立场

| 项 | 结论 |
|----|------|
| **cellp Dashboard (`web/`)** | 固定 **Vite + React SPA**；不引入 Next.js SSR / App Router |
| **用户 Worker 项目** | 可与 CF 一样尝试 **OpenNext** 产出物，但 **无** cellp 官方模板与 support 槽位承诺 |
| **与 Vercel** | Next 全栈 SSR 仍优先 Vercel / Node；cellp 目标是 **Workers 语义 + 私有化** |

CF 侧 Next 依赖 **OpenNext** 或 **vinext**，将 App Router 编译为 Worker + 资产目录；cellp **不**运行 `wrangler deploy`，只消费 **artifact 目录 + wrangler.json(c)**。

---

## 2. 为何容易失败（today）

| 风险 | 原因 |
|------|------|
| **celld 二次 esbuild** | OpenNext 输出含大量 `.open-next/`、Node 兼容层；celld `deploy` 再 bundle 易挂（同 S16 pastebin） |
| **构建图过大** | `.md`、CSS、Tailwind、动态 import 在 Worker 图内 |
| **多入口 / middleware** | 与单 `main` + `assets` 假设冲突 |
| **nodejs_compat** | 部分 OpenNext 路径依赖 Node API；需 celld `node_crypto` / compat 矩阵对齐 |

**原则：** CI **预构建** → artifact 内 **`no_bundle: true`**（或等价：仅上传已编译 `index.js`）→ celld **只**解析 wrangler + 挂载静态资产。

---

## 3. 推荐优化流水线（对齐 support 套路）

与 S08 / S14 / S19 已验证的 **prepare-artifact** 一致。

**S30 PoC（2026-09-03）固定路径：**

| 项 | 值 |
|----|-----|
| 上游 | [cloudflare/templates `next-starter-template`](https://github.com/cloudflare/templates/tree/main/next-starter-template) |
| 构建 | `npm run build` → `npx opennextjs-cloudflare build` → `.open-next/worker.js` + `.open-next/assets` |
| celld 入口 | `wrangler deploy --dry-run --outdir .cellp-bundle` → `main: .cellp-bundle/index.js`，`no_bundle: true` |
| 静态 | `.open-next/assets` → `.cellp-assets`（wrangler `assets.directory`） |
| overlay | `dev/examples/support-opennext/` · deploy `S30` |

```text
1. CI: opennext build（或 @opennextjs/cloudflare 文档命令）
2. 将 worker 入口固定为单一文件（如 .open-next/worker.js 或 wrangler 指定 main）
3. prepare-artifact.sh:
   - wrangler deploy --dry-run --outdir .cellp-bundle（若上游支持）
   - 或 esbuild 一次打包，celld 侧 no_bundle
4. stage-artifact-extra.sh:
   - rsync .open-next/assets / _next/static → .cellp-assets 或 wrangler [assets].directory
5. wrangler.cellp.jsonc overlay:
   - compatibility_flags: ["nodejs_compat"]（按需）
   - assets + not_found_handling: "single-page-application"（若适用）
6. POST /versions → Host ingress 验收
```

**S30 本地结论（2026-09-03 末批）：** deploy **v22** ready + promote · prod **400**（`protocol-relative URL (//)`）· **308** / **500** 已消除 · 矩阵 **不支持**（实验，非 tier-1）。

### S30 tier-1 验收不通过项（当前）

| # | 项 | 状态 | 说明 |
|---|-----|:----:|------|
| 1 | deploy → `ready` | ✅ | v22 · prepare 10 patch · `docs/evidence/support-S30.log` |
| 2 | prod `GET /` → **200** | ❌ | Actual **400** · 54 B JSON |
| 3 | body 含 `<!DOCTYPE` / Next 首页 | ❌ | 无 HTML |
| 4 | 非 308 `Location: ?` | ✅ | slash + `request.url` 补丁 |
| 5 | 非 500 cookie/`node:http` | ✅ | celld `8a7bfaa` `node_http.js` |
| 6 | **`//` protocol-relative URL** | ❌ | OpenNext `_next/image` 或 SSR 仍生成/校验失败；bundle 内已有部分 guard 仍不足 |

**复验：** `curl -H 'Host: support-opennext.lvh.me' http://127.0.0.1:8787/`

**禁止：** 把完整 monorepo `node_modules` 打进 artifact（用 slim stage，见 `deploy-support-app.sh`）。

---

## 4. wrangler / cellp overlay 要点

- **`PUBLIC_BASE_URL` / `__CELLP_DEPLOY_URL__`：** overlay 注入 preview/prod Host（AD-12）。
- **strip：** `deploy-support-app.sh` 会删 `routes`、`workers_dev` 等 celld 不吃的键；OpenNext 生成的 wrangler 需 **合并** 而非整文件覆盖上游（见 [MULTI-WORKER-DEPLOY.md](./MULTI-WORKER-DEPLOY.md) §2 讨论）。
- **D1 / KV：** 与任意 Worker 相同；子 version branch 见 AD-8。

---

## 5. 与 Cloudflare 文档的对照

| CF | cellp |
|----|--------|
| `wrangler deploy` + 账号绑定 | `POST /versions` + version bucket |
| Workers Builds / Pages | **外部 CI** 构建 artifact |
| OpenNext 官方 guide | 本文件 + **celld** [cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) |

参考（需自行打开 CF 文档核对版本）：

- [OpenNext on Cloudflare](https://developers.cloudflare.com/workers/framework-guides/web-apps/next-js/)
- CF **vinext** 为另一编译路径；cellp 未单独验收

---

## 6. 验收标准（若将来开实验槽位）

**不**写入 support-matrix，除非显式新 ADR。实验通过最低线：

1. `GET /` prod Host → **200**（或预期 3xx 登录链，**单 Worker**）
2. 无 celld deploy 阶段 esbuild loader 错误
3. 关键客户端路由可刷新（SPA / OpenNext 静态回退）

---

## 7. 下一步（工程）

1. 从 `cloudflare/templates` 的 `next-starter-template` 或 OpenNext 官方样例做 **一次性** PoC（`dev/examples/support-next/` overlay，**不**占 S22–S25）。
2. 将 PoC 结论回写本文件 §3（固定 main 路径与 assets 目录名）。
3. **不**把 Next 标为一等公民，除非 AD-13 修订。
