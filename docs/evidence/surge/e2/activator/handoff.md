# WP-GW-ACT — Gateway activator (E2)

> **phase:** SURGE AD-15 E2 · **flag:** `CELLP_ELASTIC_RUNTIME`（默认关闭）

## 变更摘要

| 区域 | 内容 |
|------|------|
| `cellp/internal/gateway/activator/` | 新包：请求分类、budget、本地 singleflight、`EnsureCapacity` 客户端（`RegistryEnsureClient` CAS bump `serving_desires`）、`Admit` + `503`/`Retry-After`/`X-Cellp-Reason` |
| `cellp/internal/gateway/activator_wire.go` | `tryColdActivator`：`deploy_ready` + snapshot 无 warm endpoint 时调用 activator |
| `cellp/internal/gateway/gateway.go` | ingress 路径在 legacy route 前尝试 cold activator |

## 行为（`CELLP_ELASTIC_RUNTIME=1`）

- 仅 `deploy_ready` 且 snapshot 显示 cold（无 ready endpoint）走 activator。
- `archived` → `503` `version_archived`（不隐式唤醒）。
- GET/HEAD/小 body：singleflight + `EnsureCapacity(min=1)` + 有界 poll；超时 `wake_timeout`。
- 大 body / chunked / WebSocket：触发 ensure 后快速 `503` `wake_retry`。
- `CELLP_ELASTIC_RUNTIME=0`：activator 不启用，网关行为与 E1 前一致。

## 验证

```bash
cd cellp && go test ./... -count=1
```

**结果：** PASS（2026-09-05，本机 `go test ./... -count=1 -timeout 120s`）

## 未做（后续 WP）

- WP-API：EnsureCapacity HTTP server
- WP-WIRE：完整 elastic 路由与 prod preview idle 策略
- SP-E6：校准 `wake_timeout`、body/queue 数值
