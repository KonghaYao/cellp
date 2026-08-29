# celld D1 跨 version 共享基线 — 对抗审查

> 审查日期：2026-08-29 · 审查者：composer（对抗）  
> 计划：`docs/plans/celld-d1-branch.md`（**已完成 T1–T5**）  
> 源码锚点：当前树 celld `ltx_repl.rs` / `replica.rs` / `d1_import.rs` · cellp `manager.go` / `orchestrator.go` · 冻结契约 `docs/plans/D1-IMPORT-RPC.md`

---

## 1. Verdict

**APPROVE-WITH-CHANGES**

方向正确：在 celld LTX 对象层做 `base.json` + 父前缀只读合成 restore，是 AD-1 /「celld 不读 offshoot」约束下唯一不重复上传 100 MB 全量 snapshot 的路径。计划对否决捷径（`import_sqlite_seed` 冒充 branch、`fs::copy`、JSON 塞字节）的立场与 D1-IMPORT 冻结一致。

**不能原样派发的原因：**

1. §2.6 owner 序列只写 `seed_pos(fork_txid)`，**未绑定** `restore_from_plan` 之后本地 LTX 为空、`Db::pos()==0` 时 `verify()` 会走「first sync → MinTXID==1 全量 snapshot」——直接违反 §2.3 与 B2/B6。
2. `ChainedReplicaClient` 只写了 `ltx_files` 伪代码，未冻结 `open_ltx_file` 路由、`calc_restore_plan` 在 L9 snapshot + L0 链上的合并语义，以及 activate 在 **单 client** restore 之前如何短路。
3. `parent_bucket` SSRF 门仅有口号，无可对照的解析/拒绝规则（artifact 侧已有 `validateArtifactURI` 可对齐）。
4. cellp **今天**仍 export + `D1Execute`；`database_id` 父子一致、父 version ready、跳过 export 后 fail-closed 均未写入可执行接线。
5. 无独立 `D1-BRANCH-RPC.md`；branch 超时、负例门禁、一层 fork 的检测点未冻结到函数级。

对抗审查通过后，Grok 必须先合入下文 **§3 强制修正**，再并行 T1–T5。

---

## 2. Claim audit

### 2.0 用户可见问题（§0）

| 声明 | 判定 | 证据 |
|------|------|------|
| offshoot store 层可共享基线 + 分支增量（对象 ~200 MB 非 300 MB） | **true**（offshoot 设计） | `DESIGN.md:145`（base.json + branch ref） |
| celld **今天**无跨 version LTX 共享 | **true** | 全树无 `base.json` / `ChainedReplicaClient` / `d1 branch` |
| 每 ready version：独立进程（AD-1） | **true** | `manager.go:89-119` 每 version 起一个 `celld` |
| 每 version 独立 bucket `s3://cellp-celld/{project}/{version}` | **true** | `manager.go:55-57` `versionBucket` |
| 每 cell 独立 LTX 前缀 `cells/<scope>/ltx/e<epoch>/` | **true** | `ltx_repl.rs:1219-1227` `client_for` |
| import / 首次 restore 依赖连续 LTX + 锚点 | **true**（根 version） | `import_sqlite_seed` 强制 `read_ltx_file(0, TXID(1), …)`：`ltx_repl.rs:2151-2157`；空前缀 → `TxNotAvailable`：`replica.rs:748-749` |
| cellp 子 version 仍 export 整份 seed → `d1 import` | **true** | `orchestrator.go:176-207` export → `D1Execute` → `d1 import` |
| G3 restore 对象约 **105 750 979 B** | **true** | `docs/evidence/d1-import-scale-report.md:30` |
| 本地 watch 仍各一份完整 `db.sqlite`（AD-1） | **true** | `manager.go:121-139` 每 version 独立 `CELLD_WATCH` |

### 2.1 源码事实 §1.1 — D1 = cell SQLite + LTX

