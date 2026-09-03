# Support 验证批次 Handoff（2026-09-03）

> **读者：** 下一任 agent / 维护者  
> **仓库：** cellp `main`（末批相关 commit：`8172b6a`、`5f67aae`；**工作区可能有未提交 doc/gateway 改动**）  
> **celld submodule：** `445569a`（wasm sibling + `no_bundle` ESM + `request_url` 路径 `//` 折叠）

---

## 1. 任务进度总览

| 轨道 | 状态 | 说明 |
|------|------|------|
| **AD-13 tier-1（S22–S25）** | **完成** | prod Host 用户验收 PASS（2026-09-03 sync verify） |
| **README 框架 S26** | **完成** | Hono **支持** · v3 |
| **S27 SolidStart** | **阻塞** | C3 非交互 / `create-solid` TTY |
| **S28 Qwik** | **阻塞** | deploy health timeout + `process.stdin` unenv |
| **S29 Waku** | **阻塞** | deploy health timeout + ESM `export named 'r'` |
| **S30 OpenNext** | **阻塞** | celld **ready**（PD-08 wasm fixed）· prod **308 `Location: ?`** · 矩阵 **不支持** |
| **P0 Agent（A01/A03/A04）** | **部分** | HTTP/WS 101 PASS；Workers AI / SSE / 完整 agent 回合未通 |
| **e2e `run-all.sh`** | **部分** | VE-2..5 PASS；**offshoot CLI 未装** → `v1-d1-seed.sh` FAIL |
| **go test** | **完成** | `cd cellp && go test ./...` PASS（sync verify） |
| **overnight goal 文档项** | **完成** | 见 `docs/support-overnight-goal-20260903.md` |

**证据索引：**

- 全量复验：`docs/evidence/verify-full-20260903.log`、`verify-restart-20260903.log`
- 用户行为表：`docs/support-framework-user-acceptance.md`（含 **S30** 节）
- 矩阵：`docs/support-matrix.md` §README 框架（S26–S30）
- 阻塞矩阵：`docs/plans/FRAMEWORK-README-EXT-ANALYSIS.md` §8–§10

---

## 2. Redirect 支持了吗？

**结论：分两层，不要混为一谈。**

### 2.1 平台 / Gateway / celld（一般 HTTP 重定向）

| 能力 | 状态 |
|------|------|
| Worker 返回 **3xx** + `Location` 透传给客户端 | **支持**（S22–S26、A 类等正常 302/308 由应用发出） |
| celld **静态 assets** `_redirects` / 规范化路径 **307** | **支持**（见 `celld/crates/celld/assets.rs`） |
| Gateway **X-Forwarded-Host/Proto** + celld **`CELLD_TRUST_FORWARDED_HEADERS=1`** | **已合入**（`cellp/internal/runtime/manager.go`）· 用于正确 `request.url` |

### 2.2 S30 OpenNext — **「可接受的 redirect / 首页」未支持**

| 项 | 现状 |
|----|------|
| **矩阵 verdict** | **不支持** |
| **prod `GET /`（Host `support-opennext.lvh.me`）** | **308 Permanent Redirect** |
| **`Location` / body** | **`?`**（畸形，非合法 URL） |
| **响应头** | `X-Opennext: 1`、`Refresh: 0;url=?` |
| **v10 artifact** | deploy + promote **ready** · 仍 **308**（非 200 HTML） |
| **已尝试、未解决** | 去掉 `global_fetch_strictly_public`；celld `collapse_request_path_and_query`；trust forwarded headers |

**含义：** OpenNext/Next 在 cellp ingress 下进入 **错误 308 链**（与 Next `handleRequestImpl` 重复斜杠 / OpenNext `normalizeLocationHeader` 等相关），**不是**「cellp 不支持 HTTP redirect」这么简单。

**未做 / 待验证：**

- 在 celld 打日志确认 **`request.url` / `req.url`** 实值（是否含 `//` 或全 URL 误入 pathname）
- OpenNext **`vars` / public URL**（`http://support-opennext.lvh.me:8787`）是否消除 308
- 单独 **`docs/plans/NEXT-OPENNEXT-S30-308.md`**（308 专篇，与 wasm 根因文档拆分）

**Redirect 产品口径：** **S30 不算支持**；修复标准 = prod **`GET /` → 200** + body 含 Next HTML（见 acceptance 文档）。

---

## 3. 已合入代码（交接重点）

### celld `445569a+`

- **`read_wasm_modules_from_dir`**：`no_bundle` 时 sibling `*.wasm`（PD-08 **fixed**）
- **`read_js_modules_from_dir`**：`no_bundle` 递归 `.js/.mjs/.cjs`（S29 多模块）
- **`collapse_request_path_and_query`**：入站 path 前导 `//` → `/`（防 OpenNext 重复斜杠 308）

