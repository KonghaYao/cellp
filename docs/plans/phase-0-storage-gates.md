# Phase 0 — 存储门禁

> **TP：** TP-V0a–V0d · TP-V0b-L（可选）· TP-DEV-2  
> **阻塞：** TP-V0a 不过 → celld 不得 prod

## Exit Criteria

- [ ] `e2e/scripts/v0a-celld-diagnose.sh` exit 0
- [ ] `e2e/scripts/v0d-offshoot-attach.sh` exit 0
- [ ] V0b：`v0b-offshoot-rustfs.sh` exit 0 **或** `docs/evidence/v0b-deferred.md`
- [ ] V0c：通过或 `docs/evidence/v0c-skip.md`
- [ ] `docs/evidence/.gitkeep` 存在

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

**Deferred 模板 `docs/evidence/v0b-deferred.md`：**

```markdown
# V0b Deferred
Date: …
Reason: …
Impact: offshoot prod uses local; RustFS for celld+artifacts only.
```

## P0-T4 — V0c（可选）

单 VIP dev/prod baseline → skip 文档即可。

## Subagent prompt 模板

```
Track P0-T{n}. Repo: /workspace. Read REVIEW.md + this file + test-plan TP-V0*.
Deliver: e2e/scripts/v0*.sh, exit 0, evidence under docs/evidence/.
Do not implement cellpd.
```
