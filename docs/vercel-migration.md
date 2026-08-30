# Vercel → cellp 迁移指南

> **定位：** cellp 是**私有化 Workers 平台控制面**，不是 Vercel 替代品。  
> **决策：** [decisions.md](./decisions.md) **AD-10**（不做 Git 托管 · 不做账号/RBAC）  
> **若你跑 Next.js SSR / Node serverless：** 见 [supported-stacks.md](./supported-stacks.md) — **非目标运行时**。

本文说明从 Vercel 心智模型迁移到 cellp 时，**预览/生产**、**环境变量**与**数据绑定**如何重新接线。

---

## 1. 部署流：Git 推送 → 外部 CI + cellp Version

### Vercel 今天

```
git push → Vercel 自动 build → Preview URL / Production alias
```

分支名、PR 号驱动预览；Production 常等于 `main` 分支。

### cellp 等价

cellp **不**监听 Git webhook，**不**托管仓库。Forgejo / GitHub + CI 负责：

```
build Workers bundle (wrangler)
  → upload artifact  s3://cellp-artifacts/{project}/{version}/
  → POST /v1/projects/{project}/versions
  → poll until status=ready
  → preview: {GATEWAY}/{project}/{version}/
  → promote: POST …/versions/{id}/promote  →  {GATEWAY}/{project}/
```

| Vercel | cellp |
|--------|-------|
| Preview Deployment | **Version**（显式 `id`，如 commit SHA 或 `pr-42`） |
| Production | `prod_version_id` + Gateway `/{project}/` |
| Git branch URL | **无** — 用 version ID，见 Dashboard **Versions** 页 |
| PR 自动预览 | CI 在 PR 打开时 `POST /versions` + `parent_version_id` |

示例 workflow：[dev/examples/ci-pr-preview.example.yml](../dev/examples/ci-pr-preview.example.yml)（与 CF 迁移共用模式）。

---

## 2. 环境变量

| Vercel | cellp |
|--------|-------|
| Project / Preview / Production 三套 env | **per-version** `env_json`（Settings 或 `PUT …/env`） |
| 加密 env（Dashboard） | 明文 `env_json`；生产 secret 由 **CI 注入** 或外层 Vault（见 [cloudflare-migration.md §4](./cloudflare-migration.md)） |
| Redeploy 才更新 env | ready version 上 `PUT …/env` 触发 Stop+Start（Worker env 热更新路径） |
| `VERCEL_*` 平台变量 | `PROJECT_ID` / `VERSION_ID` **只读**（平台注入） |

子 version 创建时**继承**父 version 的 env overrides（不含平台键）。

---

## 3. 数据与预览「血缘」

Vercel Postgres / KV 预览常与 production **共享**或手动复制。cellp 的卖点是 **App + Data 同版**：

| 资源 | 子 version（`parent_version_id`） |
|------|-------------------------------------|
| D1 | `celld d1 branch` |
| KV / R2 / Queue | **AD-8 branch**（继承父数据，写入隔离） |
| Workflow / Cron | **空起步**（不 branch） |
| Worker 脚本 | 来自**子** artifact |

Dashboard version 详情展示 **Parent version**、**D1 branch method**、**Binding branch** 行。

---

## 4. 运维对照

| 操作 | Vercel | cellp |
|------|--------|-------|
| 回滚生产 | Redeploy 旧 deployment / instant rollback | [runbooks/rollback.md](./runbooks/rollback.md)：wake archived → re-promote |
| 停预览省资源 | 无等价（SaaS 托管） | `POST …/archive` → 503；`POST …/wake` 恢复 |
| 长期 QA 预览 | 保持 deployment | **Pin** version（跳过 idle archive） |
| 日志 / Analytics | 内置请求日志 | [observability.md](./observability.md) — Prometheus + 文件日志；**无**请求 UI |
| 团队权限 | Org / RBAC | `DEPLOY_TOKEN` + `ADMIN_TOKEN`（AD-10 不做账号体系） |

---

## 5. 诚实缺口（不要期待 cellp 提供）

- **Next.js SSR / Edge Middleware / Node serverless** — 仅 Workers/wrangler 形态
- **Git 集成 UI** — 外部 Forgejo + CI
- **自定义域名 / TLS / WAF** — 外层 Caddy / nginx / Cloudflare（AD-10）
- **全球边缘 POP** — 分布式 cellpd 即可；非 CF 式边缘网络
- **Serverless Functions 冷启动 SLA** — archive/wake 显式模型（默认 idle 45m）

---

## 6. 相关文档

- [cloudflare-migration.md](./cloudflare-migration.md) — wrangler / binding 细节（与 Vercel Workers 用户重叠）
- [DESIGN.md](../DESIGN.md) · [test-plan.md](./test-plan.md)
- [decisions.md §15 AD-10](./decisions.md#15-ad-10--产品边界权威否定与核心范畴)