| 声明 | 判定 | 证据 |
|------|------|------|
| 类名 `__D1Database`；scope = `d1_cell_scope(database_identity)` | **true** | `deploy.rs:64` `D1_CLASS`；`js.rs:7155-7157` |
| `database_identity` = `database_id` 或 fallback `database_name` | **true** | `deploy.rs:874-878` |
| 本地 `<watch>/<cell>/ltx/e<epoch>/db.sqlite` | **true** | `ltx_repl.rs` `db_path`（`activate` 用 `self.db_path`：`1258+`） |
| 远端 `cells/<cell>/ltx/e<epoch>/` | **true** | `ltx_repl.rs:1226` `config.path` |
| 跨 version 共享要求父子 `database_id` 相同 | **true**（scope 由 identity 哈希） | `js.rs:7155-7157` + `deploy.rs:874-878` |
| T4 必须复制父 `database_id`，禁止每 version 新 uuid | **unproven**（计划要求，**树中无实现**） | `orchestrator.go` 仅 `artifact.Fetch` 子 artifact；全 cellp 树无 `database_id` 复制逻辑 |

### 2.2 源码事实 §1.2 — Restore 与 import 锚点

| 声明 | 判定 | 证据 |
|------|------|------|
| `calc_restore_plan` 只对**一个** `ReplicaClient` 列目录 | **true** | `replica.rs:625-658` 单 `client` 参数 |
| 计划为空 → `Error::TxNotAvailable` | **true** | `replica.rs:748-749` |
| import 强制 `pos.txid >= 1` 且 `read_ltx_file(0, TXID(1), pos.txid)` | **true** | `ltx_repl.rs:2151-2157` |
| 子前缀仅有 fork 后增量、无 MinTXID==1 → **今天** restore 失败 | **true** | `activate` 远程 restore 用单子 bucket client：`ltx_repl.rs:1385-1394`；无 LTX → `TxNotAvailable` |
| 子 version 不能靠「只上传父 head 之后增量」通过**今天**的 restore | **true** | 同上 |

**计划未写但审查发现的阻塞事实：**

| 声明 | 判定 | 证据 |
|------|------|------|
| `restore_from_plan` **只写** `db.sqlite`，不写本地 LTX 目录 | **true** | `replica.rs:530-537` `restore_from_plan_inner` → DB 镜像；无 `seed_l0_baseline` |
| 本地无 LTX 时 `Db::pos()` 返回 `Pos::ZERO` | **true** | `db.rs:692-694` |
| `Db::verify()` 在 `pos.txid==0` 时标记 first sync → **全量 snapshot**（MinTXID=1） | **true** | `db.rs:884-887`；snapshot `min_txid: TXID(1)`：`db.rs:1778-1779` |
| 已有 API `Db::seed_l0_baseline` + `Replica::check_database_behind_replica` 可衔接远程 LTX | **true** | `db.rs:763-781`；`replica.rs:342-375` |
| 100 MB import 的 restore plan 为 **L9:1** 单对象（非仅 L0 MinTXID==1） | **true** | `d1-import-scale-report.md:30` `levels=L9:1` |

### 2.3 源码事实 §1.3 — cellp 部署路径

| 声明 | 判定 | 证据 |
|------|------|------|
| checkpoint → fork → export → Deploy → Start → D1Execute | **true** | `orchestrator.go:148-207` |
| `versionBucket` = `s3://cellp-celld/{project}/{version}` | **true** | `manager.go:55-57` |
| 父子进程默认读不到对方 bucket 前缀 | **true** | 各 `celld` 仅 `--bucket` 自己的 `versionBucket`：`manager.go:113-118` |
| `D1Execute` 对 SQLite 走 `d1 import`（G1 已落地） | **true** | `manager.go:237-245` |
| 默认 bundle 仍为 `dev/examples/counter`（无 D1） | **true** | `orchestrator.go:187-188` |
| wrangler 0 个 `d1_databases` 时 `D1Execute` 跳过 | **true** | `manager.go:227-232` `d1DatabaseName` → `""` |

### 2.4 已否决捷径（§1.4）— 当前树仍成立

| 捷径 | 判定 | 证据 |
|------|------|------|
| celld 打开 offshoot store | **仍否决**（无代码路径） | 计划 §7；D1-IMPORT 冻结 |
| `fs::copy` 父 `db.sqlite` | **仍否决** | import 用 `sqlite_snapshot`：`ltx_repl.rs:2140` |
| 子 version 再跑全量 `d1 import` | **今天仍发生** | `orchestrator.go:207` |
| 同一 celld 多 version D1 | **仍否决** | AD-1：`manager.go:89+` |
| JSON 塞 SQLite bytes | **仍否决** | `D1-IMPORT-RPC.md`；`d1_import.rs` path-only |
| `execute --file` 魔数 branch | **仍否决** | `d1_cli.rs:477-485` 拒绝 SQLite，指向 `import` |

