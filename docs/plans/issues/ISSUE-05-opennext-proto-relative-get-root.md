# ISSUE-05: S30 OpenNext prod `GET /` — protocol-relative URL `//` → 400

**优先级:** P1 · **类型:** 兼容性 / S30 验收 · **状态:** ✅ 2026-09-04 已闭环（prod v55）
**负责人:** _待认领_  
**关联:** S30 · [S30-OPENNEXT-HARD-PROBLEM.md](../S30-OPENNEXT-HARD-PROBLEM.md) · [NEXT-OPENNEXT-CELLP.md](../NEXT-OPENNEXT-CELLP.md) · PD-20260904-10（**已关**：`process.setImmediate` hang，**与本 issue 无关**）

## 背景（接手必读）

| 项 | 状态 |
|----|------|
| OpenNext 构建 / deploy / `ready` | ✅ |
| celld ingress `pathname === "//"` 308 环 | ✅（`celld/main.rs` `ensure_absolute_url`） |
| SSR **0-byte hang**（curl 28） | ✅ **2026-09-04**（celld `__celldPatchProcessTimers`，`4b3a3bf`） |
| **prod `GET /` 200 HTML** | ✅ **2026-09-04**（v55 · `Create Next App`，本 issue 已闭环） |

初始调查时 registry **prod 指针：`support-opennext` → v22**（旧 artifact，未含最新 bundle 补丁组合），因此 v22 稳定复现 400。闭环后的当前 prod 指针为 **v55**，见下方验收证据。

## 问题

经 Gateway（Host 路由）访问 OpenNext prod 首页：

```bash
curl -sS -m 10 -H 'Host: support-opennext.lvh.me' 'http://127.0.0.1:8787/'
```

**实际：** HTTP **400**，body 约 54B：

```text
"url" parameter cannot be a protocol-relative URL (//)
```

响应头含 `X-Opennext: 1`（OpenNext Worker 已执行，**非** Gateway 502/超时）。

**对照（应继续通过）：**

```bash
curl -sS -m 5 -o /dev/null -w '%{http_code}\n' \
  -H 'Host: support-opennext.lvh.me' \
  'http://127.0.0.1:8787/_next/static/chunks/8152fee336e967d5.css'
# 期望 200
```

## 根因假设（文档与代码线索）

1. **Next ImageOptimizer / `validateParams`**（或同类）在 bundle 内仍对 **以 `//` 开头的 url** 走硬编码 400；错误串与 Next 一致，**不是 celld 生成的 HTTP 状态**。
2. **`prepare-artifact.sh`** 已对 `.open-next` / worker bundle 做 **proto-rel → pathname** 补丁（`__cellpProtoRel`、`__cellpImageUrl` 等）；**v22 artifact 可能未应用或补丁未覆盖当前 Next 16 代码路径**。
3. 与 **ingress 把 `request.url` 编成带 `//` pathname** 的组合需再核对（celld 已修 308，但 **400 文案来自 bundle 内校验**）。
4. 环境变量 **`PUBLIC_BASE_URL` / `DEPLOY_URL`**（deploy 时注入 `http://support-opennext.lvh.me:8787/`）参与 `new URL(url, base)` 归一化；错 base 可能导致仍传入 `//` 形态。

**不要与本 issue 混淆：** preview v38–v42 的 **0-byte hang** 已由 PD-20260904-10 处理；新 deploy 若再 hang，先确认 celld 二进制为 **lab 构建** 且进程已 **kill/wake**。

## 验收标准

- [x] **prod**（promote 后的 version）`GET /` Host `support-opennext.lvh.me` → **200**，body 含可辨认 Next HTML（`<title>Create Next App</title>`）
- [x] 同上 **&lt; 5s** 完成（v55 实测 **0.201s**，无 curl 28）
- [x] `/_next/static/chunks/8152fee336e967d5.css` → **200**（24,703 B）
- [x] 使用既有已构建 v50 artifact 复制为全新 v55，经 `sync_artifact_to_rustfs` → version **ready**；preview 通过后才 **promote**，prod 指针为 v55
- [x] 更新 [support-framework-user-acceptance.md](../../support-framework-user-acceptance.md) §S30 与 [support-matrix.md](../../support-matrix.md)（仍保持「不支持」，未修改 AD-13）
- [x] 修复在 **celld**：`url` / `node:url` lazy builtin + V8 module-evaluation 回归测试

## 闭环（2026-09-04）

### 根因

