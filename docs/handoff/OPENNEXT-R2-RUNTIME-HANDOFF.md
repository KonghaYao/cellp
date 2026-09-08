# OpenNext 实验验收 + R2 bulk 证据链 — 任务交接

> **状态（2026-09-07）：** App Router **harness 修复后已重跑**（55/58 runnable pass，3 fail）；R2 证据链与 v13 门禁已在本机复验。Pages/App+Pages 全量重跑仍为 P1。  
> **产品结论不变：** OpenNext 仍为 **AD-13 experimental / 非 tier-1**，不得夸大兼容声明。

---

## 1. 任务目标（给接手人）

在 cellp 上建立**可复现、可审计**的 OpenNext 实验验收路径，并证明：

1. **R2 bulk 接口**与 OpenNext 官方 cache-population 输出的 `{key,file}` manifest 兼容。
2. 导入对象写入**真实 RustFS/S3** fleet bucket（非仅内存）。
3. 运维侧 `celld r2 bulk put` 写入的字节，可被**已部署 Worker 的 R2 binding** 原样读回（含 binary / overlay / tombstone 场景）。
4. 固定 pin 的 `@opennextjs/cloudflare` 官方 Playwright 套件：真实 build → cache 导入 → preview deploy → runtime 测试（**不 promote**）。

**不在范围内：** 把 Next.js 标为一等公民、改上游 OpenNext fixture 源码/断言、为过测弱化门禁、全栈无条件重启、读取/提交凭据文件。

---

## 2. 已完成工作（可直接依赖）

### 2.1 Git 提交

| 仓库 | Commit | 摘要 |
|------|--------|------|
| `celld/` 子模块 | `c28db43` | Worker R2 写路径在 commit 后清除 child tombstone；新增 `celld r2 bulk put` |
| 根仓库 | `de39f3a` | OpenNext harness fail-closed + 官方 R2 cache 导入；V13 扩展；文档更新 |

根仓库 **未** 提交大量无关 dirty（elastic、s surge 证据、web 等）；接手时 `git status` 仍可能显示其他并行工作，**不要** `git add .`。

### 2.2 R2 / overlay（celld）

- **问题：** 子 version 上 Worker `delete()` 写 tombstone 后，再 `put()` 同一 key 未清 tombstone，overlay 读仍视为已删。
- **修复：** 所有成功写完成点（direct put、`PutStream::finish`、multipart complete）在对象 commit **之后** 再 `clear_tombstone`；条件写失败不清 tombstone。
- **CLI：** `celld r2 bulk put BUCKET --filename manifest.json`，manifest 为 Wrangler 形状 `[{ "key", "file" }]`，相对路径相对 manifest 目录解析；child overlay 下 bulk 写成功后同样清 tombstone。

### 2.3 E2E 证据

- **`e2e/scripts/v13-r2-branch.sh`**（已在 `e2e/scripts/MANIFEST`）：
  - 父写 → 子 fallback 读；子 overwrite；子 delete → 子 Worker put 同 key → 子读 replacement；父不变。
  - 独立 key：官方风格 manifest + **NUL 字节** → `celld r2 bulk put` → 子 Worker GET → `cmp` 字节一致；父不变。

### 2.4 OpenNext harness（dev/scripts）

- **`prepare-opennext-official-e2e.sh`**
  - `oncf_require_existing_prod`：**preview 前**必须已有 production，拒绝 first-ready 自动 bootstrap prod。
  - 在调用方 `set +e` 下，staging/deploy 各步**显式** `|| return $?`，避免“打印 FAIL 仍继续 deploy”。
  - `oncf_populate_r2_cache`：用 pinned 包 `getCacheAssets()` + `computeCacheKey()` 生成 manifest，再 `celld r2 bulk put` 到该 version 的 `NEXT_INC_CACHE_R2_BUCKET`（真实 fleet bucket）。
- **`run-opennext-official-e2e.sh`**
  - preview version id 使用 `localhost-oncf-${fixture}-...`，满足官方 middleware 对 `Host` 以 `localhost` 开头时用 HTTP 的假设（Gateway 仍 `:8787` HTTP）。

