# SURGE AD-15 交付摘要（E2–E5）

> **flag：** `CELLP_ELASTIC_RUNTIME`（默认 `0`）· 合并自 2026-09-05 各 WP handoff · 细节以代码与 [SURGE-AD15-HANDOFF](../handoff/SURGE-AD15-HANDOFF.md) 为准  
> **后续阶段：** 由 [scripts/surge-phased-delivery.mjs](../../scripts/surge-phased-delivery.mjs) 追加行到「## 自动化追加」，勿再写 `evidence/surge/**/handoff.md`。

| 阶段 | WP | 主要变更 | 验证 | 显式未做 |
|------|-----|----------|------|----------|
| **E2** | GW-ACT Gateway activator | `gateway/activator/`：分类、budget、singleflight、`EnsureCapacity`（CAS `serving_desires`）、`503`/`Retry-After`/`X-Cellp-Reason`；`tryColdActivator` 于 `deploy_ready` 且无 warm endpoint | `go test ./...` PASS 2026-09-05 | EnsureCapacity HTTP server；完整 elastic 路由；SP-E6 数值校准 |
| **E3** | RT Runtime / Node Agent 桩 | `ServingStore` + `runtime_nodes`；`elastic/agent/`：`StartReplica`/`ProbeReplica`/`StopReplica` 桩、命令校验、`enabled=false` → `elastic_disabled` | PASS 2026-09-05 | HTTP+mTLS listener；Local/RemoteBackend；真实进程生命周期 |
| **E4** | SCALE Autoscaler stub | `elastic/autoscaler/`：`Tick`/`Run`（`CELLP_AUTOSCALER_INTERVAL` 默认 30s）；`ListElasticServingPolicies`；background policy guard；stub **只读** desire，输出 `VersionGap` | PASS 2026-09-05 | 信号驱动扩缩；scheduler 写 desire；SP-E3 多 replica 证明 |
| **E5** | ORCH Promote / deploy_ready | `CommitProdPromote` 单事务 CAS+route+`route_revision`；elastic promote 要求 snapshot 可路由 endpoint；`maybeEnterDeployReady`（enrolled + flag=1） | PASS 2026-09-05 | rollback reserve/prewarm；`deploy_ready` 缩 cold reconciler；e4 promote 压力证据 |

## 行为要点（flag=1）

- **E2：** 仅 `deploy_ready` 且 cold 走 activator；`archived` → `503 version_archived`；大 body/WS → `wake_retry`。
- **E3：** registry 写 `starting`  replica，不启 celld 进程。
- **E4：** autoscaler 设计上为 `serving_desires` 唯一写者；本阶段 stub 不写 CAS（activator 仍可 bump ensure）。
- **E5：** promote 与 `deploy_ready` 过渡；flag=0 时与 E4 前 promote/deploy 一致。

## 自动化追加

| 阶段 | 日期 | Commit | 验证 | 备注 |
|------|------|--------|------|------|
| *(workflow 运行后由 agent 填行)* | | | | |
