# Phase 0 — 存储门禁

> **TP：** TP-V0a–V0d · TP-V0b-L（可选）· TP-DEV-2  
> **阻塞：** TP-V0a 不过 → celld 不得 prod

## Exit Criteria

- [x] `e2e/scripts/v0a-celld-diagnose.sh` exit 0
- [x] `e2e/scripts/v0d-offshoot-attach.sh` exit 0
- [x] V0b：`v0b-offshoot-rustfs.sh` exit 0（[v0b-pass-report.md](../evidence/v0b-pass-report.md) · 2026-08-29）
- [x] V0c：通过或 `docs/evidence/v0c-skip.md`
- [x] `docs/evidence/.gitkeep` 存在

## Parallel Tracks

| Track | ID | 并行 | Gate | 交付 |
|-------|-----|------|------|------|
| Evidence bootstrap | **P0-T0** | 最先 | 无 | `docs/evidence/.gitkeep` |
| celld 探针 | **P0-T1** | ∥ T2,T4 | T0, RustFS | `e2e/scripts/v0a-celld-diagnose.sh` |
| offshoot attach | **P0-T2** | ∥ T1,T4 | T0, RustFS | `e2e/scripts/v0d-offshoot-attach.sh` |
| offshoot 全序列 | **P0-T3** | **串行 T2 后** | T2 pass | `e2e/scripts/v0b-offshoot-rustfs.sh` |
| 多节点 | **P0-T4** | ∥ T1,T2 | 多节点 env | `v0c-*.sh` 或 skip 文档 |

## P0-T0 — Evidence bootstrap

```bash
mkdir -p docs/evidence
touch docs/evidence/.gitkeep
```

## P0-T1 — celld diagnose

```bash
# e2e/scripts/v0a-celld-diagnose.sh
source dev/.env
celld diagnose --bucket s3://cellp-celld --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" | tee docs/evidence/v0a.log
grep -q 'ok bucket conditional write' docs/evidence/v0a.log
```

**Subagent：** 仅实现脚本；不改 cellpd。

## P0-T2 — offshoot attach (V0d)

```bash
# e2e/scripts/v0d-offshoot-attach.sh
export OFFSHOOT_STORE=s3://cellp-offshoot
# init/attach — exit 0
```

## P0-T3 — offshoot branch 全序列 (V0b)

**依赖 P0-T2。** 序列见 test-plan TP-V0b。可选 TP-V0b-L 大库路径。

**✅ PASS（2026-08-29）：** `v0d-offshoot-attach.sh` → `v0b-offshoot-rustfs.sh` exit 0；证据 [v0b-pass-report.md](../evidence/v0b-pass-report.md)。`v0b-deferred.md` 已删除；失败时不再 soft-pass。

## P0-T4 — V0c（可选）

单 VIP dev/prod baseline → skip 文档即可。

## Subagent prompt 模板

```
Track P0-T{n}. Repo: /workspace. Read REVIEW.md + this file + test-plan TP-V0*.
Deliver: e2e/scripts/v0*.sh, exit 0, evidence under docs/evidence/.
Do not implement cellpd.
```