### 2.5 文档（已更新，experimental 表述保留）

- `site/docs/bindings/r2.md` — bulk put / OpenNext cache 导入
- `site/docs/get-started/dashboard.md` — 无 R2 浏览器，但有 CLI 导入
- `site/docs/migrate/frameworks.md` — Next experimental + 最新 App Router 数字
- `docs/plans/NEXT-OPENNEXT-CELLP.md` — 内部计划与证据索引
- `dev/README.md` — 官方 e2e 入口与 preview-only 说明

---

## 3. 固定 Pin（勿擅自漂移）

| 项 | 值 |
|----|-----|
| 上游 repo | `opennextjs/opennextjs-cloudflare` |
| Tag | `@opennextjs/cloudflare@1.14.0` |
| Commit | `a644ee1597de577632a29af9a005684554607b2f` |
| Clone 路径 | `dev/support-corpus/opennextjs-cloudflare`（gitignored corpus） |
| Playwright 分母 | 119 listed，5 declared skip，**114 runnable** |

Fixtures：`app-router`（58 runnable）、`pages-router`（36）、`app-pages-router`（20）。

---

## 4. 当前实测结果（接手后需重跑验证）

### 4.1 App Router — 修复 harness **之前**的一次完整 run

- **日志：** `docs/evidence/opennext-official-e2e-20260906-134943-81233.log`
- **结果：** 52 passed，5 failed，1 flaky，3 skipped（58 runnable → **约 91.4% runnable pass**）
- **典型失败：**
  - ISR / revalidate / cache：多表现为 404/500/MISS，与 **未预填充 incremental cache（R2）** 一致 → harness 已加 `oncf_populate_r2_cache`，**该 log 发生在导入路径接入之前**。
  - `middleware.redirect`：`net::ERR_SSL_PROTOCOL_ERROR` — 官方 middleware 对非 `localhost` Host 生成 **https** Location，本地 Gateway 仅 HTTP → version id 已改为 `localhost-oncf-...` **尚未在该 log 对应 run 中验证**。
- **Harness 历史事故：** 该 run 中 `oncf_require_existing_prod` 失败后仍继续 deploy，导致隔离项目 `opennext-e2e-app-router` 的 prod 被设为 `v-oncf-app-router-1788673783-32222`。修复后 prod 检查前置；**建议保留该 prod 作为 preview 安全基线**，勿动 demo-app 等业务 prod。

### 4.2 Pages / App+Pages — 较早证据（见 `NEXT-OPENNEXT-CELLP.md`）

- Pages：33 pass / 3 fail（rewrite/query/trailing）
- App+Pages：17 pass / 3 fail + ISR flaky 记录  
- **不得**把子集通过率宣称为“整体 OpenNext 支持率”。

### 4.3 App Router — harness 修复后重跑（2026-09-07）

- **命令：** `./dev/scripts/run-opennext-official-e2e.sh --only app-router --fast`
- **日志：** `docs/evidence/opennext-official-e2e-20260907-135945-61847.log`（Playwright 明细：`docs/evidence/opennext-official-app-router-localhost-oncf-app-router-1788760786-29887.log`）
- **version：** `localhost-oncf-app-router-1788760786-29887`；R2 cache **50** 对象已 `celld r2 bulk put` 导入
- **结果：** **55 passed / 3 failed / 3 skipped**（58 runnable → **约 94.8% runnable pass**）；harness `exit=1`（Playwright 非零）
- **相对 §4.1 改善：** `middleware.redirect` 已绿；主 ISR 用例已绿；失败从 5+flaky 降为 **3 项稳定 fail**
- **剩余 3 fail（仍 experimental，勿夸大）：**
  - `isr.test.ts` — Incremental Static Regeneration **with data cache**（revalidate 后 `originalFetchedDate` 与 `finalFetchedDate` 不一致，约 14s 级漂移）
  - `revalidateTag.test.ts` — **Revalidate tag**（`x-nextjs-cache` / `x-opennext-cache` 期望 HIT，收到 MISS）
  - `ssr.test.ts` — **Fetch cache properly cached**（`page.reload()` 后 cached fetch 时间戳未保持稳定）