### 2.5 冻结设计 §2.1–2.9

| 声明 | 判定 | 证据 |
|------|------|------|
| **2.1** base pointer：子 bucket 指针 + 增量；restore 父到 `fork_txid` 再叠子 | **unproven**（合理，未实现） | 无 `base.rs` / `ChainedReplicaClient` |
| **2.2** `base.json` 路径与字段；CAS create-only | **unproven** | 树中无 `base.json` 读写；CAS 可用 `bucket.put_cas`：`bucket.rs:838+` |
| **2.3** 子禁止 MinTXID==1 全量；第一条 `MinTXID=fork_txid+1`；合成 plan 须连续 | **unproven**（**与 §2.6 step 7 冲突**，见 §4） | 合同合理；实现缺失 |
| **2.4** `celld d1 branch` CLI 形状 | **unproven** | `d1_cli.rs` 无 `branch` 子命令 |
| **2.5** HTTP `branch` RPC；`fork_txid` 服务端决定 | **unproven** | `js.rs` 仅有 `__d1_import`：`5156+` |
| **2.6** owner 顺序 close → 读父 head → CAS base → 合成 restore → `seed_pos` → reopen | **partially false** | close/reopen 模式可复用 `d1_import.rs:152-211`；**缺本地 L0 seed**（§4.1） |
| **2.6** 禁止 `import_sqlite_seed` / `reset_local_state` on branch | **unproven**（禁令正确） | 无 branch 路径 |
| **2.7** `activate` 遇 `base.json` 走合成 restore | **false**（今天） | `activate` 仅 `client_for(cell, from)` 单子 restore：`1385-1394` |
| **2.8** `D1Branch`；有父则 skip export；否则 import | **false**（今天） | `orchestrator.go` 无条件 export + `D1Execute` |
| **2.8** 从父 artifact 拷 `database_id` | **false**（今天） | 无 cellp 实现 |
| **2.9** Gate 不以本地 du 减半为成功 | **true**（陈述） | 可测性在 B6 |

---

## 3. Mandatory plan amendments（派发前 Grok 必须写入计划）

