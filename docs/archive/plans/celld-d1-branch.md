# celld D1 跨 version 共享基线 — LTX base pointer

> **代号：** D1-BRANCH  
> **作者：** Grok（plan）  
> **状态：** 已完成 T1–T5 · 证据见 `docs/evidence/d1-branch-e2e-report.md` · `d1-branch-scale-report.md` · `d1-branch-multi-100mb.json`  
> **对抗：** [REVIEW-celld-d1-branch.md](./REVIEW-celld-d1-branch.md) · 契约 [D1-BRANCH-RPC.md](./D1-BRANCH-RPC.md)  
> **日期：** 2026-08-29  
> **前置：** D1-IMPORT G1–G6 已在当前树证明（`docs/evidence/d1-import-scale-report.md`）  
> **约束：** 不做 PG、不做多租户、不改 AD-1（每 ready version 一个 celld 进程）。默认 Worker 包仍是 `dev/examples/counter`。`D1Execute` 在 wrangler 0 个 `d1_databases` 时仍跳过。冻结的 import RPC 仍是 path-only，本计划**另开** `branch` RPC，禁止把 SQLite 字节放进 JSON。

---

## 0. 用户可见问题

offshoot 在 **store 对象层**可以：100 MB 基线只存一份；branch A、B 再各写 50 MB 时，对象大约 **200 MB**，不是 300 MB 三份整库。

celld **今天没有这个能力**。每个 ready version 是：

1. 独立进程（AD-1）
2. 独立 bucket `s3://cellp-celld/{project}/{version}`
3. D1 cell 独立 LTX 前缀 `cells/<scope>/ltx/e<epoch>/`
4. import / 首次 restore 要求 **MinTXID==1 的全量 snapshot**

所以 cellp 部署子 version 时仍是：offshoot export 整份 `seed.db` → `celld d1 import` → 子 bucket 再存一份 ~100 MB LTX。G3 已测过 restore 对象约 **105 750 979 B**。

用户要的基础能力：

> 一个 100 MB 库分成 A、B 两个 branch，各自再记录 50 MB 时，**对象存储里的 D1 LTX 接近 200 MB**，而不是 A、B 各一份 150 MB 全量快照。Worker `env.DB` 在 A 上看不到 B 的写入。

本地 watch 里每个进程仍会物化一份完整 `db.sqlite`（和 offshoot checkout 一样）。本计划优化的是 **bucket 里的对象**，不是两个 live 进程的工作副本。

---

## 1. 源码事实（当前树，必须对照，禁止凭记忆）

### 1.1 D1 = 一个 cell 的 SQLite + LTX

- 类名 `__D1Database`；scope = `d1_cell_scope(database_identity)`（`celld/crates/celld/js.rs`）
- `database_identity` = wrangler `database_id` 或 fallback `database_name`（`deploy.rs`）
- 本地：`<watch>/<cell>/ltx/e<epoch>/db.sqlite`（`ltx_repl.rs` `db_path`）
- 远端：bucket 内 `cells/<cell>/ltx/e<epoch>/`（`client_for` 把 `config.path` 设成该前缀）
- 一个 cell 同一时刻一个 writer；换 owner 靠 bucket CAS

**跨 version 共享的前提：** 父子 wrangler 的 `database_id` **必须相同**，否则 scope 哈希不同，base pointer 会对着空前缀。T4 必须保证 fork 出的 version **复制父 version 的 `database_id`**，禁止 e2e 再给每个 version 新 uuid。

### 1.2 Restore 要求本前缀内有连续 LTX，且通常要 MinTXID==1 锚点

- `calc_restore_plan`（`celld/crates/ltx/src/replica.rs`）只对 **一个** `ReplicaClient` 列目录
- 计划为空 → `Error::TxNotAvailable`（G3 在毒 guestbook 上空前缀见过）
- import 路径强制 `pos.txid >= 1` 且 `read_ltx_file(0, TXID(1), pos.txid)`（`ltx_repl.rs` `import_sqlite_seed`）