- **归因方向（待插桩，非 R2 bulk 导入缺口）：** Next **data cache / tag cache**（OpenNext DO：`NEXT_TAG_CACHE_DO_SHARDED` 等）在 celld 上与 Cloudflare 参考行为仍有差距；incremental cache（R2 预填充）路径已证实接入成功。
- **本机前置：** `~/.local/bin/celld` 须指向子模块 `c28db43` 的 `target/lab/celld`（勿用陈旧 worktree 二进制），否则 `v13-r2-branch` 会在 delete→put 读回处失败。

`--fast` 仅复用 pnpm 依赖，**不跳过** clean build、OpenNext 转换、artifact、R2 cache 导入、deploy、Playwright。

---

## 5. 环境与运行方式

### 5.1 必读（按顺序）

1. 根目录 `DESIGN.md`
2. `docs/decisions.md`
3. `docs/test-plan.md`
4. `dev/AGENTS.md`、`dev/README.md`
5. 本文件 + `docs/plans/NEXT-OPENNEXT-CELLP.md`

### 5.2 本地栈（贡献者）

```bash
# 日常：复用 RustFS / celld / offshoot / platform
./dev/scripts/up.sh --fast && ./dev/scripts/health.sh --quick

# 若 :8792 celld 不健康而 platform/gateway/rustfs 仍 OK：
# 只对 RustFS 跑 diagnose，再按 dev 约定启动共享 celld（勿 pkill  broad pattern）
# 见 dev/scripts/up.sh / up-native.sh
```

- Gateway `:8787`，Platform `:8790`，共享 celld `:8792`，RustFS S3 `:19000`（以 `dev/.env.example` 为准）。
- **禁止**手改 `dev/data/` 内 sqlite/状态；重置用 `./dev/scripts/reset.sh`。
- **禁止**读取、修改、`source` 后泄露 `dev/.env`、`web/.env`、`~/.cellp/config.json` 等到文档或日志。E2E 脚本会 source `dev/.env`；接手人勿在交接材料里复制 token。

### 5.3 celld 二进制

改子模块后：

```bash
cd celld && cargo build -p celld --profile lab
# 确保 ~/.local/bin/celld 指向 target/lab/celld
```

当前验收相关子模块 commit：**`c28db43`**（根仓库 pointer 已在 `de39f3a` 更新）。

### 5.4 隔离 OpenNext 项目（API 可查，勿乱 promote）

| Fixture | 默认 project id | 说明 |
|---------|-----------------|------|
| app-router | `opennext-e2e-app-router` | 曾有 first-ready prod；见 §4.1 |
| pages-router | `opennext-e2e-pages-router` | 已有 prod baseline |
| app-pages-router | `opennext-e2e-app-pages-router` | 已有 prod baseline |

查询示例（使用本地已知 dev token，**勿写入公开文档**）：

```bash
curl -sS -H "Authorization: Bearer dev-local-token" \
  http://127.0.0.1:8790/v1/projects/opennext-e2e-app-router | jq '{id,prod_version_id}'
```

---

## 6. 推荐验证顺序（接手后）

```bash
# 1. 栈
./dev/scripts/health.sh --quick

# 2. celld 单测（R2）
cd celld && cargo test -p celld --lib r2_overlay::tests
cd celld && cargo test -p celld --lib r2_cli::tests
cd celld && cargo build -p celld --profile lab

# 3. 真实栈 R2 门禁
bash e2e/scripts/v13-r2-branch.sh

# 4. Harness 语法
bash -n dev/scripts/prepare-opennext-official-e2e.sh dev/scripts/run-opennext-official-e2e.sh
shellcheck dev/scripts/prepare-opennext-official-e2e.sh dev/scripts/run-opennext-official-e2e.sh  # 若已安装

# 5. OpenNext（先 app-router，再全量）
./dev/scripts/run-opennext-official-e2e.sh --only app-router --fast
./dev/scripts/run-opennext-official-e2e.sh --fast

# 6. 文档
cd site && npm run docs:build

# 7. 显式 full gate（发布前，非日常默认）
./e2e/scripts/run-all.sh
```

