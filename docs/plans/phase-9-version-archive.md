# Phase 9 — Version 热备与封存（AD-9）

> **规格来源：** 对话 canvas「Version 热备与封存」  
> **状态：** 已落地（2026-08-30）  
> **硬性变更：** **取消每 project 5 个 ready version 硬上限**（删除 `ready_version_limit_exceeded` 429）。资源靠封存策略，不靠拒绝 CD。

## 状态

新 `status=archived`：celld 已 `Stop`、watch 已删、`route.active=false`、**S3 与 offshoot 保留**、**可当 branch 父**。  
`destroyed` 仍是 Destroy。`failed` 不是封存。

`CountReadyVersions` 只数 `ready`。Archived 不计。无 ready 数量上限。

## mayArchive

| 条件 | 自动封存 |
|------|----------|
| `id == prod_version_id` | **永不**（人工 POST 也 422） |
| pinned | 永不（先 unpin） |
| `now - ready_at < 15m`（`CELLP_ARCHIVE_GRACE`） | 禁止 |
| `id == previous_prod` 且距 promote `< 60m`（`CELLP_ROLLBACK_KEEP`） | 禁止 idle；无容量上限后也不因「挤槽」封存 |

## 触发器（无容量驱逐）

已去掉 5 槽上限，**不要**在 Start 前 LRU archive。触发只剩：

1. **Idle reaper**（默认 1min tick）：`ready` 且 mayArchive 且 `now-last_access ≥ 45m`（`CELLP_ARCHIVE_IDLE`）→ Archive  
2. **人工** `POST .../archive`（禁 prod；pin → 409）  
3. **Promote**：只更新 `prod_version_id` + 记录 `previous_prod_version_id`。**不立刻** Archive(oldProd)。过了 `CELLP_ROLLBACK_KEEP` 后由 idle 收走。

`last_access = max(ready_at, last Gateway 成功代理, last version-scoped API)`。503 不计。Gateway 写库每 version 最多 1/min。

## API

| 方法 | 作用 |
|------|------|
| `POST /v1/projects/{id}/versions/{vid}/archive` | Stop |
| `POST .../wake` | Start + S3 restore + health + route active + `ready` |
| `POST .../pin` · `.../unpin` | 钉住仍占进程，只防 idle |
| `DELETE` | Destroy；若仍有 `ready\|archived` 子 `parent_version_id` 指向 → 409 |

Gateway：`GET /{project}/{archived}/` → **503** `version_archived`（第一期 **不**同步 wake）。`GET /{project}/` prod 永远活。

## 实现落点

- `cellp/internal/registry`：status 常量、`previous_prod_version_id`、`pinned`、`last_access_at`  
- `cellp/internal/orch`：`Archive` / `Wake`；**删除** deploy 路径上的 `ready >= maxReady`  
- `cellp/internal/api/server.go`：删除 429 上限；新 handler  
- `cellp/internal/gateway`：inactive/archived 503 文案；节流更新 last_access  
- `cellp/internal/gc` 或 orch ticker：idle reaper  
- OpenAPI + 单测 + 改掉 `TestReadyVersionLimitExceeded429`（第 6 个 POST 必须 **不是** 429）  
- Dashboard：热/封存徽章、唤醒、pin（`web/` 仅 Vite SPA）

## 验收

- `cd cellp && go test ./...`  
- 6+ ready versions 可同时存在（无 429）  
- archive prod → 422  
- idle（测试里把 idle 调到秒级）后非 prod 变 archived，进程不在，S3 仍在  
- wake 后 preview 200  
- 从 archived 父 `d1 branch` 仍成功（父进程不必活）  
- promote 后 `/{project}/` 仍 200  
