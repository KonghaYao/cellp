# 社区项目验证 — 踩坑记录（ingress / 部署 / 工具）

> 2026-09 批次 3–4（S17 r2filebox、S05 FlareMo、S08 EdgeEver 等）总结。  
> **当前 dev 推荐 ingress：** `lvh.me`（见 `dev/INGRESS-HOST.md`）。

---

## 1. Ingress / URL（最高频）

| 错误 | 现象 | 正确做法 |
|------|------|----------|
| 用过期 **nip**（如 `192-168-12-36.nip.io`）而本机 IP 已变 | 浏览器打不开 / 连错机器 | 用 **`lvh.me`**（`*.lvh.me`→127.0.0.1，与 LAN IP 无关）或换 IP 后重跑 magic |
| 地址栏只写 **`http://127.0.0.1:8787/`** | **404** | AD-12 按 **Host** 选路；URL 必须是 `http://{project}.lvh.me:8787/` |
| 省略 **`:8787`** | 连到 80，白屏 | dev Gateway 在 **8787**（HTTP） |
| 换 `CELLP_INGRESS_BASE_DOMAIN` 后未 **promote** | 新 Host **404**，旧 Host 偶发仍通 | `./dev/scripts/restart-cellpd.sh` + `ingress-repromote-support.sh` |
| **Clash** 未直连 `lvh.me` | 浏览器 **502**，curl 200 | `dev/clash/cellp-verge-*.yaml` 合并 `lvh.me` 规则 |
| Worker 用 `new URL(c.req.url).origin` 拼分享链 | **`synthetic.*` 域名**、502 | `publicOrigin()` + `X-Forwarded-Host` / `PUBLIC_BASE_URL`（S17 patch） |
| wrangler **`run_worker_first` 含 `"/api/*"`** | `parse wrangler: unexpected end` | 用 `"/api/"` 等**无 `/*`** 前缀，或去掉（cellp `stripJSONC` 把 `/*` 当注释） |
| `PUBLIC_BASE_URL` / vars 里写 **`http://`** | D1 seed / deploy **parse 失败** | 注入时用 `\u002f` 或省略该 var |

---

## 2. 部署 / 运行时

| 错误 | 现象 | 正确做法 |
|------|------|----------|
| artifact 带全量 **pnpm node_modules** | celld `/src` 缺 symlink、RustFS 上传失败 | **wrangler dry-run** + `no_bundle` + slim stage（FlareMo / r2filebox） |
| 堆积多个 **celld** 进程 | **diagnose/deploy SIGKILL**（内存） | `pkill -f 'celld --bucket'` 后重 deploy；`CELLP_SKIP_CELLD_DIAGNOSE=1` 仅 dev 救急 |
| 对 **destroyed** version id 再 `SUPPORT_DESTROY_FIRST` | create 失败 / poll destroyed | 换 **新 vN** |
| Agent **单次 Bash** 跑 `bun install` + 全量 deploy | 超时 / 中断 | 本机终端跑；脚本支持 `SUPPORT_SKIP_BUILD=1` |
| **subagent** 跑长 shell | 同样卡住 | 主管只改脚本/文档，构建交给用户终端 |

---

## 3. 产品与沟通

| 错误 | 说明 |
|------|------|
| 未确认域名就假定用户在测 **FlareMo** | 「分片校验失败」文案来自 **r2filebox** |
| 用户要 nip 稳定，却先切 **ingress.local** | 应用 **lvh.me** 作为本机稳定 magic 替代 |
| Dashboard / 文档仍主推 **nip + hosts 多模式** | 收敛为 **lvh.me + Clash 一条直连** |

---

## 4. 仍开放（cellp 侧，非社区 patch）

- `stripJSONC` 误伤 `http://` 与 `/*` 路径 → 应换正规 JSONC 解析或部署前 strip 仅真注释
- Preview Host 换 base 后偶发 **404**（仅 prod promote 更新）→ orchestrator 应对齐 preview binding
- dev **HTTPS :8788** 与 `lvh.me` 证书 SAN 需 mkcert / `CELLP_TLS_EXTRA_SAN`

---

## 5. FlareMo 登录「无法读取认证状态」（500）

| 原因 | 修复 |
|------|------|
| 无 D1 表 | `dev/examples/support-flaremo/seed.sh` + deploy 写入 `seed.db` |
| 缺 `BETTER_AUTH_SECRET` / `FLAREMO_BOOTSTRAP_SECRET` | `wrangler.cellp.jsonc` |
| `FLAREMO_PUBLIC_URL` 中 `//` 与 stripJSONC | `\u002f` 转义 |
| `http://*.lvh.me` 非「本地」 | `patch-local-dev-hosts.sh` / `patch-bundle-local-hosts.sh` |
| celld health down / 旧 prod | `promote` 后 `restart-cellpd.sh` |

Dev：**v6** · `http://support-flaremo.lvh.me:8787/` · `setup_available: true` 时走首次初始化。

---

## 6. FlareMo 初始化页「需要管理员恢复」（`recovery_required`）

| 原因 | 说明 |
|------|------|
| 首次 bootstrap **中途失败** | `claimOwnerBootstrap` 已写入 `auth_bootstrap`，但 `completeOwnerBootstrap` 未成功 → `state=recovery_required`，`setup_available=false` |
| 仅有 bootstrap 行、**无** `auth_users` | 常见于 dev 反复试填表单；UI 不会接受 bootstrap secret |
| **bootstrap / sign-in 500**（`Initial setup could not finish`） | **不是 Drizzle**：Better Auth 用 `node:crypto.scrypt`；旧 celld 未实现 → `signUpEmail` 写库前失败。需 **新 celld**（`crypto.rs` + `node_crypto.js`），`pkill -f 'celld --bucket'` 后 `restart-cellpd.sh` |

| 修复（dev） | |
|------|------|
| 重新走 **/setup**（含已完成但忘了账号） | `./dev/scripts/reset-support-flaremo-bootstrap.sh [vN]` — 清空 auth_*、`users`、`auth_bootstrap`，回到 `ready` |
| 已有 1 个 auth 用户且需保留身份只改密 | 配置 `FLAREMO_RECOVERY_SECRET` 后 `POST /api/auth/flaremo/recover` / `recover-bootstrap` |

**初始化密钥（`state=ready` 时）：** `FLAREMO_BOOTSTRAP_SECRET` = `cellp-dev-flaremo-bootstrap-secret-32c`（请求头 `x-flaremo-bootstrap-secret`）。