| ID | 严重度 | 修正 |
|----|--------|------|
| **B1** | **BLOCKER** | §2.6 在 `seed_pos(fork_txid)` **之前**增加：**本地 L0 基线**。`restore_from_plan` 只产出 `db.sqlite`（`replica.rs:530+`）。必须在 isolate 线程对 `Db` 调用 `seed_l0_baseline(min, fork_txid, parent_ltx_bytes)`（`db.rs:763-781`），其中 `parent_ltx_bytes` 为父 plan 在 `fork_txid` 的**终端 LTX 文件**（与 `fork_checksum` 同源）。**禁止**仅靠 `Replica::seed_pos`：`Db::pos()==0` 时 `verify()` 会 first-sync 打 MinTXID==1（`db.rs:884-887`），违反 §2.3/B2。Gate：子首条 SQL 后 bucket 列目录**不得**出现 `min_txid==1` 的 LTX。 |
| **B2** | **BLOCKER** | 冻结 `ChainedReplicaClient` 完整契约：`ltx_files` **与** `open_ltx_file` 必须按 `(level, min_txid)` 路由父/子 store；`write_*` / `delete_*` 仅子 client。`calc_restore_plan` 输入 chained client 时结果须等于 `parent_plan(txid≤fork_txid) ++ child_plan(min>fork_txid)`，且覆盖 **L9 snapshot + L0 链**（import 实证 `levels=L9:1`：`d1-import-scale-report.md:30`）。单测：空子前缀 restore 到 `fork_txid`；故意断档 → `TxNotAvailable`。 |
| **B3** | **BLOCKER** | `LtxRepl::activate`（`ltx_repl.rs:1344-1416`）：在单子 `client_for` restore **之前**，若子前缀存在 `base.json`，改走 `chained_client_for(cell, epoch)` 合成 restore；无 `base.json` 且无 MinTXID==1 锚点 → 保持 `TxNotAvailable`。`epoch_replicated` / `has_any_object` 在仅有 `base.json` 时为 true（`object_store.rs:593+`），**不得**把「有对象」等同于「可 LTX restore」。 |
| **B4** | **BLOCKER** | `parent_bucket` 校验函数（建议 `d1_branch.rs::validate_parent_bucket`）：解析 `s3://cellp-celld/{project}/{parentVersion}`；`bucket` 必须 `cellp-celld`；`project` 必须等于当前 `CELLD_VAR_PROJECT_ID` / CLI project；**拒绝** `http(s)://`、其它 bucket、路径穿越。对齐 `artifact/store.go:82-99` `validateArtifactURI` 精神。CLI 与 owner **共用**同一校验。 |
| **B5** | **BLOCKER** | 新增 `docs/plans/D1-BRANCH-RPC.md`（比照 `D1-IMPORT-RPC.md`）：request 仅 `parent_bucket` + `parent_epoch`（**禁止**客户端 `fork_txid`）；success/error 族；owner 顺序（含 B1 本地 L0 seed）；exit 0/1/2；**timeout** `max(600s, parent_snapshot_mb * 2s)`（100 MB 父 restore 可能 >120s execute 预算）。 |
| **B6** | **CRITICAL** | cellp `orchestrator.go`：`v.ParentVersionID != ""` && 唯一 `d1_databases` && **父 version `StatusReady`** → **跳过** `export`（`branchStep` export）；`Start` 后调 `D1Branch` 而非 `D1Execute`。父非 ready → fail deploy（勿 silent skip）。`strictOffshoot` 下 `D1Branch` 失败必须 abort（今天仅 wrap `D1Execute`：`207-210`）。 |
| **B7** | **CRITICAL** | cellp T4：**部署前**从 `{ArtifactsDir}/{project}/{parentVersion}/wrangler.jsonc`（或 bundle）读取 `d1_databases[0].database_id`，写入子 `destDir/wrangler.jsonc`（子 artifact 无同 id 时覆盖）。`manager_test.go` 加 `TestD1BranchPassesParentBucket` + `TestOrchestratorSkipsExportWhenParentD1`。e2e **必须** `dev/examples/d1-seed`（`wrangler.jsonc:9` 固定 `database_id`），**不得**用 counter 声称 B3–B7。 |
| **B8** | **CRITICAL** | 一层 fork：`branch` 前读父前缀 `base.json`；若存在 → **fail fast**（「parent is already a branch; compact first」）。默认 **1 层**；若 Grok 采纳 2 层，须写清递归读祖 `base.json` 上限与 `ChainedReplicaClient` 嵌套，**不得超过 2**。 |
| **B9** | **MAJOR** | `fork_checksum`：从父终端 LTX trailer 读取 `post_apply_checksum` 写入 `base.json`；子**首条**增量 `pre_apply_checksum` 必须匹配（`db.rs` capture 路径）。负例单测：故意错 checksum → `sync` fail，不得 silent fork。 |
| **B10** | **MAJOR** | `parent_epoch`：**禁止** RPC 默认盲写 `1`；owner 用父 bucket 上 `highest_nonempty_epoch(parent_cell)`（复用 `ltx_repl.rs:1233-1247` 逻辑，但 client 指向父 URI）。 |
| **B11** | **MAJOR** | isolate 线程：`d1_branch` 复用 `d1_import` 的 `prepare`/`reopen` 分裂（`d1_import.rs:133-211`）；`register_d1_branch_reopen` 比照 `register_d1_import_reopen`（`js.rs:1449,5190`）。**禁止**在 tokio 任务上 `storage::close` / `open_at_epoch`。 |
| **B12** | **MAJOR** | `base.json` CAS：用 `bucket.put_cas(key, body, None)` create（`bucket.rs:838`）；已存在 → `D1_BRANCH_ERROR`。禁止 overwrite。 |
| **B13** | **MAJOR** | 加强 B 门禁（§10）：B5 必须 wipe `CELLD_WATCH` 后**新进程** restore（禁止 warm restart）；B6 断言子前缀 `sum(keys)` 含 `base.json` 但**无**全量 LTX key；加负例：`celld d1 import` 在已有 `base.json` 子前缀 **必须拒绝**。 |

---

## 4. Hunt：计划必须正面回答的风险

### 4.1 TXID / checksum 连续性（**计划缺口 → B1/B9**）