子 version 若只上传「从父 head 之后的增量」、自己前缀里 **没有** MinTXID==1 文件，**今天的 restore 会直接失败**。这是本计划在 ltx crate 里必须改的核心。

### 1.3 cellp 部署路径

`orchestrator.go`：checkpoint → fork（offshoot）→ **export `seed.db`** → `Deploy` → `Start` → `D1Execute`（`d1 import --file seed.db` 到 **子 version bucket**）。

`versionBucket` = `s3://cellp-celld/{project}/{version}`。父子进程 **读不到对方前缀**，除非显式再开一个 S3 客户端指向父 URI。

### 1.4 已否决、且本计划继续否决的捷径

| 捷径 | 为什么否决 |
|------|------------|
| 让 celld 打开 offshoot store / checkout 当 D1 | 两套 LTX 布局、lease、WAL 捕获绑死；D1-IMPORT 已冻结「celld 不读 offshoot」 |
| `fs::copy` 父 `db.sqlite` 到子 watch | 无 LTX，G3 级 restore 空库或 TxNotAvailable |
| 子 version 再跑一遍 `d1 import` 全量 seed | 这就是今天的路径，对象仍是 N× 全量 |
| 同一 celld 进程托管多个 version 的 D1 | 违反 AD-1 |
| JSON 里塞 SQLite bytes | 违反 D1-IMPORT 冻结精神 |
| `execute --file` 见魔数就 branch | operator 脚枪；独立子命令 |

---

## 2. 冻结设计（对抗后不得改方向）

### 2.1 一句话

**在 celld 自己的 LTX 对象层做 offshoot 那种 base pointer：子 bucket 只存指针 + 分叉后的增量；restore 先读父前缀到 `fork_txid`，再叠子前缀。** 不把 offshoot 当 VFS。

这与 offshoot `data/{lineage}/base.json` + `store.Chain` 同构，对象格式仍是 celld 已用的 LTX（Litestream v0.5 系），不是去解析 offshoot 的 lineage 目录。

### 2.2 `base.json`（子前缀，create-only）

路径（子 version bucket 内）：

```text
cells/<scope>/ltx/e<epoch>/base.json
```

正文 **仅** JSON（无 SQLite）：

```json
{
  "parent_bucket": "s3://cellp-celld/{project}/{parentVersion}",
  "parent_cell": "__D1Database:<hex>",
  "parent_epoch": 1,
  "fork_txid": 42,
  "fork_checksum": "<hex PreApplyChecksum of next incremental>"
}
```

- `parent_bucket`：父 **version** 的 celld bucket，不是 offshoot store。
- `parent_cell`：必须等于子 D1 scope（同一 `database_id`）。
- `fork_txid`：branch 调用返回时父 replica 的 `pos.txid`。父之后继续写入，子 **看不见**（git fork 语义）。
- 禁止字段：`path`、`bytes`、`sql`。

CAS：`If-None-Match: *` 创建；已存在则 fail（不可把已 diverge 的子改绑到另一个父）。

### 2.3 TXID 合同

- 父前缀：现有规则不变（含 MinTXID==1 snapshot）。
- 子前缀：**禁止**再打 MinTXID==1 全量 snapshot（那会再次上传整库）。
- 子的第一条 LTX：`MinTXID = fork_txid + 1`，`PreApplyChecksum` 与父在 `fork_txid` 的 post-apply checksum 衔接。
- 子 restore：`plan = parent_plan(txid<=fork_txid) ++ child_plan(min>fork_txid)`，必须连续，否则 `TxNotAvailable`。

`import_sqlite_seed` **只用于没有父的根 version**。`d1 branch` **禁止**调用 `reset_local_state` + 全量 `sync` 那种 MinTXID==1 导入。

### 2.4 CLI

