# v1 收尾计划 — V0b + Phase 6

> **状态：** 历史收尾记录（2026-08-29 已完成）。**执行与门禁以** [test-plan-phase6.md](../test-plan-phase6.md) **与** [../../stress/phase6/README.md](../../stress/phase6/README.md) **为准。**  
> **日期：** 2026-08-29  
> **Goal：** 完成 README v1 交付范围中两项 deferred/后续项的**可验收闭环**

---

## 1. 排查结论

### V0b — offshoot prod × RustFS

| 项 | 结论 |
|----|------|
| **阻塞性质** | 政策 deferred，非已知 RustFS 故障 |
| **已有证据** | V0b-L 大库 fork 在 RustFS 已通过（`offshoot-branch-scale-report-rustfs.md`） |
| **缺口** | 全序列（并行 fork + promote + destroy）无 `v0b-*.log` |
| **脚本** | `e2e/scripts/v0b-offshoot-rustfs.sh` 已就绪 |
| **修复** | 移除 `v0b-deferred.md`；`lib.sh` S3 端口默认对齐 `19000` |

**验收：** `v0d` + `v0b` exit 0，产出 `docs/evidence/v0b-*.log` + export DB。

### Phase 6 — 千万扩展

| 项 | 结论 |
|----|------|
| **产品约束** | 不做 PostgreSQL、不做多租户/RBAC |
| **在 scope** | **仅 6A**（SQLite 分页 + Gateway cache + GC + Dashboard + 压测基线） |
| **OUT OF SCOPE** | 6B–6F（百万 project、50k–500k RPS、deploy storm、PG） |
| **6A 代码** | TP6-A1..A4 ✅ |
| **6A 缺口** | TP6-A5：ListProjects @10k p99 ~238–262ms（gate 200ms）；10×10k versions 未跑；100k Gateway RPS 未跑（需 infra） |

**诚实交付：** 6A **实现完成** + SQLite 天花板 **文档化豁免**；不宣称千万 sign-off。

---

## 2. 实施轨道

### Track V0b（P0 — 今日）

1. [x] 修复 `e2e/scripts/lib.sh` 默认 S3 端口
2. [x] 删除 `docs/evidence/v0b-deferred.md`
3. [x] 运行 `v0d-offshoot-attach.sh` → `v0b-offshoot-rustfs.sh`
4. [x] 新增 `docs/evidence/v0b-pass-report.md`
5. [x] 更新 `README.md` · `decisions.md` · `test-plan.md` · `phase-0-storage-gates.md`

### Track 6A-close（P1）

1. [x] 复测 `registry-bench.sh` @11k projects — ListProjects p99 **7ms** ✅
2. [x] `registry-size-report.sh` — 5.07 MB / 11k projects
3. [x] `gateway-scale.sh -short` — dev 50 RPS p99 105ms
4. [x] 更新 `scale-report-6A.md` · `test-plan-phase6.md` · `phase-6-scale-10m-master.md`

### Track docs（P1）

- README v1 表：V0b ✅ · Phase 6 → **6A complete (SQLite scope)**
- `decisions.md` AD-4 prod tier 更新

---

## 3. 不在本 Goal 范围

- PostgreSQL / 多租户（6B）
- 50k–500k Gateway RPS（6E–6F）
- `deploy-storm.sh` / `seed-orgs.sh`
- 修改 DESIGN 一期「≤5 ready versions」上限

---

## 4. 验收门禁

| ID | 命令 | 通过 |
|----|------|------|
| **V0b** | `e2e/scripts/v0b-offshoot-rustfs.sh` | exit 0，无 deferred 文件 |
| **TP6-A5-bench** | `stress/phase6/registry-bench.sh` | 10×10k versions p99 <100ms |
| **TP6-A5-report** | `stress/phase6/registry-size-report.sh` | 输出 registry 体量报告 |
| **TP6-A5-gateway-dev** | `stress/phase6/gateway-scale.sh -short` | 记录 dev RPS 基线到 scale-env.json |