| 风险 | 结论 | 证据 |
|------|------|------|
| 仅 `seed_pos(fork_txid)` 能否避免子前缀 MinTXID==1？ | **否** | `restore_from_plan` 不写本地 LTX；`Db::pos()==0` → first sync snapshot `min_txid=1`（`db.rs:884-887,1778`） |
| `Replica::sync` 在 `dpos==0` 时行为？ | 上传前即失败 `"no position, waiting for data"` | `replica.rs:237-238`；但 **SQL 路径先 `db.sync()`**，会在 replica sync 前打出 forbidden L0 |
| `fork_checksum` 如何注入 capture？ | 计划未绑定 API | 需写明：终端父 LTX bytes → `seed_l0_baseline` + 首条 incremental 校验 `pre_apply_checksum` |
| 父仅 L9 snapshot、无 L0 时子第一条 incremental？ | 计划未写 | `calc_restore_plan` 先探 L9（`replica.rs:661-672`）；chained client 须保证父 L9 进入 plan |

### 4.2 isolate close / open（**可复用 import，须写进 B11**）

`d1_import::prepare` 在 isolate turn 上 `storage::close`；async 段跑 LTX；`reopen` 回 isolate（`d1_import.rs:152-211`）。branch **必须**同一模式，否则违反 D1-IMPORT 已证的线程约束。

### 4.3 `parent_bucket` SSRF（**计划过软 → B4**）

今天 cellp artifact 拒绝非允许 bucket 与 http SSRF（`artifact/store.go:82-99`）。`celld d1 branch --parent-bucket` 若不做同等校验，operator 可把子 celld 指向任意可读 S3 前缀（凭同一 `AWS_*`）拉取无关 LTX。

### 4.4 `database_id` mismatch（**今天会静默失败 → B7**）

scope = `d1_cell_scope(database_identity)`（`js.rs:7155-7157`）。子 deploy 用**子 artifact** 的 wrangler（`orchestrator.go:132-189`），若 CI 给每 version 新 `database_id`，`parent_cell` 与子的 scope **永不相等**，base pointer 指向空父前缀。T4 复制 `database_id` 是 **BLOCKER 级接线**，不是 e2e 礼仪。

### 4.5 AD-1 vs 共享 store

计划正确：**不**合并进程；共享仅在 S3 对象层。`manager.go:121-139` 每 version 独立 watch。无冲突。

### 4.6 一层 fork limit（**须 fail fast → B8**）

若父前缀已有 `base.json`（父已是 branch），仍允许 branch → 需递归 chain 或 compact。计划默认 1 层但未写**检测函数**与错误文案落点（`d1_branch.rs` validate 阶段）。

### 4.7 `ChainedReplicaClient` 是否破坏根 version `MinTXID==1` import？

**不会**——若 gated 正确：

| 路径 | 行为 |
|------|------|
| 根 version import | `import_sqlite_seed` → `reset_local_state` + MinTXID==1（`ltx_repl.rs:2146-2157`） |
| 根 activate（无 `base.json`） | 单子 client restore（`1385-1394`） |
| 子 activate / branch | **仅**在存在 `base.json` 时启用 chained client（B3） |

审查要求：T1 **不得**改 `import_sqlite_seed` 或根 import 路径；单测回归 G3 import scale。

### 4.8 跳过 export 是否安全（offshoot 仍 fork）

| 项 | 结论 |
|----|------|
| offshoot fork 仍执行 | **true** `orchestrator.go:166-168` — 计划保留 |
| D1 不依赖 `seed.db` 若走 `D1Branch` | **true** — 但须 **显式** `D1Branch`；今天 `os.Stat(seedPath)` 失败则跳过 D1（`205-215`），不会自动 branch |
| 其它依赖 `seed.db` 的子系统 | 当前 orch **仅** D1 读 `seedPath`；跳过 export 可接受 **当且仅当** B6 接线 |
| 父在子 deploy 期间继续写入 | git-fork 语义；子 `fork_txid` 在 branch 时刻固定 — **须在父 ready 后** branch（B6） |

### 4.9 health / ready-gate

| 项 | 结论 | 证据 |
|----|------|------|
| `Start` 注入 `CELLD_READY_FLEET_GATE_MS`（默认 5s） | **true** | `manager.go:126-140` |
| Health `/.well-known/celld/health` | **true** | `manager.go:358-374` |
| Health **不**验证 D1 行数 | **true** | 仅 HTTP 200 |
| orch 顺序：Start → D1 → Health | **true** | `orchestrator.go:201-219` |
| 计划须保持：D1Branch 在 Health 前完成 | 要求写入 B6/B5 timeout | 否则 B3 假阳 |