```text
celld d1 branch DATABASE --parent-bucket URI [PROJECT] --bucket CHILD_URI
```

- `DATABASE`：唯一 `d1_databases[].database_name`（与 import 相同规则：0 个跳过由 cellp 做；>1 个 fail）。
- `--parent-bucket`：父 version 的 `s3://cellp-celld/{project}/{parentVersion}`。
- `--bucket`：子 version bucket（已有 fleet flags）。
- 需要 **正在运行的子 fleet**（与 `d1 import` 相同：走 owner `/runtime/<scope>`）。
- 父 bucket 只需 **读** LTX + 读父 head txid；写只发生在子 bucket。

Exit：`0` 成功 · `1` 校验/复制/IO · `2` 用法。`--help` **不得**要求 DATABASE 位置参数（与 import 相同：先 peek help）。

### 2.5 HTTP（子 owner `/runtime/<scope>`）

Request **仅**：

```json
{
  "branch": {
    "parent_bucket": "s3://cellp-celld/demo-app/v-parent",
    "parent_epoch": 1
  }
}
```

`fork_txid` **由 owner 读父 replica 决定**，禁止客户端乱填（防把子接到错误高度）。

Success：

```json
{
  "ok": {
    "fork_txid": 42,
    "parent_bucket": "s3://...",
    "bytes_parent": 105005056,
    "duration_ms": 800
  }
}
```

`bytes_parent` 是父 snapshot 大小（诊断），**不是**子前缀新上传的字节。

Failure family：`D1_BRANCH_ERROR`。

### 2.6 Owner 顺序（不得省略、不得 fs::copy）

在 **子** isolate 线程上：

1. 解析 wrangler，确认唯一 D1，scope 与父 `database_id` 一致（父 wrangler 由 CLI 读 `--parent-bucket` 的 deploy 指针 **或** 要求调用方传入同一 PROJECT 目录；一期：**同一 `projectDir`，database_id 不变**）。
2. `storage::close(scope)`（与 import 相同：关连接必须在 isolate 线程）。
3. 用父 bucket 凭证（与子相同的 AWS key，endpoint 指向 RustFS）构造父 `ObjectStoreClient`，`highest_nonempty_epoch` + `calc_pos` 得到 `fork_txid`。
4. 若父前缀无法 restore 到该 txid → fail，**不要**写 base.json。
5. 子前缀 CAS 写入 `base.json`。
6. **本地**按合成 restore plan 重建 `db.sqlite`（父页 + 尚无子增量）。禁止 `std::fs::copy` 父 watch 文件。
7. 打开 managed `Db` 后，在任何 `db.sync()` / SQL **之前**：
   - 将父 plan 在 `fork_txid` 的**终端 LTX 文件字节**写入本地 L0：`Db::seed_l0_baseline(min, fork_txid, parent_ltx_bytes)`（`db.rs`）。
   - **禁止**只调用 `Replica::seed_pos(fork_txid)`：本地无 LTX 时 `Db::pos()==0`，`verify()` 会 first-sync 打出 **MinTXID==1 全量 snapshot**，直接违反 §2.3 / B2 / B6。
   - `fork_checksum` = 该终端 LTX 的 `post_apply_checksum`；子**首条**增量 `pre_apply_checksum` 必须匹配。
   - **不要** `reset_local_state`。
8. `storage::open_at_epoch` 仍在 isolate 线程 reopen（与 import 相同：`prepare` on turn → async restore+seed_l0 → `register_d1_branch_reopen`）。
9. 应答 CLI。子此后的 SQL 提交只向 **子前缀** `sync_cell` 增量。

`parent_epoch` **禁止** RPC 默认盲写 `1`。owner 对父 bucket 调用 `highest_nonempty_epoch(parent_cell)`。

`base.json` 用 `bucket.put_cas(key, body, None)` 创建；已存在 → `D1_BRANCH_ERROR`。

