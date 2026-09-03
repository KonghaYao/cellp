# Cloudflare Workers → cellp 迁移指南

> **定位：** cellp 是**私有化 Workers 平台控制面**，不是自托管 Cloudflare。  
> **决策：** [decisions.md](./decisions.md) **AD-8**（binding branch）· **AD-10**（产品边界）  
> **运行时兼容：** [celld/docs/cloudflare-compat.md](../celld/docs/cloudflare-compat.md)

本文说明从 Cloudflare Workers 开发流迁移到 cellp 时，**概念映射**、**CI 接线**与**诚实缺口**。

---

## 1. 部署流：wrangler deploy → 外部 CI + cellp API

### Cloudflare 今天

```bash
wrangler deploy          # 直连 Cloudflare API，绑定自动落在账号下
wrangler dev             # 本地 + 远程绑定
```

### cellp 等价

cellp **不**接受 `wrangler deploy` 直连。外部 CI（GitHub Actions、Forgejo、GitLab 等）负责构建与上传，再调用 cellp：

```
build wrangler bundle
  → upload artifact to RustFS  s3://cellp-artifacts/{project}/{version}/
  → POST /v1/projects/{project}/versions
  → poll GET …/versions/{id} until status=ready
  → preview_url: `http://{version}.{project}.{baseDomain}:8787/` (AD-12 Host; see dev/INGRESS-HOST.md)
```

**`POST /versions` body（摘要）：**

| 字段 | 用途 |
|------|------|
| `id` | version 标识（CI 生成，如 commit SHA 或 PR 号） |
| `parent_version_id` | 可选；非空则触发 **D1 + KV + R2 + Queue branch**（AD-8） |
| `git_ref` / `git_sha` | 元数据标签；**不**驱动路由或自动 promote |
| `artifact_digest` | 可选；服务端校验 `s3://cellp-artifacts/{project}/{version}/` |
| `env` | 可选 per-version Worker 环境变量覆盖 |

鉴权：`DEPLOY_TOKEN`（仅 `POST /versions`）。其余 API 与 Dashboard 用 `ADMIN_TOKEN`。

**Promote 到生产：** `POST …/versions/{id}/promote`（saga 切流），非 wrangler 的 production branch。

完整 API：[openapi.yaml](../cellp/api/openapi.yaml) · [DESIGN.md §6](../DESIGN.md)

---

## 2. Version 模型 vs Cloudflare preview branches

| Cloudflare | cellp |
|------------|-------|
| Worker 脚本 + 绑定挂在**账号**下 | 每个 **Version** = 独立 celld 进程 + 独立 bucket（AD-1） |
| Preview URL / 环境分支（Pages、Workers preview） | Preview Host `{version}.{project}.{base}`；Prod Host `{project}.{base}`（**废弃** path `/{project}/{version}/`） |
| D1 **database_id** 跨环境共享或手动复制 | 根 version：`celld d1 import`；子 version：`celld d1 branch`（D1 契约） |
| KV / R2 / Queue 通常共享或手动 | 子 version **自动 branch** 父数据（AD-8） |
| `wrangler.toml` / `wrangler.jsonc` 由 Wrangler CLI 消费 | bundle 内 wrangler 文件由 **celld deploy** 解析；cellp **不**管理 wrangler.toml 生命周期 |

**`parent_version_id`：** 表示「从父 version fork App + Data」。典型 PR preview：`parent_version_id` 指向 seed 或 staging version，**不要**指向当前 prod（会 422 或 scrubbed seed，见 test-plan TP-V*）。

**`archived`：** 停进程、保留 S3；仍可作为 branch 父（AD-9）。无 Cloudflare 式「休眠 Worker 仍秒开」；需显式 `POST …/wake`。

---

## 3. Binding 映射与 branch 行为

cellp 沿用 celld 0.4.0 wrangler 绑定；控制面只解析清单并包装已有 CLI（AD-6）。

| Binding | 根 version（无 parent） | 子 version（有 `parent_version_id`） | Dashboard / operator |
|---------|-------------------------|----------------------------------------|----------------------|
| **D1** | `d1 import`（根）或空 | `d1 branch` 或 offshoot export fallback | D1 browser · SQL · branches |
| **KV** | 空 namespace | **branch** 父 KV（链式读父 blob） | KV 浏览器 |
| **R2** | 空 bucket 前缀 | **branch** overlay（读父、写子） | 清单可见；**无**对象浏览器 |
| **Queue** | 空 queue | **branch** 父队列 | Queue operator |
| **Workflow** | 空实例 | **不 branch** | 只读实例列表 |
| **Cron** | wrangler 声明 | **不 branch**（脚本随 deploy；触发由 celld） | 只读展示 |
| **Worker 脚本** | 来自 artifact | 来自**子** artifact；**不**继承父脚本 diff | — |

