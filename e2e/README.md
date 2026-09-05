# cellp 端口级 E2E

后端 `cellpd` 完成后的 **前端开工门禁**（[VALIDATION.md §VE](../VALIDATION.md)）。

## 原则

- **只打端口**，不用浏览器、不测 Dashboard
- 依赖 `dev/scripts/up.sh` 或 CI 等价栈
- exit 0 = VE 通过

## 端口

| 端口 | 服务 |
|------|------|
| 8787 | cellpd Gateway（内置 reverse proxy） |
| 8790 | cellpd API |
| 8792 | celld |

## 脚本

```
e2e/scripts/
  health-all.sh      # 各端口探活
  run-all.sh         # MANIFEST 顺序执行全部验收
  v1-d1-seed.sh              # offshoot export → celld d1 import → Worker /count
  v1-d1-branch.sh            # parent import → child d1 branch → isolation + B5
  v1-d1-branch-multi-100mb.sh  # 100 MB parent + 3 sibling branches (manual; heavy)
  ve-cd-loop.sh              # POST version → poll ready → curl gateway
  ve-promote.sh              # promote → 验证 prod 切换
  v4b-promote-offshoot-fail.sh  # promote 在 offshoot 失败时不切 prod
  v9-kv.sh                   # KV put/get + sibling isolation (:8790)
  v10-queue.sh               # Queue info/peek/purge (producer-only)
  v11-workflow-cron.sh       # Workflow instances ≠ 500 + cron bindings
```

### TP-V1 — D1 binary import (`v1-d1-seed.sh`)

1. Offshoot 分支写入 `entries` 表（与 `celld/examples/d1` Worker schema 一致），export 为 `seed.db`
2. `celld deploy` D1 fixture，再 `celld d1 import guestbook --file seed.db`（`DATABASE` 为 wrangler `database_name`，**不是** export 路径）
3. 断言 seed 行数 = `celld d1 execute` 查询行数 = Worker `GET /count`

契约见 [D1-IMPORT-RPC.md](../docs/plans/D1-IMPORT-RPC.md)。

### TP-D1-BRANCH — D1 branch (`v1-d1-branch.sh`)

1. 父 version：offshoot seed → `d1 import`（根路径）
2. 子 version：cellp orchestrator 跳过 export，走 `celld d1 branch`
3. 断言 B3/B4/B5：行数继承、父隔离、kill + wipe-watch restore

契约见 [D1-BRANCH-RPC.md](../docs/plans/D1-BRANCH-RPC.md)。证据：`docs/evidence/d1-branch-e2e-report.md`。

### TP-D1-BRANCH-MULTI — 100 MB 三分支（`v1-d1-branch-multi-100mb.sh`）

**不在** `run-all.sh` 默认路径（耗时 ~30s+）。手动跑：

```bash
D1_BRANCH_MULTI_SIZE_MB=100 D1_BRANCH_MULTI_COUNT=3 \
  bash e2e/scripts/v1-d1-branch-multi-100mb.sh
```

证据：`docs/evidence/d1-branch-multi-100mb.json`。压测档见 `stress/phase6/d1-branch-scale.sh`。

### Bindings — TP-V9 / TP-V10 / TP-V11

端口级 curl，**不**用浏览器（Dashboard 是 T4 / TP-UI-7..12）。`run-all.sh` 在 `v7-external-ci.sh` 之后执行这三条（见 `e2e/scripts/MANIFEST`）。

| ID | 脚本 | 通过 |
|----|------|------|
| **TP-V9** | `e2e/scripts/v9-kv.sh` | `celld/examples/kv` deploy；`:8790` PUT/GET；子 version 同 key **404** |
| **TP-V10** | `e2e/scripts/v10-queue.sh` | `dev/examples/queue` producer-only（celld：consumer 不能 `export fetch`）；info/peek；purge 无 `force` → 400 |
| **TP-V11** | `e2e/scripts/v11-workflow-cron.sh` | bindings 含 workflow + cron；`GET …/workflows/{name}/instances` **不 500** |

```bash
# 日常只跑受影响项（名称可省略 .sh）
./e2e/scripts/run-all.sh --only v9-kv,v12-kv-branch

# 调试时保留已有 v-e2e-*；确认状态不会污染断言后再用
./e2e/scripts/run-all.sh --only health-all --skip-cleanup

# 查看可选脚本
./e2e/scripts/run-all.sh --list

# 完整 MANIFEST（沿用既有默认，不含 Phase 0）
RUN_GATES=0 ./e2e/scripts/run-all.sh

# 存储准入/发布门禁（包含 celld diagnose + offshoot RustFS）
RUN_GATES=1 ./e2e/scripts/run-all.sh
```

`--only` 仍按 gate/MANIFEST 顺序执行，但它只是开发快环，不能声明 TP-VE-ALL/M2 全绿。runner 会打印每个脚本和整套测试的耗时，便于发现慢项。环境变量等价形式：`E2E_ONLY=v9-kv,v12-kv-branch`、`E2E_SKIP_CLEANUP=1`。

栈未起来时脚本 **SKIP**（exit 0 + 说明）。健康路径：`/.well-known/celld/health`（`health-all.sh`）。

验收标准见 [VALIDATION.md](../VALIDATION.md#ve--端口级-e2e后端-p0-完成门禁--前端开工前必过)。