### cellp `5f67aae` / `8172b6a`

- 启动 celld：**`CELLD_TRUST_FORWARDED_HEADERS=1`**
- **`Deploy`/`Diagnose`**：`context.WithoutCancel`（减 premature cancel）
- OpenNext prepare/overlay：**移除 `global_fetch_strictly_public`**
- dev：`CELLP_SKIP_CELLD_DIAGNOSE` 导出给 cellpd

### 部署约定

- npm 默认：**npmmirror**（`deploy-support-app.sh` / 调用前 `npm_config_registry=https://registry.npmmirror.com`）
- OpenNext：**勿**对已是 ready 的 version 盲目 `DESTROY_FIRST`；用 **新 version id**（如 v10）

---

## 4. 环境 / 运维坑（必读）

1. **Fleet 冷启动**：cellpd 重启后大量 route `celld_health=down`；**promote ≠ 60s 内可 curl**。验收前需 reconcile 或手动 `celld --listen` 对齐 registry 端口（sync verify 对 S22–S26 曾手动拉起）。
2. **内存 / SIGKILL**：~28 个 `celld` + orch 子进程 **`celld deploy` OpenNext ~8.6 MiB** 易被 kill；**同目录手工 `celld deploy` 可成功**。
3. **verification subagent**：部分配置 **禁止写 repo**；证据与 acceptance 更新需 **主 agent 或 coder** 落盘（本轮已写 `verify-*.log` + acceptance，**可能未 commit**）。
4. **Agent 调度**：长批次用 **同步** `Agent(verification)`（`run_in_background: false`），避免 thread active 无法 resume。
5. **e2e**：`go install github.com/sricola/offshoot/cmd/offshoot@latest` 后再跑 `./e2e/scripts/run-all.sh`。

---

## 5. 建议下一步（优先级）

| P | 动作 | 完成标准 |
|---|------|----------|
| **P0** | 修 **S30 308 `Location: ?`**（celld URL / OpenNext env / Next config） | prod 200 HTML · matrix S30 → **支持** |
| **P1** | **S28** Qwik：`process.stdin` unenv 或 compat flag | deploy ready + `grep Welcome to Qwik` |
| **P1** | **S29** Waku：ESM export / bundle 策略 | deploy ready + prod grep |
| **P2** | **S27** 非交互 SolidStart（rsync `templates/solid`） | deploy + prod grep |
| **P2** | 提交未 commit 的 **doc + gateway 工作区**（若有意保留） | clean `git status` |
| **P3** | offshoot + 全 e2e | `run-all.sh` green |

**复验命令模板：**

```bash
./dev/scripts/health.sh
SUPPORT_SKIP_GIT_FETCH=1 GITHUB_CLONE_DIRECT=1 npm_config_registry=https://registry.npmmirror.com \
  SUPPORT_VERSION=v10 SUPPORT_POLL_SECS=300 ./dev/scripts/deploy-support-app.sh S30
curl -sS -D - -o /tmp/s30.html -H 'Host: support-opennext.lvh.me' http://127.0.0.1:8787/ | head -15
grep -iE 'next|<!DOCTYPE' /tmp/s30.html | head -3
```

---

## 6. 相关文档地图

| 文档 | 用途 |
|------|------|
| [DESIGN.md](../../DESIGN.md) | 顶层设计 |
| [docs/decisions.md](../decisions.md) | AD-* |
| [support-matrix.md](../support-matrix.md) | 唯一 **支持/不支持**  verdict |
| [support-framework-user-acceptance.md](../support-framework-user-acceptance.md) | Prod 用户行为验收 |
| [NEXT-OPENNEXT-S30-ROOT-CAUSE.md](./NEXT-OPENNEXT-S30-ROOT-CAUSE.md) | wasm / health（**已 fixed**） |
| [NEXT-OPENNEXT-CELLP.md](./NEXT-OPENNEXT-CELLP.md) | OpenNext 集成路径 |
| [FRAMEWORK-README-EXT-ANALYSIS.md](./FRAMEWORK-README-EXT-ANALYSIS.md) | S27–S30 阻塞与 P0 清单 |
| [platform-defects-log.md](../platform-defects-log.md) | PD-20260903-08 wasm 等 |

---

## 7. Handoff 签收清单

- [ ] `~/.local/bin/celld` = submodule **lab** build ≥ **445569a**
- [ ] `dev/data/cellpd` = 最新 `go build ./cmd/cellpd`
- [ ] `./dev/scripts/health.sh` 全 OK
- [ ] 读过 `verify-full-20260903.log`
- [ ] 明确：**S30 redirect 未支持**；P0 = 200 HTML 而非「能 308」

*生成：主 agent · 2026-09-03 · 承接 sync verification 子 agent 结果*