最新 artifact 已消除最初的 proto-relative 400，但 celld 未实现 Node builtin `url` / `node:url`。OpenNext bundle 的默认导入因此解析为通用 callable `__nodeStub` Proxy：

```text
_url.parse("/")
  → Proxy
  → parsedUrl.pathname 仍是 Proxy
  → pathname.startsWith("/_next/image") 返回 truthy Proxy
  → Next 把 GET / 误判为 image request
  → normalizeAndAttachMetadata 提前结束
  → 正常 page matcher 不运行
  → /_not-found/page（HTTP 404）
```

v51–v54 的单请求诊断逐层确认：OpenNext 外层静态路由命中 `/`，Next 只加载 `/_not-found/page`，catchall matcher 未进入，最终定位到 `handleNextImageRequest` 对 `/` 返回 `true`。该链路与已关闭的 `setImmediate` hang 无关。

### 修复

- `celld/crates/celld/js/node_url.js`：提供 bundle 实际使用的 `parse`、`format`、`pathToFileURL`、`fileURLToPath`，并复用 prelude 的 WHATWG `URL` / `URLSearchParams`。
- `celld/crates/celld/js/modules.rs`：以 lazy module 注册 bare `url` 和 `node:url`，不再回落到 `__nodeStub`。
- `celld/crates/celld/lib.rs`：真实 V8 module evaluation 同时覆盖 bare default import 与 `node:url` named imports；断言 `parse("/").pathname === "/"` 且 image 前缀判断为 `false`。

### 验收证据

| 项 | 结果 |
|----|------|
| `cargo test -p celld` | **PASS** · 67 tests |
| `cargo build -p celld --profile lab` | **PASS** |
| `cd cellp && go test ./...` | **PASS** |
| v55 preview `GET /` | **200** · 0.082s · 12,301 B · `<title>Create Next App</title>` |
| v55 preview CSS | **200** · 24,703 B |
| promote | 客户端 5s 上限先超时；只读复核确认 saga 已完成，prod 指针为 **v55**，未重试 |
| v55 prod `GET /` | **200** · 0.201s · 12,301 B · `Content-Type: text/html` · `X-Opennext: 1` · `<title>Create Next App</title>` |
| v55 prod CSS | **200** · 24,703 B |
| `./dev/scripts/health.sh` | **PASS** · 全项 OK |
| 8 MiB D1 branch scale | **PASS** · `branch_ms=514` |
| D1 seed / D1 branch / `run-all.sh` | **BLOCKED** · 本机缺 offshoot CLI；`run-all.sh` 在此之前 health、CD、promote、补偿、destroy 均 PASS |

S30 的该生产根页 issue 已关闭，但 AD-13 未变：Next/OpenNext 仍是实验路径，不是 tier 1。

## 建议调查顺序（历史）

1. 读 [S30-OPENNEXT-HARD-PROBLEM.md](../S30-OPENNEXT-HARD-PROBLEM.md) §1–§4，勿重复已排除项。
2. 对 **当前 prod bundle**（`dev/data/artifacts/support-opennext/v22/.cellp-bundle/index.js` 或 S3）`grep`  
   `protocol-relative URL` / `__cellpProtoRel` / `validateParams`。
3. 本地 `prepare-artifact.sh` 全量重建 → preview Host `{version}.support-opennext.lvh.me` 测 `GET /` **再** promote。
4. 若 url 来自 `request.url`：对照 `celld` loopback / `main.rs` `ingress_path_and_query` 与 OpenNext `convert` 路径（`harness.js` OpenNext SSR re-fetch 注释）。
5. 证据写入 `docs/evidence/`（gitignore，本地即可）；结论回写本 issue 或 S30 实录。

## 非目标

- 把 S30 标成 support-matrix **「支持」**（AD-13）
- Dashboard / Next 一等公民
- 重开 PD-20260904-10（hang 已关）

## 环境

```bash
./dev/scripts/up.sh && ./dev/scripts/health.sh
cd celld && cargo build -p celld --profile lab   # ~/.local/bin/celld → target/lab/celld
# 换二进制后：kill 旧 support-opennext celld 或 POST .../wake
source dev/.env
curl -sf -H "Authorization: Bearer ${CELLP_ADMIN_TOKEN:-dev-local-token}" \
  http://127.0.0.1:8790/v1/projects/support-opennext | jq '.prod_version_id'
```

**Ingress：** [dev/INGRESS-HOST.md](../../../dev/INGRESS-HOST.md) — 必须 `Host: support-opennext.lvh.me`，URL `http://127.0.0.1:8787/`（带 **:8787**）。
