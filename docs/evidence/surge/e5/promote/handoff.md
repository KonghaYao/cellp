# WP-ORCH — Promote snapshot revision coupling (E5)

> **phase:** SURGE AD-15 E5 · **flag:** `CELLP_ELASTIC_RUNTIME`（默认关闭）

## 变更摘要

| 区域 | 内容 |
|------|------|
| `cellp/internal/registry/promote_sqlite.go` | `CommitProdPromote`：单事务内 CAS `prod_version_id`、激活新 prod route、`route_revision` 仅递增一次 |
| `cellp/internal/registry/store.go` | `Store` 接口增加 `CommitProdPromote` |
| `cellp/internal/gateway/invalidating_store.go` | 包装 `CommitProdPromote` 并失效 gateway 缓存 |
| `cellp/internal/orch/promote_elastic.go` | `validatePromoteTarget`（elastic 下要求 snapshot 有 routable endpoint）、`commitProdPromote`（elastic 走事务 cutover，否则 legacy CAS+SetRouteActive） |
| `cellp/internal/orch/elastic_lifecycle.go` | `maybeEnterDeployReady`：enrolled + flag=1 时在 qualification 前写入 `deploy_ready` |
| `cellp/internal/orch/orchestrator.go` | Promote 使用 `commitProdPromote`；deploy 路径在 `deploying` 后调用 `maybeEnterDeployReady` |

## 行为

- **`CELLP_ELASTIC_RUNTIME=0`**：Promote 与 deploy 与 E4 前一致（无 snapshot endpoint 门禁、无 `deploy_ready` 过渡）。
- **`CELLP_ELASTIC_RUNTIME=1`**：
  - Promote 目标须 `ready` 且 `BuildLegacyRouteSnapshot` 中含该 version 的 endpoint；`offshoot_promote` 成功后 CAS+激活 prod route 与 revision bump 同事务。
  - 已 `elastic_enrolled` 的 version 在 deploy 进入 celld qualification 前短暂处于 `deploy_ready`，验证通过后仍为 `ready`。

## 未做（后续 E5 / WP）

- rollback reserve、prewarm、qualification replica 与 elastic endpoint set 全量 cutover。
- `deploy_ready` 缩回 cold 的 reconciler 钩子。
- `docs/evidence/surge/e4/promote/<run-id>/` 压力与 chaos 时间序列。

## 验证

```bash
cd cellp && go test ./... -count=1
```

**结果：** PASS（2026-09-05）
