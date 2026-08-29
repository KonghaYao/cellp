# Phase 3 — 端口级 E2E

> **TP：** TP-VE-* · TP-DEV-1 · **M1 里程碑**  
> **前置：** Phase 2 Exit

## Exit Criteria

- [x] `./e2e/scripts/run-all.sh` exit 0
- [x] `./dev/scripts/up-native.sh` + `health.sh` exit 0
- [x] **M1 达成** → 解锁 Phase 4

## Execution Status

**Audit date:** 2026-08-28 (Track P3-AUDIT)

### Script verification (plan alignment)

| Artifact | Status | Notes |
|----------|--------|-------|
| `e2e/scripts/MANIFEST` | OK | Ordered: `health-all.sh` → `ve-*.sh` (5) → `v1..v7` |
| `e2e/scripts/run-all.sh` | OK | `RUN_GATES=1` runs Phase 0 gates (`v0a`, `v0d`, `v0b`); then MANIFEST loop |
| `e2e/scripts/health-all.sh` | OK | TP-VE-1: platform `/v1/health`, gateway `/health`, celld `__celld/health` |
| `ve-cd-loop.sh` | OK | TP-VE-2 |
| `ve-promote.sh` | OK | TP-VE-3; asserts prod URL body matches promoted version |
| `ve-fail-compensate.sh` | OK | TP-VE-4 |
| `ve-destroy.sh` | OK | TP-VE-5 |
| `dev/scripts/up-native.sh` | OK | RustFS + cellpd (if built) + celld + offshoot |
| `dev/scripts/up.sh` mock gate | OK | P3-T3: cellpd first (`dev/data/cellpd` or build); mock only when `CELLP_USE_MOCK=1`; skipped when cellpd runs |

### Evidence commands

```bash
# Dev harness (exit 0 / 0)
./dev/scripts/up-native.sh
./dev/scripts/health.sh

# M1 smoke — default gates off (exit 0)
RUN_GATES=0 ./e2e/scripts/run-all.sh

# Full suite incl. Phase 0 storage gates (exit 0)
RUN_GATES=1 ./e2e/scripts/run-all.sh
```

**Results (2026-08-28 UTC):**

- `up-native.sh` → exit **0** (cellpd at `dev/data/cellpd`, RustFS `:19000`)
- `health.sh` → exit **0** (gateway, platform, celld, rustfs, registry OK)
- `RUN_GATES=0 ./e2e/scripts/run-all.sh` → exit **0** (~57s)
- `RUN_GATES=1 ./e2e/scripts/run-all.sh` → exit **0** (~57s)

### Fix applied during audit

- **`e2e/scripts/lib.sh` `wait_http_gone`:** accept HTTP **503** (inactive/draining route per cellpd gateway) in addition to 404/410. Without this, `ve-destroy.sh` failed after destroy because routes are deactivated, not deleted.

## Tracks（串行合流，非假并行）

| 顺序 | ID | 交付 |
|------|-----|------|
| 1 | **P3-T2** | `ve-*.sh` · `v*-*.sh`（Phase 2 可能已部分存在，补齐） |
| 2 | **P3-T1** | `e2e/scripts/MANIFEST` · `run-all.sh` · `health-all.sh` |
| 3 | **P3-T3** | `up-native.sh` · mock 默认关闭 |

## P3-T2 — 场景脚本（先写）

| 脚本 | TP |
|------|-----|
| `health-all.sh` | VE-1 |
| `ve-cd-loop.sh` | VE-2 |
| `ve-promote.sh` | VE-3 |
| `ve-fail-compensate.sh` | VE-4 |
| `ve-destroy.sh` | VE-5 |

**ve-promote 断言：** `curl $GATEWAY_URL/$PROJECT/` body = 新 prod

## P3-T1 — 聚合（后写）

**`e2e/scripts/MANIFEST`：** 有序脚本列表

**`run-all.sh`：**

```bash
# 1. Phase 0 gates (if RUN_GATES=1)
# 2. health-all.sh
# 3. ve-*.sh  (M1 smoke)
# 4. v1..v7   (integration depth)
while read -r s; do [[ -x "$s" ]] && "$s" || exit 1; done < e2e/scripts/MANIFEST
```

## P3-T3 — Dev harness

- `dev/scripts/up-native.sh`：compose rustfs only + cellpd + celld + offshoot
- `up.sh`：cellpd 优先；mock 仅 `CELLP_USE_MOCK=1`

**Owner：** P3-T3 修改 `up.sh` mock 分支；P1-T4 已添加 cellpd 路径

## M1 定义

TP-V0a + TP-API-1..7 + TP-SEC-1,2,5 + **TP-VE-ALL** = `[x]`

（TP-V1–V7 可在 M2 完成；M1 不要求）