### 4.10 禁止捷径复现路径（审查猎杀）

| 捷径 | 实现者可能犯的路径 | 防护 |
|------|-------------------|------|
| 子前缀全量 snapshot | T3 为 CLI 变绿调 `import_sqlite_seed` | 计划 §4 已禁；加 B13 负例 |
| `fs::copy` 父 watch | 省略合成 restore | §2.6 已禁；B1 要求 LTX bytes 来自父 bucket |
| `execute --file` 带 seed | 已有拒绝 | 保持 |
| activate 单子 restore + 空子 LTX | **默认代码路径** | B3 |
| 仅写 `base.json` 不重建 `db.sqlite` | 省略 step 6 | B5 owner 顺序不可删 |

---

## 5. `ChainedReplicaClient` vs `calc_restore_plan`（实现提示，写入 T1）

计划 §5.1 只列了 `ltx_files`。**必须**同时规定：

```text
open_ltx_file(level, min, max):
  if child owns (level,min,max): child.open
  else: parent.open

ltx_files(level, seek):
  parent_files = parent.ltx_files(level, seek) filtered max_txid <= fork_txid
  child_files  = child.ltx_files(level, max(seek, fork_txid+1))
  merge sorted by min_txid; dedupe by (level,min,max)
```

`calc_restore_plan(chained, fork_txid)` 必须产出与「先 restore 父到 fork_txid、再叠子增量」相同的有序 `FileInfo` 列表，否则 `restore_from_plan`（`replica.rs:500+`）与 B5 wipe-watch 失败。

---

## 6. Gates that are too weak

| 原 Gate | 问题 | 加强 |
|---------|------|------|
| B2「无 MinTXID==1」 | 未定义如何列 key / 何谓全量 | 点名：`celld diagnose` 或 `aws s3 ls` 子前缀 `cells/<scope>/ltx/`，断言**无** `ltx/0/*` 且 `min==0000000000000001` 的对象；**有** `base.json` |
| B6「子前缀 < 20% seed」 | 首写后增量会涨 | 度量时点 = **branch 刚完成、子尚无 SQL**；另记录首条 INSERT 后增量 |
| B7「counter 无 D1 skip」 | 与 B3–B6 矛盾 | 拆成：B7a counter skip（保留）；B7b `d1-seed` 父子 branch e2e（必须） |
| B8「cargo test」 | 「合成 restore」无测试名 | 点名：`celld-ltx` chained plan tests + `d1_branch` integration |
| 无 import/branch 互斥负例 | 脚枪 | B13：`base.json` 存在时 `d1 import` reject |
| 无 checksum 负例 | B9 空洞 | 单测 corrupt `fork_checksum` |
| 100 MB 仅 stress | 可接受 | 但 8 MB 档 **同一不变量**（计划已写）；审查加：8 MB 档也要 wipe-watch B5 |

---

## 7. 仍采纳的计划要点（勿扩 scope）

- LTX 层 base pointer，不解析 offshoot lineage 目录。
- AD-1、per-version bucket、celld 不读 `OFFSHOOT_STORE`。
- 根 version 仍 `d1 import --file`（`D1-IMPORT-RPC.md` 冻结）。
- 独立 `d1 branch` 子命令，不污染 `execute --file`。
- 对象体积为成功标准，非本地 `du`。
- P0 不做 `d1 compact`、不做无限 fork 链。

---

## 8. 派发前检查（本 feature）

- [ ] §3 B1–B13 已合入 `celld-d1-branch.md`
- [ ] `D1-BRANCH-RPC.md` 契约存在
- [ ] §2.6 含 `seed_l0_baseline` 步骤（非仅 `seed_pos`）
- [ ] `ChainedReplicaClient` 含 `open_ltx_file` 路由
- [ ] cellp `database_id` 复制 + 父 ready 门 + skip export 接线
- [ ] e2e 用 `dev/examples/d1-seed`，父子同 `database_id`
- [ ] B5 wipe-watch 为 restore 唯一证据（非 restart）

---

*审查完成 · APPROVE-WITH-CHANGES · 待 Grok 合入 §3 修正后派发 T1–T5*
