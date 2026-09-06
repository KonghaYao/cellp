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
| preview gate | v12：`/` 200 + `Hello, Next.js!`；静态 chunk 200；`/dynamic` 两次 SSR 时间戳不同；`/api/health` 直接解析 `request.url` 后返回 200、`pathname=/api/health` 且两次时间戳不同；不存在路由由 Next 返回 404 |
| promote | 仅在上述 preview gate 全绿后执行；prod v12 再验 200 |
| 证据 | `docs/evidence/support-S40.log` · overlay `dev/examples/support-next-basic/` |
| AD-13 tier-1 | ❌；这是固定源码、固定依赖、固定 artifact 的实验性兼容证据，未覆盖任意 Next/OpenNext 版本、middleware、image optimizer、缓存或完整 Node API |

S40 首次运行暴露 celld `Request` 构造器使用公开 `this.url` 的兼容缺陷：Next 风格 getter-only 子类会抛错。修复改为内部 URL slot 并增加 getter-only 子类复制/clone 回归测试；没有修改 Route Handler 来绕过错误。

### OpenNext 官方核心套件（2026-09-05）

固定上游 `opennextjs/opennextjs-cloudflare` tag `@opennextjs/cloudflare@1.14.0`（commit `a644ee1597de577632a29af9a005684554607b2f`），使用其 Next.js `15.5.6`、React `19.0.0`、Wrangler `4.49.1` 与 Playwright `1.51.1`。clone 位于 gitignored 的 `dev/support-corpus/`；harness 只改部署 identity、fixture wrangler 的 cellp main/assets/URL、preview Host 与启动方式，并删除 celld 不支持的部署 flags；不改上游 Playwright assertion、routes、middleware 或 Next 应用源码，且所有官方 suite version 均未 promote。

Playwright `--list` 是分母权威：**119 declarations，5 个上游 `test.skip`，114 runnable**。

| Fixture | Runnable | Build | Preview deploy | Playwright 结果 |
|---------|---------:|:-----:|:--------------:|-----------------|
| App Router | 58 | ✅ | ✅ | **52 passed / 5 failed / 1 flaky / 3 skipped**；失败集中在 ISR/cache 与 middleware redirect；最新证据 `docs/evidence/opennext-official-e2e-20260906-134943-81233.log` |
| Pages Router | 36 | ✅ | ✅ | **33 passed / 3 failed**；失败集中在带 query 的 rewrite 与 trailing-slash redirect query 保留；no-patch 新 preview 稳定复现 |
| App + Pages Router | 20 | ✅ | ✅ | **17 passed / 3 failed**；失败为 request Host、middleware redirect、Server Actions；主套件 ISR 首轮通过，后续同 preview 无 retry repeat-3 复现 1 次失败，确认 flaky |

2026-09-06 增量（仍 **experimental / tier-1 不支持**）：

- **R2 bulk import 证据链已补齐：** `celld r2 bulk put` 接受官方 `{key,file}` manifest，写入真实 RustFS fleet bucket；`e2e/scripts/v13-r2-branch.sh` 证明 child overlay 下 delete→put 与 operator bulk import 后 Worker R2 binding 可 byte-for-byte 读回。
- **OpenNext harness 安全修复：** preview 部署前必须已有 production，且 `set +e` 下 fail-closed，不再在拒绝 first-ready 后继续 staging/deploy。
- **OpenNext cache 导入路径已接入 harness：** 使用 pinned `@opennextjs/cloudflare@1.14.0` 的 `getCacheAssets()` + `computeCacheKey()` 生成 canonical `{key,file}` 清单，再导入目标 version 的 `NEXT_INC_CACHE_R2_BUCKET`。
- **App Router 最新实测：** 58 runnable 中 **52 passed / 5 failed / 1 flaky / 3 skipped**（约 **91.4% runnable pass rate**）。这 **不是** tier-1 或整体 OpenNext 支持率声明；Pages / App+Pages 仍保留既有失败项，且 App Router 尚未按修复后的 cache 导入路径重跑。

两个成功部署 fixture 的主套件合计实际执行 56 个 runnable：最终结果为 **50 passed / 6 failed，已执行断言通过率 89.3%（50/56）**；其中一个 pass 已被后续 repeat-3 再次复现为 flaky，因此严格稳定通过为 **49/56（87.5%）**。相对全部 114 runnable，主套件最终通过覆盖为 **43.9%（50/114）**，严格稳定通过覆盖为 **43.0%（49/114）**，另有 **58/114（50.9%）因 App Router 部署阻塞而未执行 assertion**。因此不能把 89.3% 宣称为“OpenNext 整体支持率”，也不能把未运行的 58 个伪装成断言失败或从总分母隐藏；当前结论仍是 **AD-13 experimental / tier-1 不支持**。

