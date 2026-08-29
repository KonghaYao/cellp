# D1 import RPC 契约（冻结）

> 对抗审查 A1/A2/A9 · 2026-08-29  
> **禁止**把 SQLite 字节放进 JSON。

## CLI

```text
celld d1 import DATABASE --file PATH [PROJECT] --bucket NAME [--endpoint URL] [--region R]
```

- `DATABASE`：wrangler `d1_databases[].database_name`（一期只允许一个 binding）
- `--file`：本机绝对或相对路径，指向 **SQLite 二进制**（魔数 `SQLite format 3`）
- `execute --file` **只接受 SQL**；对 `.db` 必须失败并提示用 `import`

Exit：`0` 成功 · `1` 校验/SQL/IO · `2` 用法错误

## HTTP（owner `/runtime/<scope>`）

Request JSON **仅**：

```json
{ "import": { "path": "/abs/path/to/seed.db" } }
```

禁止字段：`bytes`、`base64`、`sql`（import 请求里）。

Success：

```json
{ "ok": { "bytes": 105005056, "duration_ms": 1200, "snapshot_txid": 1 } }
```

Failure：

```json
{ "error": { "family": "D1_IMPORT_ERROR", "message": "..." } }
```

## Owner 顺序（不得省略）

1. `storage::close(scope)`
2. 校验 seed：魔数、无 `*-wal`/`*-shm`、`PRAGMA page_size = 4096`
3. `replication::sqlite_snapshot(seed, db_path, None)`（rusqlite backup，禁止 `fs::copy`）
4. LTX `Db::reset_local_state()`
5. `db.sync()` 打 MinTXID==1 snapshot
6. 上传 bucket（`sync_cell` / `write_ltx_file`）
7. `storage::open_at_epoch(...)`

## Timeout

Import `Reachable` Subject timeout：**max(600s, size_mb * 2s)**。Execute 仍 120s。
