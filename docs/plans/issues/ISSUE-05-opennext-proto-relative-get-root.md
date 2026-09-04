# ISSUE-05: S30 OpenNext prod `GET /` — protocol-relative URL `//` → 400

**优先级:** P1 · **类型:** 兼容性 / S30 验收  
**负责人:** _待认领_  
**关联:** S30 · [S30-OPENNEXT-HARD-PROBLEM.md](../S30-OPENNEXT-HARD-PROBLEM.md) · [NEXT-OPENNEXT-CELLP.md](../NEXT-OPENNEXT-CELLP.md) · PD-20260904-10（**已关**：`process.setImmediate` hang，**与本 issue 无关**）

## 背景（接手必读）

| 项 | 状态 |
|----|------|
| OpenNext 构建 / deploy / `ready` | ✅ |
| celld ingress `pathname === "//"` 308 环 | ✅（`celld/main.rs` `ensure_absolute_url`） |
| SSR **0-byte hang**（curl 28） | ✅ **2026-09-04**（celld `__celldPatchProcessTimers`，`4b3a3bf`） |
| **prod `GET /` 200 HTML** | ❌ **本 issue** |

当前 registry **prod 指针：`support-opennext` → v22**（旧 artifact，未含最新 bundle 补丁组合）。即使 hang 已修，**v22 仍稳定复现 400**。

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

- [ ] **prod**（promote 后的 version）`GET /` Host `support-opennext.lvh.me` → **200**，body 含可辨认 Next HTML（非纯 JSON 400）
- [ ] 同上 **&lt; 5s** 完成（无 curl 28）
- [ ] `/_next/static/...` 至少一项 **200**
- [ ] `GITHUB_CLONE_DIRECT=1 ./dev/scripts/deploy-support-app.sh S30`（或 `SUPPORT_SKIP_BUILD=1` + 已有 artifact）→ 新 version **ready** + **promote** 后复验
- [ ] 更新 [support-framework-user-acceptance.md](../../support-framework-user-acceptance.md) §S30 与 [support-matrix.md](../../support-matrix.md)（**仍保持「不支持」除非 PM 改 AD-13**）
- [ ] 若修复在 **celld**：补回归测试；若在 **prepare-artifact**：文档说明补丁命中条件，避免 app 内永久 fork

## 建议调查顺序

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