一层 fork：若父前缀已有 `base.json` → fail fast（`parent is already a branch; compact first`）。P0 不实现 compact。

Isolate panic 约束与 import 相同：close / open_at_epoch 不得在任意 tokio 任务上跑（见 `d1_import` prepare/reopen 分裂）。实现应复用同一模式：`prepare` on turn → async restore → `reopen` on op delivery。

### 2.7 子进程启动（activate）

`LtxRepl::activate`：若本地无 db 且子前缀有 `base.json`：

- 合成 restore（父+子），再打开 replica
- 若 `base.json` 缺失且无 MinTXID==1 → 保持今天的 `TxNotAvailable`（根 version 仍须 import）

### 2.8 cellp

`D1Execute` 保持：有 seed.db 且 SQLite 魔数 → `d1 import`（**根 version / 无父**）。

新增 `D1Branch(ctx, project, childVersion, parentVersion, projectDir)`：

```text
celld d1 branch DATABASE --parent-bucket s3://cellp-celld/{project}/{parent} \
  projectDir --bucket s3://cellp-celld/{project}/{child} ...
```

Orchestrator：

- 若 `v.ParentVersionID` 非空 **且** bundle 有且仅有一个 `d1_databases` **且** 父 version `StatusReady`：  
  **跳过 export**；`Start` 之后调 `D1Branch` 而非 `D1Execute`。父非 ready → **fail deploy**（禁止 silent skip）。`CELLP_STRICT_OFFSHOOT_FORK=1` 时 `D1Branch` 失败必须 abort（与今天 `D1Execute` 相同）。
- 否则保持今天的 export + `D1Execute`。
- wrangler 0 个 D1：整段跳过（已有测试）。
- **database_id**：部署前从 `{ArtifactsDir}/{project}/{parentVersion}/wrangler.jsonc`（或父 bundle）读取 `d1_databases[0].database_id`，写入子 `destDir/wrangler.jsonc`。e2e 必须用 `dev/examples/d1-seed` 的固定 id，禁止每 version 新 uuid。
- `D1Branch` 必须在 Health 门之前完成。

Offshoot 的 fork 仍做（数据分支隔离给以后 checkout）。本计划 **不要求** 去掉 offshoot；只去掉「为了 D1 再 export+全量 import」。

### 2.9 本地磁盘诚实声明

两个 ready version 同时跑时，watch 下仍是两份完整 `db.sqlite`（AD-1）。Gate **不**把「本地 du 减半」当成功。成功标准是 **RustFS/S3 上子前缀对象体积 ≪ 父 snapshot**。

---

## 3. 目标门禁（必须可测；对抗可加严，不可改成「restart 近似」）

| ID | 要求 | 权威证据 |
|----|------|----------|
| **B1** | `celld d1 branch --help` 不要求 DATABASE；子命令存在 | CLI `-h` |
| **B2** | 根 version import 100 MB（或测试用 ≥8 MB 不可压缩）后，子 version branch，子前缀 **没有** MinTXID==1 全量 LTX，**有** `base.json` | `aws s3 ls` / celld diagnose 列出 keys |
| **B3** | 子 Worker `GET /count`（或 `d1 execute` checksum）等于 **父 seed 行数** | e2e |
| **B4** | 子再 INSERT 独有行；父 Worker **看不到** 这些行；另一从同一父 branch 出的兄弟 C **看不到** 子的行 | e2e 双 version |
| **B6** | 对象体积：度量时点 = **branch 刚完成、子尚无 SQL**。父 snapshot ≈ seed；子前缀 `sum(keys)`（含 `base.json`）**小于 seed 的 20%**，且 **无** `min_txid==1` 的全量 LTX key。另记录首条 INSERT 后增量。100 MB 档子前缀 **不得**再出现 ~100 MB 对象 | jsonl |
| **B7a** | counter 无 D1：仍 skip celld d1 | go test |
| **B7b** | cellp 有父+D1：走 `d1 branch`，日志无 `d1 seed took` import；e2e 用 `dev/examples/d1-seed` | go test argv + e2e |
| **B8** | 单测：chained restore plan（L9+L0）、错误 parent_cell、二次写 base.json、checksum 错则 sync fail、`base.json` 存在时 `d1 import` reject | cargo + go |
| **B5** | kill 子 celld，**擦子 CELLD_WATCH**，**新进程**同子 bucket，查询仍见父行 + 子 INSERT。禁止 warm restart 当证据。8 MB 档也必须跑 B5 | e2e / stress |

