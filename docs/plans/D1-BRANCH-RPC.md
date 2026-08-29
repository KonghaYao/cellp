# D1 branch RPC 契约（冻结）

> 对抗审查 B5 · 2026-08-29  
> **禁止**把 SQLite 字节放进 JSON。根 version 仍走 [D1-IMPORT-RPC.md](./D1-IMPORT-RPC.md)。

## CLI

```text
celld d1 branch DATABASE --parent-bucket URI [PROJECT] --bucket CHILD_URI
```

- `DATABASE`：wrangler 唯一 `d1_databases[].database_name`
- `--parent-bucket`：`s3://cellp-celld/{project}/{parentVersion}`（见校验）
- `--bucket`：子 version bucket
- `--help` / `-h` **不得**要求 DATABASE 位置参数

Exit：`0` 成功 · `1` 校验/复制/IO · `2` 用法错误

## `parent_bucket` 校验（CLI 与 owner 共用）

`validate_parent_bucket(uri, project_id)`：

- 必须 `s3://cellp-celld/{project_id}/{parentVersion}`
- `parentVersion` 非空、无 `..`、无额外 `s3://`
- **拒绝** `http://`、`https://`、其它 bucket、其它 project

## HTTP（子 owner `/runtime/<scope>`）

Request JSON **仅**：

```json
{
  "branch": {
    "parent_bucket": "s3://cellp-celld/demo-app/v-parent",
    "parent_epoch": 1
  }
}
```

禁止字段：`bytes`、`base64`、`sql`、`fork_txid`、`path`。

`parent_epoch` 若省略或 0：owner 对父 bucket 跑 `highest_nonempty_epoch(parent_cell)`，**禁止**默认盲写 1。

Success：

```json
{
  "ok": {
    "fork_txid": 42,
    "parent_bucket": "s3://cellp-celld/demo-app/v-parent",
    "bytes_parent": 105005056,
    "duration_ms": 800
  }
}
```

Failure：

```json
{ "error": { "family": "D1_BRANCH_ERROR", "message": "..." } }
```

## Owner 顺序（不得省略）

1. isolate 线程 `storage::close(scope)`
2. `validate_parent_bucket`；`parent_cell` 必须等于本 D1 scope（同一 `database_id`）
3. 父前缀若已有 `base.json` → fail（一层 fork）
4. 父 `highest_nonempty_epoch` + restore 能力检查；不能 restore → **不要**写 `base.json`
5. 子前缀 `put_cas(base.json, None)` create-only
6. 合成 restore → 本地 `db.sqlite`（禁止 `fs::copy`）
7. `Db::seed_l0_baseline`（父终端 LTX bytes @ `fork_txid`），再 `open_at_epoch`
8. **禁止** `import_sqlite_seed` / `reset_local_state` / MinTXID==1 snapshot

已有 `base.json` 的前缀上 **`d1 import` 必须拒绝**。

## Timeout

Branch `Reachable` Subject timeout：**max(600s, parent_snapshot_mb * 2s)**。
