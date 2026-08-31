# cellp 设计对抗审查记录

> Subagent 审查日期：2026-08-27 · 修正后版本：**v2 可执行**

## 审查来源

| 审查 | 焦点 | Agent |
|------|------|-------|
| R1 技术可行性 | celld T3 · Gateway prod · saga · SQLite | bc-791965bf |
| R2 验收完整性 | TP 映射 · 安全 · DoD · 自动化 | bc-e5ce676d |
| R3 派发可执行性 | 并行 track · 合并冲突 · 契约 | bc-53451d20 |

## 已采纳的关键架构决策

### AD-1 — 多 Version 路由：每 Version 独立 celld upstream（非 celld 内置 path 路由）

**问题：** celld 官方限制「1 fleet = 1 deploy」；Gateway 仅靠 path 无法同时服务不同 artifact 的 version。

**决策（Option A）：**

| 项 | 一期实现 |
|----|----------|
| celld | **每 ready version 一个 upstream 端口**（`127.0.0.1:{8792+N}`）或独立 `CELLD_BUCKET=s3://cellp-celld/{project}/{version}` + 独立 celld 子进程 |
| Registry `routes` | `upstream_host` + `upstream_port` per version |
| Gateway | `/{project}/{version}/*` → 查 Registry route → reverse proxy |
| 资源上限 | ≤5 ready versions / project（DESIGN 一期） |

**Spike 门禁：** Phase 2 开工前完成 `docs/evidence/celld-multi-fleet-spike.md`（2 version 不同 counter 值同时 200）。

### AD-2 — Gateway prod 路径为一期必交付

`GET|POST|… /{project}/*` → `prod_version_id` 的 upstream。**Phase 1 P1-T3 Exit Criteria**，非可选。

### AD-3 — Orchestrator job 持久化

SQLite 表 `jobs` + lease；cellpd 重启可恢复。Phase 1 schema 预留，Phase 2 T4 实现 pickup。

### AD-4 — 部署层级（offshoot store）

| Tier | offshoot | 门禁 |
|------|----------|------|
| Dev / 功能验收 | local dir | test-plan 可全绿 |
| Prod 数据面 | RustFS | **TP-V0b 必须** |

test-plan 全绿 ≠ prod 数据面就绪；压测报告须注明 tier。

### AD-5 — Promote saga（自动补偿，非 manual）

```
forward:  validate → drain_old → deactivate_old_route → offshoot_promote → CAS_prod → activate_prod_route
compensate: 任一步失败按逆序 idempotent 回滚
```

`offshoot_promote` 失败则中止 saga（不 CAS）；`SetProdVersionCAS(project, expected, new)` 为 Registry 必交付。

## 修正清单（v1 → v2）

| ID | 严重度 | 修正 |
|----|--------|------|
| C1 | CRITICAL | AD-1 写入 phase-2 · test-plan TP-V3 |
| C2 | CRITICAL | AD-2 写入 phase-1 P1-T3 |
| C3 | CRITICAL | AD-3 jobs 表 + phase-1/2 |
| M1 | MAJOR | AD-5 promote saga |
| M2 | MAJOR | SQLite busy_timeout=60s + 重试 + TP2-C5 脚本 |
| M3 | MAJOR | AD-4 部署 tier |
| M4 | MAJOR | Gateway drain：`route.active=false` → 503/410 |
| QA | GAP | 新增 TP-SEC-* · TP-API-5..7 · TP-VE-5 · TP-UI-5/6 |
| EXEC | BLOCKER | go.mod 根目录 · evidence/ · OpenAPI 路径 · script ownership |
| EXEC | BLOCKER | P0 T3 串行于 T2；P3/P4 去假并行 |
| EXEC | MINOR | 移除 calendar 估计；VALIDATION 降级为索引 |

## 仍待 Spike（不阻塞 Phase 1，阻塞 prod offshoot RustFS）

- [x] celld 多进程/多 bucket 资源占用与启动时序 → `docs/evidence/celld-multi-fleet-spike.md`
- [x] D1 binary import → `docs/evidence/d1-import-scale-report.md` · 契约 `D1-IMPORT-RPC.md`
- [x] D1 branch（子 version 共享父 LTX）→ `docs/evidence/d1-branch-e2e-report.md` · 契约 `D1-BRANCH-RPC.md`
- [x] V0b RustFS offshoot prod 路径 → **PASS**（`docs/evidence/v0b-pass-report.md` · 2026-08-29）；`v0b-deferred.md` 已删除

## 派发前检查（每次开 subagent 前）

- [ ] 目标 track 的 **Gate** 已满足（见各 phase Parallel Tracks）
- [ ] 必读 AD-1..5 + 对应 phase 文件 + test-plan TP 列表
- [ ] `cd cellp && go test …`（module root = `cellp/`）
- [ ] 不得修改 `go.mod`（除非 track 明确为 T4/deps owner）

---

*v2 · 审查修正完成 · 可派发*
