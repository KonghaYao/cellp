# Phase 8 — KV / R2 / Queue branch（AD-8）

> **规格来源：** 对话 canvas「其他绑定 Branch」  
> **决策：** 本文件 + [decisions.md](../decisions.md) AD-8  
> **前置：** D1 branch 已落地（`D1-BRANCH-RPC.md` **冻结，禁止改字段**）  
> **状态：** 已落地（2026-08-30）

子 version（`parent_version_id` 且父 `ready|archived`）自动 branch：**D1（已有）+ KV + R2 + Queue**。  
**不 branch：** Worker 脚本、Workflow 实例、Cron。

**禁止：** cellp 循环 `kv get/put`、S3 CopyObject 全家桶、子 `CELLD_BUCKET` 指向父 URI、`fs::copy` watch。机制必须在 **celld 对象层**。

## 机制

| 资源 | 存储 | Branch |
|------|------|--------|
| D1 | LTX cell | 已有 `celld d1 branch` |
| KV | `__KvNamespace` 1 shard LTX + `kv/blobs-v2/` | 通用 cell-branch + **父桶链式 blob GET**；PUT 只写子桶 |
| Queue | `__Queue` LTX，消息 ≤128KB 无 blob sidecar | 同一套 cell-branch |
| R2 | `r2/<name>/` 非 cell | overlay：miss 读父、写子、DELETE 墓碑、LIST 合成 |

身份必须与父相同：`kv_namespaces[].id`、`queue` 名、`r2 bucket_name`（同 D1 `database_id`）。一层 fork：父已有 `base.json` / R2 overlay 指针 → fail compact first。`validate_parent_bucket` 与 D1 相同。

## Tracks

| ID | 范围 | 边界 |
|----|------|------|
| **P8-T0** | celld 抽出通用 cell-branch；D1 改调用 | `celld/`；现有 `v1-d1-branch.sh` 必须仍绿 |
| **P8-T1** | `celld kv branch {ns} --parent-bucket --bucket` | `celld/` |
| **P8-T2** | KV blob 子 miss → GET 父桶同 key | `celld/`；1.5 MiB 门禁 |
| **P8-T8** | `celld queue branch {name} --parent-bucket --bucket` | `celld/` |
| **P8-T5** | R2 overlay + `celld r2 branch` 写指针 | `celld/` |
| **P8-T3/T6/T9** | cellp `KvBranch` / `R2Branch` / `QueueBranch` + 从父 wrangler 复制 id | `cellp/` 禁止改 `go.mod` |
| **P8-T4/T7/T10** | e2e：父写、子读、兄弟隔离、子前缀无全量拷贝 | `e2e/` · `docs/evidence/` |

Orchestrator：Start 之后 Health 之前，顺序 D1 → 每个 KV → 每个 R2 → 每个 Queue。无声明则跳过。父非 `ready` 且非 `archived` → fail-closed。

## 契约草案（对抗后冻结）

CLI 对齐 D1：`--parent-bucket` URI，禁止 JSON 塞字节。失败 family：`KV_BRANCH_ERROR` / `QUEUE_BRANCH_ERROR` / `R2_BRANCH_ERROR`。Timeout：`max(600s, parent_snapshot_mb * 2s)`。

## 验收（最低）

- 父 put KV / enqueue / put R2；子 get/peek 到同一数据；子写入后父不变；兄弟交叉可见性 0
- KV >1 MiB：子可读且子桶无该 blob digest
- 子 S3 前缀 ≪ 朴素全量
- `cd cellp && go test ./...`
- D1 e2e 回归：`bash e2e/scripts/v1-d1-branch.sh`