100 MB 全链路（B6 大档）允许作为 `stress/phase6/d1-branch-scale.sh`；CI/默认可用 8 MB。大档与小档 **同一套不变量**（无子全量 snapshot、wipe-watch restore、兄弟隔离）。

---

## 4. 分轨（对抗通过后并行；文件独占）

| Track | 仓库 | 独占 | 产出 |
|-------|------|------|------|
| **T1** | `celld/crates/ltx` | `replica.rs` restore 合成、新 `base.rs`（parse/serialize `base.json`）、`ReplicaClient` 组合类型 | 单测：父 snapshot + 子增量 plan；空子前缀仍能 restore 到 fork_txid |
| **T2** | `celld/crates/celld` `ltx_repl.rs` | `client_for` 读 base、activate restore、**禁止** branch 路径调用 `import_sqlite_seed` | activate 遇 base.json 走合成 restore |
| **T3** | `celld` CLI/RPC | 新 `d1_branch.rs`（不要塞进 `d1_import.rs` 的 validate_seed）、`d1_cli.rs` 子命令、`js.rs`/`harness.js` `__d1_branch` op、isolate close/reopen 与 import 同模式 | CLI + RPC |
| **T4** | `cellp` | `runtime/manager.go` `D1Branch`、`orchestrator.go` 分支、`manager_test.go` | 有父则 branch，无父则 import |
| **T5** | `e2e/` + `stress/phase6/` | `e2e/scripts/v1-d1-branch.sh`、`stress/phase6/d1-branch-scale.sh`、`docs/evidence/d1-branch-*.md` | B3–B6 |

T1 必须先合入（或 T1/T2 同一 composer 串行），否则 T3 无法 restore。T4 依赖 T3 CLI。T5 依赖 T3+T4。

**禁止** T3 为了让 CLI 变绿而在子前缀 `import_sqlite_seed`。

---

## 5. 实现要点（给 Composer，禁止发挥成别的架构）

### 5.1 `ChainedReplicaClient`

完整契约（T1 冻结，不得只实现 `ltx_files`）：

```text
open_ltx_file(level, min, max):
  if child owns (level, min, max): child.open
  else: parent.open

ltx_files(level, seek):
  parent_files = parent.ltx_files(level, seek) filtered max_txid <= fork_txid
  child_files  = child.ltx_files(level, max(seek, fork_txid+1))
  merge sorted by min_txid; dedupe by (level, min, max)

write_* / delete_*: child only
```

`calc_restore_plan(chained, txid)` 的结果必须等于「父 plan（txid≤fork_txid，含 **L9 snapshot + L0**）再叠子增量」。100 MB import 实证 restore 为 `levels=L9:1`，chained client 必须让父 L9 进入 plan。

`LtxRepl::activate`：在单子 `client_for` restore **之前**，若子前缀有 `base.json`，走 `chained_client_for`。`has_any_object` 在仅有 `base.json` 时可能为 true，**不得**当成「可单前缀 LTX restore」。

T1 **不得**修改 `import_sqlite_seed` 或根 version 无 `base.json` 的 restore 路径。

### 5.2 checksum 衔接

