# WP-RT — Runtime node store & Node Agent scaffolding (E3)

> **phase:** SURGE AD-15 E3 · **flag:** `CELLP_ELASTIC_RUNTIME`（默认关闭）

## 变更摘要

| 区域 | 内容 |
|------|------|
| `cellp/internal/registry/serving_store.go` | `ServingStore` 增加 `UpsertRuntimeNode` / `GetRuntimeNode` / `ListRuntimeNodes` |
| `cellp/internal/registry/serving_sqlite.go` | `runtime_nodes` 表读写实现 |
| `cellp/internal/registry/store.go` | `Store` 接口同步上述方法 |
| `cellp/internal/elastic/agent/` | 新包：`Handler`（`StartReplica` / `ProbeReplica` / `StopReplica` 桩）、`RegistryStores` 适配器、`NewFromRegistry` |

## 行为（`CELLP_ELASTIC_RUNTIME=1` 且 Handler `enabled=true`）

- 命令校验 `contract.ValidateCommandScope`（generation、lease、nonce）。
- 节点须已登记且未 cordon；lease 过期拒绝。
- Start：写入 `runtime_replicas` 为 `starting`（不启动 celld 进程）。
- Probe / Stop：按 replica_id + generation 读写 registry 事实。
- `enabled=false`：所有命令返回 `elastic_disabled`（fail-closed）。

## 未做（后续 E3+）

- HTTP+mTLS Node Agent listener 与证书/replay（E3 安全契约冻结后）。
- LocalBackend / RemoteBackend 与现有 `runtime.Manager` 包装。
- Drain / List 命令与真实进程生命周期。

## 验证

```bash
cd cellp && go test ./... -count=1
```

**结果：** PASS（2026-09-05）