运行入口：`./dev/scripts/run-opennext-official-e2e.sh`；可用 `--collect-only` 验证固定分母，或以 `--only` 单独运行 fixture。证据为 `docs/evidence/opennext-official-*.log`。

#### 失败归因与责任边界

归因前先消除两类 harness 污染：生成的 Playwright `baseURL` 不再带末尾 `/`；官方 fixture 显式设置 `CELLP_OPENNEXT_SKIP_PATCH=1` 与 `CELLP_OPENNEXT_SKIP_NEXT_CONFIG_PATCH=1`，因此不继承 S30 的生成 bundle 或 `next.config.ts` patch。上游 assertion 与应用源码未修改。no-patch 新 preview 结果：

- App Router `v-oncf-app-router-1788616744-24413`：build 成功，activation 超时，celld `instantiate: <none>`；58 项 Playwright 均未执行。
- Pages Router `v-oncf-pages-router-1788614929-19670`：33 pass / 3 stable fail。
- App + Pages Router `v-oncf-app-pages-router-1788614744-1071`：主套件 17 pass / 3 stable fail；ISR 主套件首轮通过，后续同 preview 无 retry repeat-3 复现 1 次失败，确认为 flaky。

| 项目 | 可复现证据 | 责任边界与排除项 | 置信度 | Owner | 补救与复验门禁 |
|------|------------|------------------|--------|-------|----------------|
| App Router 58 项 deployment-blocked | artifact 的 `index.js` 静态 import 两个 `*.wasm?module`，并带一个动态 `.ttf.bin`；`celld/deploy.rs` 的 no-bundle 收集只接受扩展名恰为 `.wasm` 或 `.js/.mjs/.cjs`；version 在 warm isolate 报 `stateless Worker failed to load` / `instantiate: <none>` | OpenNext build 与 Wrangler dry-run已成功；celld publication 会漏掉 `?module` sidecar，随后在 activation 的 V8 module linking 停止。尚未进入 binding/DO 实例化或 HTTP，不能归因为 cache DO assertion 失败。漏收 sidecar 是**已证实的 deploy 缺陷**；它是否为唯一 blocker 需修复后复验 | 高 | celld deploy/module owner | 按 import specifier 原名发布 `*.wasm?module`，补 no-bundle module-closure 校验与 `.bin` 策略；门禁为 version `ready`、无 instantiate 错误、58 runnable 全部真正进入 Playwright，且不删除 DO/R2/service binding |
| `Request.url is host` | Gateway、direct celld + forwarded 两条请求都返回 synthetic URL；direct celld + public Host 返回 public URL。managed celld 的单项环境核对为 `CELLD_TRUST_FORWARDED_HEADERS=1`；Gateway 测试也证明最初会发送 public `X-Forwarded-Host`；官方 Wrangler localhost baseline 通过 | 已排除 baseURL 末尾 `/`、manager 未设 trust flag、陈旧 celld、Gateway 未发 public forwarded authority，以及 upstream fixture 的独立失败。OpenNext `@opennextjs/aws` edge converter 在 middleware handoff 无条件执行 `"x-forwarded-host": result.internalEvent.headers.host`，把 cellp synthetic Host 覆盖回 forwarded header | 高 | OpenNext integration owner；cellp ingress owner协同 | 上游/适配层保留已有 public forwarded authority，或定义不暴露 synthetic Host 的内部 ingress 契约；门禁为 `/api/host` JSON URL 与 public preview baseURL（含 `:8787`）完全相等 |
| Server Actions | 同一 no-patch celld 日志明确记录 synthetic `x-forwarded-host` 与 public `Origin` 不一致，随后 `Invalid Server Actions request`；Playwright 点击后目标文案未出现；官方 Wrangler localhost baseline 通过 | 与 Host 项同一 converter 覆盖链；Next 的 CSRF/Origin 校验按设计 fail-closed。已排除 upstream fixture 的独立失败和测试环境随机超时，也没有证据指向 cache/DO | 高 | OpenNext integration owner；cellp ingress owner协同 | 修复 public authority 保留后重跑；门禁为 `serverActions.test.ts` 绿，日志无 forwarded-host/Origin mismatch 与 `Invalid Server Actions request` |
| middleware redirect | 三条 A/B 均返回 307；Gateway/direct-forwarded 的 `Location` 是 `https://synthetic...`，direct-public 是 `https://public...:8787`。官方 middleware 对非 `localhost` Host 固定选 `https`；官方 Wrangler `http://localhost` baseline 通过 | synthetic authority 来自上述 converter；即使 public Host 正确，`lvh.me` 仍触发 HTTPS，而本地 `:8787` 只提供 HTTP。AD-10 明确 TLS 由外层入口负责，不能修改官方 middleware 制造 PASS | 高 | dev acceptance environment / 外层 TLS owner；OpenNext integration owner处理 synthetic Host | 在真实 HTTPS public origin 或带 TLS 终止的 preview 环境运行官方用例；门禁为 307 `Location` 使用 public authority，浏览器成功到达 `/redirect-destination` |
| Pages rewrite/query/trailing 3 项 | no-patch 稳定复现：rewrite 页面无 `SSR`；merge 只返回 `q=1`、缺 `b=2`；trailing 最终 URL 为 `/ssr/?`、缺 `happy=true`。Gateway、direct-forwarded、direct-public 三条 A/B 都产生相同结果；直接 `/api/query?b=2&q=1` control 三条均完整返回两个参数。同 commit、同 build 的官方 Wrangler localhost baseline 中 rewrite/trailing 文件 **7/7 通过** | 已排除 Gateway、celld 的一般性 query 截断以及 upstream fixture/assertion 在官方 runtime 上的独立失败；差异确定落在 celld 执行该 OpenNext artifact 时的 rewrite/trailing URL 兼容路径，精确到 self-fetch、absolute `Request.url` 或 redirect normalization 中哪一步仍需插桩 | 中高；平台责任边界已确定，具体函数待定位 | celld HTTP/self-fetch owner；OpenNext integration owner协查 | 对相同 raw target 在 celld URL 构造、OpenNext internal request 和最终 `Location` 处记录非敏感 path/query 元数据并与 Wrangler 对照；门禁为两个 rewrite 与 trailing 用例全绿，且 control 继续保留 query |
| ISR flaky（已复现） | 早期 preview 首次失败、retry 通过；本次 no-patch 主套件首轮通过。随后在同一 preview、`--retries=0 --repeat-each=3` 下，App Router ISR **1/3 失败**而 Pages ISR **3/3 通过**；同 commit Wrangler baseline 两类 ISR **6/6 通过**。失败轮启动时 celld 同时记录 `dynamic import of "./.next/prerender-manifest.json" is not supported` | 已排除仅由 upstream fixture 稳定性造成；flaky 属于 celld/OpenNext cache 兼容路径。动态 JSON import 与失败同轮相关，但仍不能证明它是唯一原因，R2/DO/cache 时序待隔离 | 中；平台边界已确定，唯一根因待实验 | celld dynamic-import/cache owner；OpenNext cache owner协查 | 在全新 preview 重复相同无 retry gate，并分别补齐/禁用候选动态 import 路径做 A/B；门禁为多份 fresh preview 连续多轮两类 ISR 全绿且相关动态 import 错误消失 |