子第一条增量的 `PreApplyChecksum` 必须等于父 `fork_txid` 快照的 post-apply checksum。实现时从父 LTX header 读取，写入 `base.json.fork_checksum`，子 capture 启动时注入。测：故意写错 checksum → sync fail，不得静默出分叉库。

### 5.3 深度上限

offshoot 有 fork spine 16 层改打全量 snapshot。一期 **只支持一层**：子的 parent 必须是 **根**（父前缀 **没有** `base.json`）。孙 version（parent 已是 branch）**fail fast**，文案要求先 `compact`（T2 可留 `d1 compact` 为 T6 非目标）。避免一期做传递 `Chain`。

对抗若认为一层不够「基础能力」：允许 **至多 2 层** 传递（读父 base.json 再读祖），仍禁止无限链。计划默认 **1 层**；对抗可改为 2，不得超过 2。

### 5.4 compact（非 P0）

`compact` = 子前缀打 MinTXID==1 全量 snapshot 并删除 base.json。P0 **不做**。需要时另开计划。B 门禁不得依赖 compact。

### 5.5 安全

- 子节点任意 `parent_bucket` URI：只允许 **同 endpoint 的 `s3://cellp-celld/{sameProject}/...`**。禁止指向别的项目、禁止 http 任意主机（对齐 artifact SSRF 门）。
- `parent_cell` 必须等于本 scope。

### 5.6 观测

`base.json` 存在时日志：`event="d1_branch_restore" fork_txid= parent_bucket= child_objects= parent_objects=`。T5 从 celld 日志引用。

---

## 6. 测试与证据文件

| 文件 | 内容 |
|------|------|
| `docs/evidence/d1-branch-scale-report.md` | 运行结果 B2/B5/B6 |
| `docs/evidence/d1-branch-metrics.jsonl` | `child_prefix_bytes`、`parent_snapshot_bytes`、`fork_txid` |
| `e2e/scripts/v1-d1-branch.sh` | 父 import 42 行 → 子 branch → Worker 42 → 子 INSERT → 父仍 42 |
| `cellp/internal/runtime/manager_test.go` | `D1Branch` argv 含 `--parent-bucket`；无 D1 skip；无父走 import |
| `celld` `cargo test -p celld-ltx` / `-p celld` | restore 合成、base.json CAS |

e2e 隔离：父、子 **各自** version bucket，**不要**用共享 `demo-app` 毒 guestbook。`database_id` 父子相同。`CELLD_READY_FLEET_GATE_MS=5000` 由 cellp `Start` 注入（已有）。Health 用 `/.well-known/celld/health`（已有）。

---

## 7. 非目标（对抗禁止扩 scope）

- 不改 offshoot、不让 celld 读 `OFFSHOOT_STORE`
- 不做 row-level merge、不做跨无关 D1 的页去重
- 不做 PG / 多租户 / 多 celld 同进程多 version
- 不把 50 个 live version 当成功标准（AD-1 本地仍是 N 份 `db.sqlite`）
- 不改 import RPC；根 version 仍 `d1 import --file`
- 不做 `d1 compact`、不做无限 fork 链
- 不把 checkout 磁盘减半写成 Gate

---

## 8. 派发顺序

1. Composer **对抗**本文件 → `docs/plans/REVIEW-celld-d1-branch.md`（claim-by-claim，对照当前树行号）。
2. Grok 合入强制修正。
3. Composer 并行：T1（可与 T3 骨架）→ T2+T3 → T4 → T5。
4. Grok 对照 §3 逐项用当前树证据审计；未证明不得标完成。
5. Composer tester：跑 e2e + 至少一档 stress。

---

## 9. 成功时用户能看见什么

部署 **子** version（有父、有 D1）不再把 100 MB 再 import 进新 bucket。对象存储里父仍一份快照，子只有 `base.json` 和自己的增量。两个子 version 各自写入互不可见。杀掉子 celld、清掉本地 watch，子仍能从「父 LTX + 子增量」恢复。