证据日志写入 `docs/evidence/opennext-official-*.log`（大 log 已 gitignore 规则覆盖时勿提交无必要产物）。

---

## 7. 关键文件地图

| 路径 | 作用 |
|------|------|
| `celld/crates/celld/js/r2_ops.rs` | Worker R2 写 + tombstone reveal |
| `celld/crates/celld/r2_cli.rs` | `celld r2 bulk put` / branch |
| `celld/crates/celld/r2_overlay.rs` | overlay / tombstone 语义 |
| `e2e/scripts/v13-r2-branch.sh` | 真实栈 R2 branch + bulk 读回 |
| `dev/scripts/prepare-opennext-official-e2e.sh` | clone pin、build、cache 导入、stage preview |
| `dev/scripts/run-opennext-official-e2e.sh` | 三 fixture 编排 |
| `dev/examples/support-opennext-official/prepare-artifact.sh` | OpenNext artifact → cellp bundle |
| `docs/plans/NEXT-OPENNEXT-CELLP.md` | 内部权威实验计划与归因表 |

---

## 8. 禁止事项（违反即交接失败）

- 引入 PostgreSQL / Caddy / Forgejo 作为 cellp 依赖；外部云 S3/R2 作平台存储。
- 跳过 `celld diagnose` 存储探针；无条件 `./e2e/scripts/run-all.sh` 作日常开发默认。
- `pkill` / `killall` / `pgrep -f` 等 broad 杀进程；重启健康的 Gateway/platform/RustFS 图方便。
- 修改冻结契约 `docs/plans/D1-*-RPC.md`（与本任务无关但全局有效）。
- 为让 Playwright 变绿而改上游 fixture、弱断言、或伪造 compatibility 文案。
- 提交凭据、无关 dirty、整库 `cargo fmt` 造成的 celld 噪音 diff。

---

## 9. 建议后续优先级

1. **P0：** ✅ App Router 已重跑（见 §4.3）。可选：三 fixture 全跑并更新 `NEXT-OPENNEXT-CELLP.md` 全表。
2. **P1：** 三 fixture 全跑；记录每个 fixture 的 pass/fail/skip 与 log 路径。
3. **P2：** 针对 §4.3 余下 3 fail：data cache、revalidateTag（DO）、SSR fetch cache — 按 `NEXT-OPENNEXT-CELLP.md` 归因表插桩，**禁止**未证实的 tier-1 结论。
4. **P3：** 可选：将 fail-closed 回归固化为 `dev/scripts/` 下独立小脚本（当前曾用 inline bash 验证，未单独入库）。

---

## 10. 联系人上下文

- 前序会话在 App Router 约 **70%**（修复与预验证完成，完整 58 runnable 重跑未完成）处按 owner 要求**暂停**并先提交。
-  owner 关心的口径：**兼容多少** = 最新一次 **真实 Playwright runtime** 的 runnable 通过数 / runnable 总数，且必须注明 experimental；report-only / `--collect-only` **不算** runtime 验收。

---

## 11. 快速 FAQ

**Q：现在能不能说 OpenNext 91% 兼容？**  
A：只能说固定 pin、**App Router**、**某次真实 Playwright run** 的 runnable 通过数 / runnable 总数，且必须注明 **experimental**。2026-09-07 修复后重跑为 **55/58 runnable（约 94.8%）**，日志 `opennext-official-e2e-20260907-135945-61847.log`；不得等同于三 fixture 合计或 tier-1。

**Q：`--fast` 会跳过测试吗？**  
A：不会跳过 Playwright；仅在有 node_modules 时跳过 workspace `pnpm install`。

**Q：为什么要 existing prod？**  
A：防止 preview 部署触发 first-ready 把测试 version 提成 prod，污染隔离项目。

**Q：R2 bulk 和 Dashboard 的关系？**  
A：Dashboard 仍无 R2 对象浏览器；bulk 是 CLI/运维导入，Worker 仍通过 binding 读写。

---

*交接文档版本：2026-09-06 · 对应根提交 `de39f3a` · celld `c28db43`*