身份（`database_id`、`kv_namespaces[].id`、queue 名、R2 bucket 名）在 branch 时**继承父 wrangler**，与 D1 同构（phase-8-binding-branch）。

**不做：** UI/API 上的「从 prod 同步 KV」「inherit bindings」按钮——branch 在 orchestrator Start 阶段自动完成。

---

## 4. 诚实缺口（相对 Cloudflare 默认体验）

下列为**刻意不做**或**运行时未实现**，不是「下个 sprint 补上」的隐含承诺（AD-10）。

| 能力 | cellp / celld 现状 |
|------|-------------------|
| **`wrangler deploy` / `wrangler dev` 直连** | 无；用外部 CI + `POST /versions` + Gateway URL |
| **`wrangler tail` / 实时日志** | 一期：进程 stdout；**AD-14** 门面 `logs/stream` + OTLP **架构冻结、未实现**（[OTEL-OBSERVABILITY.md](./plans/OTEL-OBSERVABILITY.md)）；**不做** CF Workers Logs 产品 |
| **Workers AI** | celld：**No**（见 [cloudflare-compat.md](../celld/docs/cloudflare-compat.md)）；实验性 `CELLD_AI_URL` 适配器非 CF 托管 AI |
| **Vectorize · Hyperdrive · Browser Rendering · Email Workers · Python Workers** | celld：**No** |
| **全球边缘 PoP / Anycast** | 不做；私有化分布式多节点即可（AD-10 §15.2） |
| **内置 DNS / CDN / TLS / WAF** | 不做；外层 Nginx / LB / 自有网关终止 TLS（AD-10 §15.3） |
| **Git 托管 · Webhook · PR 集成** | 不做；Git 平台在外部，cellp 只收 HTTP API（AD-10 §15.4） |
| **账号 · Org · RBAC · SSO** | 不做；仅 `DEPLOY_TOKEN` + `ADMIN_TOKEN`（AD-10 §15.1） |
| **R2 对象浏览器** | 无 `celld r2` operator → Dashboard 仅徽章 |
| **Workflow 控制** | 无 pause/resume/restart；只读 `cell list` |

评估 Worker 是否能在 celld 上跑：**以 [celld cloudflare-compat](../celld/docs/cloudflare-compat.md) 为准**，不要假设 CF 账号下可用的 binding 在 cellp 上默认可用。

---

## 5. 推荐迁移步骤

1. **确认 binding 在 celld compat 表中为 Yes/Partial** — 尤其 D1、KV、Queues、R2、Workflow、Cron。
2. **本地验证：** `./dev/scripts/up.sh` → `celld deploy` + `celld d1` / `kv` / `queue` 对 dev bucket。
3. **CI 模板：** build → `aws s3 sync`（或等价）到 `s3://cellp-artifacts/{project}/{version}/` → `curl -X POST …/versions`。
4. **Preview：** 带 `parent_version_id` 部署子 version，检查 Dashboard **Lineage**（parent 链接 · D1 branch method · binding branch）。
5. **Prod：** promote 目标 version；Gateway `/{project}/` 切流。
6. **外层：** 自行配置 DNS → LB → cellp Gateway（TLS 在 LB 终止）。

---

## 6. 相关文档

| 文档 | 内容 |
|------|------|
| [DESIGN.md](../DESIGN.md) | 架构 · CD 流 · 存储布局 |
| [decisions.md](./decisions.md) | AD-8 binding branch · AD-10 边界 |
| [plans/D1-IMPORT-RPC.md](./plans/D1-IMPORT-RPC.md) | 根 version D1 import |
| [plans/D1-BRANCH-RPC.md](./plans/D1-BRANCH-RPC.md) | 子 version D1 branch |
| [plans/phase-8-binding-branch.md](./plans/phase-8-binding-branch.md) | KV / R2 / Queue branch |
| [test-plan.md](./test-plan.md) | TP-V* / TP-UI-* 验收 |

---

*2026-08-30 · 与 AD-8 / AD-10 对齐*
