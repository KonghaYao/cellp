# Next.js / OpenNext on cellp（实验路径）

> **状态：** 非一等公民 · **不**纳入 [support-matrix.md](../support-matrix.md) tier-1 门禁 · **S30 根页与 S40 基础 App Router 固定 artifact 已通过实验验收**
> **决策：** [decisions.md §18 AD-13](../decisions.md#18-ad-13--前端框架一等公民与-nextjs-边界)  
> **对照：** [framework-coverage-cellp.md](../framework-coverage-cellp.md)  
> **Hard problem（闭环实录）：** [S30-OPENNEXT-HARD-PROBLEM.md](./S30-OPENNEXT-HARD-PROBLEM.md)

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

**历史（2026-09-03）：** v22 prod 曾为 400（`protocol-relative URL (//)`）；后续 artifact 补丁消除了该错误串，`setImmediate` 修复又独立消除了 preview hang。

**S30 本地结论（2026-09-04）：** celld 的 bare `url` 原先回落到 callable stub，使 Next 将 `/` 误判为 `/_next/image` 请求并渲染 404。新增真实 `url` / `node:url` lazy builtin 后，全新 v55 `ready`，preview/prod `GET /` 都为 **200**，标题为 `Create Next App`，静态 CSS 也为 **200**。详见 [ISSUE-05](./issues/ISSUE-05-opennext-proto-relative-get-root.md)。

### S30 实验验收（当前）

| # | 项 | 状态 | 说明 |
|---|-----|:----:|------|
| 1 | deploy → `ready` | ✅ | v55 · 复用已构建 artifact，经 RustFS 同步 |
| 2 | preview `GET /` → **200** | ✅ | 0.082s · 12,301 B |
| 3 | prod `GET /` → **200** | ✅ | 0.201s · `<title>Create Next App</title>` |
| 4 | `/_next/static/...` | ✅ | CSS 200 · 24,703 B |
| 5 | 非 308 / 400 / 404 / hang | ✅ | slash、artifact proto-rel、`setImmediate`、`node:url` 四个独立问题均已闭环 |
| 6 | AD-13 tier-1 | ❌ | 产品决策未改；单一 PoC 通过不等于通用支持承诺 |

### S40 基础 App Router 实验验收（2026-09-05）

S40 与 S30 的 Cloudflare starter 不同：它取自 `vercel/next.js` 的基础 `examples/hello-world`，再叠加最小 App Router 动态页面与 Route Handler，以验证 runtime，而不是重复验证同一模板。

| 项 | 固定值 / 结果 |
|----|---------------|
| 上游源码 | `vercel/next.js` commit `6685283fe8533a469ee1a9455e2bc4047c7453cb` · `examples/hello-world` |
| artifact 依赖 | Next.js `16.0.7` · React `19.2.1` · `@opennextjs/cloudflare` `1.14.0` · Wrangler `4.123.0` |
| artifact 形态 | `.cellp-bundle/index.js` + `.cellp-assets` · `no_bundle: true` · `nodejs_compat` |
| preview gate | v11：`/` 200 + `Hello, Next.js!`；静态 chunk 200；`/dynamic` 两次 SSR 时间戳不同；`/api/health` 200、`pathname=/api/health` 且两次时间戳不同；不存在路由由 Next 返回 404 |
| promote | 仅在上述 preview gate 全绿后执行；prod v11 再验 200 |
| 证据 | `docs/evidence/support-S40.log` · overlay `dev/examples/support-next-basic/` |
| AD-13 tier-1 | ❌；这是固定源码、固定依赖、固定 artifact 的实验性兼容证据，未覆盖任意 Next/OpenNext 版本、middleware、image optimizer、缓存或完整 Node API |

S40 首次运行暴露 celld `Request` 构造器使用公开 `this.url` 的兼容缺陷：Next 风格 getter-only 子类会抛错。修复改为内部 URL slot 并增加 getter-only 子类复制/clone 回归测试；没有修改 Route Handler 来绕过错误。

**celld（2026-09-03）：** 同源 `fetch` loopback 在**外层 fetch 未 settle** 时须同 isolate 完成（`finish_turn` 持 `CurrentGuard` + `wake` 见嵌套 event depth）；`op_fetch` egress 对 canonical origin fail-closed。OpenNext `_next/image` 需 **`CelldHttpBodyStream` BYOB/`readAtLeast`**（`harness.js` + `byte_streams.js` 协议）。复验前需 **rebuild celld** 并 redeploy S30（本任务不 deploy）。

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

## 6. 验收标准（实验槽位）

实验通过最低线；S40 的实际 gate 在此基础上增加动态 SSR、Route Handler payload/动态性与 Next 原生 404：

1. `GET /` prod Host → **200**（或预期 3xx 登录链，**单 Worker**）
2. 无 celld deploy 阶段 esbuild loader 错误
3. 关键客户端路由可刷新（SPA / OpenNext 静态回退）

---

## 7. 后续工程

1. 扩展独立样本和版本组合，覆盖 middleware、缓存、图片路径及更广泛 Node API；失败用例保留，不用应用级 workaround 制造通过。
2. 所有新样本先运行项目 `smoke-preview.sh`；任何关键检查失败时拒绝 promote。
3. **不**把 Next 标为一等公民，除非 AD-13 修订且更广泛门禁通过。
