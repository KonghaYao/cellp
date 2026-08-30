# Runbook：生产回滚（Promote 切流）

> **前置：** [DESIGN.md](../../DESIGN.md) AD-5 promote saga · AD-9 archive/wake  
> **API：** [openapi.yaml](../../cellp/api/openapi.yaml)

cellp **没有** Vercel 式「一键 instant rollback」别名翻转。生产切流是 **原子 promote**；回滚 = 把**之前当过 prod 的 version** 再 promote 一次，或 wake 已 archive 的旧版。

---

## 1. 快速判断

| 现象 | 含义 |
|------|------|
| `/{project}/` 返回新 Worker body | 当前 `prod_version_id` 已指向新版本 |
| 旧 version 仍 `ready` | 可直接 **re-promote**（最快） |
| 旧 version `archived` | 先 **wake**，再 promote |
| 旧 version `destroyed` | **无法回滚** — 需从 artifact/S3 重新 deploy |

---

## 2. 标准回滚（旧 prod 仍为 ready）

```bash
PROJECT=my-app
OLD_VERSION=v-previous-prod   # 上次 promote 的 version ID
ADMIN_TOKEN=…
CELLP_URL=http://127.0.0.1:8790

curl -sS -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${CELLP_URL}/v1/projects/${PROJECT}/versions/${OLD_VERSION}/promote"
```

**验证：**

```bash
curl -sS "${GATEWAY}/${PROJECT}/" | head
# 或 GET /v1/projects/${PROJECT} 查看 prod_version_id
```

Promote saga：drain 旧 prod 路由 → CAS 更新 `prod_version_id` → 激活新 prod 路由。窗口目标 **≤2s**（见 TP-V4）。

---

## 3. 旧 prod 已 archived（503 on preview URL）

Archive 停止 celld 进程，**保留 S3/offshoot 数据**。Preview URL 返回 `503 version_archived`。

```bash
# 1. Wake
curl -sS -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${CELLP_URL}/v1/projects/${PROJECT}/versions/${OLD_VERSION}/wake"

# 2. Poll until ready
until [[ "$(curl -sS -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${CELLP_URL}/v1/projects/${PROJECT}/versions/${OLD_VERSION}" | jq -r .status)" == "ready" ]]; do
  sleep 2
done

# 3. Promote
curl -sS -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${CELLP_URL}/v1/projects/${PROJECT}/versions/${OLD_VERSION}/promote"
```

Dashboard：**Versions** → 选旧 version → **Wake** → **Promote to prod**。

---

## 4. 热留窗口（rollback keep）

| 环境变量 | 默认 | 含义 |
|----------|------|------|
| `CELLP_ROLLBACK_KEEP` | **60m** | promote 后旧 prod 在 ready 状态保留时长，便于快速 re-promote |
| `CELLP_ARCHIVE_IDLE` | **45m** | ready 且无访问 → 可被 reaper archive |
| `CELLP_ARCHIVE_GRACE` | **15m** | ready 后 grace 内不 archive |

**建议：** 生产 promote 后 **pin** 上一 prod version（`POST …/pin`），直到确认新版本稳定，再 unpin 或等待自然 archive。

---

## 5. 数据一致性

- **D1 / KV / R2 / Queue：** 回滚到旧 version = 回到该 version **独立 bucket** 上的数据快照，**不会**自动合并新版本写入。
- 若新版本已 promote 且写入 prod 数据，回滚 **不**撤销那些写入；仅切读写到旧 version 的数据面。
- 需要「撤销迁移」时：在旧 version 上跑补偿 SQL/脚本，或从 branch 派生修复 version。

---

## 6. 失败与升级路径

| 错误 | 处理 |
|------|------|
| promote 409 / saga failed | 查 cellpd 日志；Registry `routes` 无泄漏（TP-V5） |
| wake 后长时间非 ready | `GET …/versions/{id}` 的 `error` 字段；celld 日志 |
| 找不到旧 version ID | Registry / Dashboard Versions 列表；Git tag 与 `git_sha` 元数据 |

---

## 7. 相关

- [vercel-migration.md](../vercel-migration.md) · [cloudflare-migration.md](../cloudflare-migration.md)
- [observability.md](../observability.md) — 切流窗口监控 Gateway 5xx
