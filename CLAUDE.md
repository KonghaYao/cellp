@AGENTS.md

# Support 与框架验证 — 标准流程（Claude Code）

> **口径：** [docs/support-matrix.md](./docs/support-matrix.md) 仅 **支持** / **不支持**。  
> **队列：** [docs/support-todos.md](./docs/support-todos.md) · AD-13 框架 S22–S25。

## 原则

1. **用户不手跑脚本** — 本机 `health` / `deploy` / `curl` 由 **subagent** 或主 agent 的 Bash 执行。
2. **两阶段：** **coder** 部署与矩阵判定 → **verification** 端口级**用户行为**验收（非只看 HTTP 200）。
3. **不在应用里长期 polyfill** `cloudflare:workers` / `caches` — 缺口记 [platform-defects-log.md](./docs/platform-defects-log.md)，修 **celld**。
4. **不提交** `dev/support-corpus/`；证据日志在 `docs/evidence/`（gitignore）。

## 环境默认值

| 项 | 值 |
|----|-----|
| Ingress | `lvh.me` · Gateway `:8787` · `Host: support-<project>.lvh.me` |
| Git clone | 默认 ghfast；直连：`GITHUB_CLONE_DIRECT=1` |
| npm | **`https://registry.npmmirror.com`**（`deploy-support-app.sh` 设 `NPM_CONFIG_REGISTRY`，`dev/.env` 可覆盖） |
| RustFS 502 / skew | `./dev/scripts/fix-rustfs-skew.sh` |

## 阶段 A — `coder`（`subagent_type: coder`）

**必须自己跑命令。**

```text
1. ./dev/scripts/health.sh
   → 失败：./dev/scripts/fix-rustfs-skew.sh 或 ./dev/scripts/up.sh
2. GITHUB_CLONE_DIRECT=1 ./dev/scripts/deploy-support-app.sh <S-id>
3. 失败则 dev/examples/support-<project>/：
   wrangler.cellp.jsonc, prepare-artifact.sh, stage-artifact-extra.sh
   （对齐 support-astro：slim artifact，wrangler dry-run / 原生 worker 树，无 runtime polyfill）
4. 更新 docs/support-matrix.md、docs/support-todos.md
5. 输出：支持 / 不支持 + 原因 + prod URL
```

**Prompt 模板（替换 `<S-id>`、框架名、验收路径）：**

```markdown
Repo: /Users/mino/code/remote/cellp. Run all commands yourself.
Deploy and validate <S-id> per deploy-support-app.sh lookup.
npm npmmirror is default. On failure add dev/examples/support-*/ overlay like support-astro.
Update support-matrix (支持/不支持). No corpus commit.
Return: Scope, Result, HTTP table, Files changed.
```

## 阶段 B — `verification`（`subagent_type: verification`）

**模拟用户行为**（HTML 标题/正文、跟链、静态资源），禁止只报状态码。

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
Repo: /Users/mino/code/remote/cellp. User-behavior acceptance on :8787 for <Host>.
Define 3–6 steps a real user would take (home, inner pages, assets).
Write/update docs/support-framework-user-acceptance.md section ## Sxx.
VERDICT: PASS/FAIL per app.
```

## 参考 overlay

| ID | 目录 |
|----|------|
| S22 Astro | `dev/examples/support-astro/` |
| S23 SvelteKit | `dev/examples/support-sveltekit/` |
| S24 Remix | `dev/examples/support-remix/` |

## 文档产出

| 文件 | 谁写 |
|------|------|
| `docs/support-matrix.md` | coder |
| `docs/support-framework-user-acceptance.md` | verification |
| `docs/platform-defects-log.md` | coder（仅新平台缺口） |

## Ultracode 工作流

| 任务 | 脚本 | 并发 |
|------|------|------|
| Nitro loopback：并行 review → 实现 → review-fix → **verify S25** | `.claude/workflows/nitro-loopback-ultracode.mjs` | `maxConcurrency: 5` |
| CF Web Crypto | `.claude/workflows/cf-web-crypto-ultracode.mjs` | — |
| cloudflare:workers polyfill | `.claude/workflows/cf-workers-polyfill-ultracode.mjs` | — |

阶段：`ParallelReview` → `ParallelImplement` → `ReviewFix` → `VerifyNitro`。设计依据 `docs/plans/NITRO-CELLD-LOOPBACK-DESIGN.md` v0.2。

## Subagent 选型

| 任务 | Agent |
|------|--------|
| 部署、overlay、改脚本 | **coder** |
| 独立复验、用户旅程 | **verification** |
| 只读搜代码 | explorer（**不能**代替部署） |

详见 [docs/README.md § Subagent](./docs/README.md#subagent-派发约定)。
