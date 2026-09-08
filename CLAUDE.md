@AGENTS.md

# Claude Code — cellp 工作手册

> **文档同步：** 2026-09-08  
> **架构决策权威：** [docs/decisions.md](./docs/decisions.md)（**AD-1 … AD-16** · D1 · 存储 tier · Bindings）  
> **使用者产品文档：** [GitHub Pages](https://konghayao.github.io/cellp/)（源码 `site/`）

本文件在 `@AGENTS.md` 之上补充 **Claude Code 会话** 的文档地图、通用派发约定，以及 **Support / 框架 / Agent 应用** 标准流程。改控制面行为时同步 `DESIGN.md`、`decisions.md` 与 `site/`；勿只改内部 ADR 而不同步对外站点。

---

## 文档层级（避免读错入口）

| 受众 | 入口 | 说明 |
|------|------|------|
| 终端用户 / 迁移 | [site/](https://konghayao.github.io/cellp/) | 唯一对外口径 |
| 贡献者 / Agent | [AGENTS.md](./AGENTS.md) · 本文件 | 仓库地图、验证、禁止项 |
| 架构与 ADR | [DESIGN.md](./DESIGN.md) · [docs/decisions.md](./docs/decisions.md) | 实现与审查以 decisions 为准 |
| 功能门禁 | [docs/test-plan.md](./docs/test-plan.md) | TP-* · M1/M2 |
| 内部索引 | [docs/README.md](./docs/README.md) | 契约、runbook、证据 |
| 过程文档 | [docs/process/README.md](./docs/process/README.md) | 交付摘要、合并 handoff |
| 本地栈 | [dev/AGENTS.md](./dev/AGENTS.md) · [dev/INGRESS-HOST.md](./dev/INGRESS-HOST.md) | RustFS · cellpd · Host |
| Dashboard | [web/AGENTS.md](./web/AGENTS.md) | Vite SPA · 仅 `:8790` API |

**过时信号：** 文中若仍写 **AD-1..14**、path 选 version、或 Next 为一等公民 → 以 [decisions.md](./docs/decisions.md) 与 [support-matrix.md](./docs/support-matrix.md) 为准并修正。

---

## 决策摘要（2026-09 现行，细节见 decisions）

| AD | 要点 |
|----|------|
| **AD-1** | 每 ready version = 独立 celld + bucket；watch 临时；S3/RustFS 持久 |
| **AD-4** | Dev local offshoot；**prod RustFS offshoot = TP-V0b**（✅ 已 PASS，压测须标 `offshoot_tier`） |
| **AD-5** | Promote saga + 补偿 |
| **AD-6–8** | celld 0.4.0 绑定；子 version **D1+KV+R2+Queue branch** |
| **AD-9** | archived / wake；无 ready 硬上限 |
| **AD-10** | 不做账号/Git/DNS/CDN/TLS/WAF/全球边缘 |
| **AD-11** | Cron **仅 prod** version 武装 |
| **AD-12** | Gateway **Host**（+ 可选 listen port）；**path 选 version 废弃** |
| **AD-13** | 一等公民框架 **S22–S25**（Astro/SvelteKit/Remix/Nuxt）；Next/OpenNext **非一等**（S30/S40 实验） |
| **AD-14** | OTLP + 查询门面；[OTEL-OBSERVABILITY.md](./docs/plans/OTEL-OBSERVABILITY.md) |
| **AD-15** | 弹性 Serving / scale-to-zero（Proposed 设计包 [SURGE-DESIGN-INDEX.md](./docs/plans/SURGE-DESIGN-INDEX.md)） |
| **AD-16** | Experimental **`native-http-v1`** · `dev/examples/native-wasm/` · `v18-native-wasm` |

---

## 通用 Agent 原则

1. **用户不手跑运维** — `health` / `up` / `deploy-support-app` / `curl` 验收由 **coder**、**general-purpose** 或主 agent 的 Bash 执行；**explorer 只读**，不能代替部署。
2. **E2E 默认子集** — `./e2e/scripts/run-all.sh --only …`；全量 **10–20+ 分钟**，仅用户/CI/M2 明确要求时跑完整 MANIFEST。
3. **证据** — `mkdir -p docs/evidence`；日志 gitignore，勿提交 `dev/support-corpus/` 克隆体。
4. **冻结契约** — `docs/plans/D1-*-RPC.md` 未经审查不改。
5. **Subagent 派发** — [docs/README.md § Subagent](./docs/README.md#subagent-派发约定)。

---

## Support 与框架验证 — 标准流程

> **唯一 verdict 口径：** [docs/support-matrix.md](./docs/support-matrix.md)（仅 **支持** / **不支持**）  
> **队列：** [docs/support-todos.md](./docs/support-todos.md) · **AD-13** 一等公民 **S22–S25** · 扩展框架 **S26–S29** · 高 Star **S31–S39** · Next 实验 **S30/S40** · Coding Agent **A01–A05**

### 原则

1. **两阶段：** **coder** 部署与矩阵判定 → **verification** 端口级**用户行为**验收（非只看 HTTP 200）。
2. **不在应用里长期 polyfill** `cloudflare:workers` / `caches` — 缺口记 [platform-defects-log.md](./docs/platform-defects-log.md)，修 **celld**。
3. **多 `[[services]]` Worker** — 平台不支持 → 依赖它的仓库一律 **不支持**（[MULTI-WORKER-DEPLOY.md](./docs/plans/MULTI-WORKER-DEPLOY.md)）。

### 环境默认值

| 项 | 值 |
|----|-----|
| Ingress | `lvh.me` · Gateway `:8787` · `Host: support-<project>.lvh.me` |
| 部署脚本 | `./dev/scripts/deploy-support-app.sh <S-id\|A-id>` |
| Git clone | 默认 ghfast；直连：`GITHUB_CLONE_DIRECT=1` |
| npm | **`https://registry.npmmirror.com`**（脚本设 `NPM_CONFIG_REGISTRY`，`dev/.env` 可覆盖） |
| RustFS 502 / skew | `./dev/scripts/fix-rustfs-skew.sh` |

### 阶段 A — `coder`（`subagent_type: coder`）

```text
1. ./dev/scripts/health.sh
   → 失败：./dev/scripts/fix-rustfs-skew.sh 或 ./dev/scripts/up.sh
2. GITHUB_CLONE_DIRECT=1 ./dev/scripts/deploy-support-app.sh <S-id|A-id>
3. 失败则 dev/examples/support-<project>/：
   wrangler.cellp.jsonc, prepare-artifact.sh, stage-artifact-extra.sh
   （对齐 support-astro：slim artifact，wrangler dry-run / 原生 worker 树，无 runtime polyfill）
4. 更新 docs/support-matrix.md、docs/support-todos.md
5. 输出：支持 / 不支持 + 原因 + prod/preview URL
```

**Prompt 模板（替换 ID、框架名、验收路径）：**

```markdown
Repo: <git root>。Run all commands yourself.
Deploy and validate <S-id|A-id> per deploy-support-app.sh lookup.
npm npmmirror is default. On failure add dev/examples/support-*/ overlay like support-astro.
Update support-matrix (支持/不支持). No corpus commit.
Return: Scope, Result, HTTP table, Files changed.
```

### 阶段 B — `verification`（`subagent_type: verification`）

```text
1. ./dev/scripts/health.sh
2. curl -H "Host: support-<name>.lvh.me" http://127.0.0.1:8787/<paths>
   - 检查 body：无 ingress_unknown、500 页、空 body
   - 至少 1 条站内导航或资源 URL（CSS/logo/内页）
3. 追加章节到 docs/support-framework-user-acceptance.md
4. 每步：URL | HTTP | 用户可见结果 | PASS/FAIL
```

**Prompt 模板：**

```markdown
Repo: <git root>. User-behavior acceptance on :8787 for <Host>.
Define 3–6 steps a real user would take (home, inner pages, assets).
Write/update docs/support-framework-user-acceptance.md section ## Sxx.
VERDICT: PASS/FAIL per app.
```

### 参考 overlay（按任务选读）

| 类别 | ID | 目录 |
|------|-----|------|
| AD-13 一等 | S22 Astro | `dev/examples/support-astro/` |
| | S23 SvelteKit | `dev/examples/support-sveltekit/` |
| | S24 Remix | `dev/examples/support-remix/` |
| | S25 Nuxt | `dev/examples/support-nuxt/` |
| 扩展框架 | S26 Hono · S27 SolidStart · S28 Qwik · S29 Waku | `dev/examples/support-{hono,solidstart,qwik,waku}/` |
| Next 实验 | S30 · S40 | `support-opennext/` · `support-next-basic/`（矩阵仍为 **不支持**） |
| Agent | A01–A05 | `deploy-support-app.sh` · [CODING-AGENT-ON-CELLP.md](./docs/plans/CODING-AGENT-ON-CELLP.md) |

完整 verdict 表：**[support-matrix.md](./docs/support-matrix.md)**（勿在 CLAUDE/AGENTS 里维护重复总表）。

### Support 文档产出

| 文件 | 谁写 |
|------|------|
| `docs/support-matrix.md` | coder |
| `docs/support-framework-user-acceptance.md` | verification |
| `docs/platform-defects-log.md` | coder（仅新平台缺口） |

### Subagent 选型（Support）

| 任务 | Agent |
|------|--------|
| 部署、overlay、改脚本 | **coder** |
| 独立复验、用户旅程 | **verification** |
| 只读搜代码 | **explorer**（**不能**代替部署） |

---

## 改文档时的检查清单

| 变更类型 | 必同步 |
|----------|--------|
| 产品行为 / API | `DESIGN.md` · `decisions.md` · `site/` · `cellp/api/openapi.yaml` |
| Support 判定 | **仅** `support-matrix.md` + `support-todos.md` |
| 框架一等公民 | `decisions` §18 · `framework-coverage-cellp.md` · `site/docs/migrate/` |
| Native Component | `decisions` §21 · `site` Native 页 · `v18-native-wasm` |
| Agent 入口 | 保持 `@AGENTS.md` 与本文 AD 范围一致 |

---

*Claude Code 手册 · 与 AGENTS.md 联动 · 2026-09-08*