**Wrangler 对照已完成：** 使用同一 pinned commit 与已构建 artifact，在隔离临时目录以原始 `wrangler.jsonc`、官方 `http://localhost` origin 和空 env file 运行，未读取 fixture dotenv。Pages rewrite/trailing **7/7 通过**；Mixed Host、middleware redirect、Server Actions **3/3 通过**；两类 ISR 无 retry 重复三次 **6/6 通过**。这把 Pages 三项和 ISR 的责任边界收窄到 celld/OpenNext 兼容层，但 ISR 的唯一根因仍未确定。

**证据索引：** `opennext-official-e2e-20260905-212313-44020.log`（119/5/114）；`opennext-official-e2e-20260905-215903-47835.log`（App Router no-patch blocker）；`opennext-official-pages-router-v-oncf-pages-router-1788614929-19670.log`；`opennext-official-app-pages-router-v-oncf-app-pages-router-1788614744-1071.log`；`opennext-official-wrangler-pages-baseline-20260905.log`；`opennext-official-wrangler-mixed-baseline-localhost-20260905.log`；`opennext-official-wrangler-mixed-isr-repeat3-20260905.log`；`opennext-official-cellp-mixed-isr-repeat3-20260905.log`。失败 version 与 Playwright traces 均保留，未 promote。

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
