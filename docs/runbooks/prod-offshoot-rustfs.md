# Runbook：生产 offshoot × RustFS（AD-4 · V0b）

> **门禁：** TP-V0b PASS — [evidence/v0b-pass-report.md](../evidence/v0b-pass-report.md)（2026-08-29）  
> **决策：** [decisions.md](../decisions.md) AD-4 · 存储 tier

M2 可在 **local offshoot** tier 达成。**生产数据面**若要用 RustFS 持久化 offshoot（`s3://cellp-offshoot`），须完成本 runbook。

---

## 1. 前置

| 项 | 要求 |
|----|------|
| RustFS | 与 Phase 0 同 tag；VIP 或单节点 endpoint |
| `offshoot` CLI | `~/go/bin/offshoot` 在 PATH |
| celld | `celld diagnose` 存储探针通过（TP-V0a） |
| 证据 | `v0d-offshoot-attach.sh` + `v0b-offshoot-rustfs.sh` exit 0 |

---

## 2. 验证序列（上线前必跑）

```bash
export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

./e2e/scripts/v0d-offshoot-attach.sh
./e2e/scripts/v0b-offshoot-rustfs.sh
```

通过标准见 [v0b-pass-report.md](../evidence/v0b-pass-report.md)：init → seed → checkpoint → parallel fork → export → promote → destroy。

大库 fork（可选）：`OB_SUITE=v0bl ./stress/phase6/offshoot-branch-scale.sh`

---

## 3. 生产配置要点

| 配置 | dev 典型值 | prod 说明 |
|------|------------|-----------|
| offshoot store URI | `s3://cellp-offshoot/e2e` | 独立 bucket/prefix，与制品桶分离 |
| S3 endpoint | `http://127.0.0.1:19000` | RustFS 内网 VIP |
| `offshoot_tier` | `local` \| `rustfs` | 压测/报告**必须**注明 |

cellpd / orchestrator 使用 offshoot export 路径时，确保：

- RustFS 条件写探针已通过（V0a）
- fork 并行无 CAS 冲突（V0b 已验证）

**禁止：** 手改 `dev/data/` sqlite；用 `./dev/scripts/reset.sh` 重置 dev。

---

## 4. 与 M2 / 压测关系

| 里程碑 | offshoot tier |
|--------|---------------|
| **M2** | local 即可 |
| **M3** test-plan-phase2 | 报告注明 `offshoot_tier` |
| **生产 sign-off** | **rustfs**（本 runbook + V0b 证据） |

---

## 5. 故障排查

| 症状 | 检查 |
|------|------|
| fork CAS conflict | 并行 fork 同一 parent；见 V0b 日志 |
| export 空库 | checkpoint 前 quiesce（TP-V2） |
| attach 失败 | `v0d-offshoot-attach.sh` · RustFS 凭证 |

---

## 6. 相关

- [rollback.md](./rollback.md) · [observability.md](../observability.md)
- [phase-0-storage-gates.md](../plans/phase-0-storage-gates.md)
