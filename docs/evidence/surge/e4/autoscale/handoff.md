# WP-SCALE — Autoscaler loop stub (E4)

> **phase:** SURGE AD-15 E4 · **flag:** `CELLP_ELASTIC_RUNTIME`（默认关闭）

## 变更摘要

| 区域 | 内容 |
|------|------|
| `cellp/internal/elastic/autoscaler/` | 新包：`Loop.Tick` 读取 enrolled policies / desires，对比 `desired` vs `ready` replica 数；`Run`/`Start` 后台 ticker（`CELLP_AUTOSCALER_INTERVAL`，默认 30s）；flag=0 时 `Tick` 直接 `Skipped` |
| `cellp/internal/elastic/contract/policy_background.go` | `ValidateServingPolicyBackground`：`resident_required` 强制 `min>=1`；未 SP-E3 证明时 `max<=1` |
| `cellp/internal/registry/serving_sqlite.go` | `ListElasticServingPolicies`；`UpsertServingPolicy` 在 `elastic_enrolled` 时调用 background guard |
| `cellp/internal/registry/serving_store.go` / `store.go` | 接口增加 `ListElasticServingPolicies` |
| `cellp/internal/serve/serve.go` | `autoscaler.Start(ctx, baseStore, …)`（仅启动 ticker；每 tick 仍受 flag 门控） |

## 行为（`CELLP_ELASTIC_RUNTIME=1`）

- Autoscaler 为 **唯一** 设计上的 `serving_desires` 写者（本阶段 stub **只读** desire，不写 CAS；activator 仍可 bump ensure）。
- `Tick` 输出 `VersionGap`（`desired - ready`），供后续 scheduler/reconciler 消费。
- 违反 background guard 的 policy 在 tick 中跳过并打日志。
- `CELLP_ELASTIC_RUNTIME=0`：与 E3 前一致，autoscaler 不比较、不写 desire。

## 未做（后续 WP-SCALE / SP）

- 并发/背压/latency 信号与快扩慢缩、stabilization、scale-to-zero 算法。
- Scheduler 根据 gap 启动/停止 replica；`CompareAndSetDesired` 由 autoscaler 写 scale reason。
- SP-E3 通过后 `MultiReplicaBackgroundProven` 配置面。
- `docs/evidence/surge/e4/scale/<run-id>/` 压力时间序列。

## 验证

```bash
cd cellp && go test ./... -count=1
```

**结果：** PASS（2026-09-05）
