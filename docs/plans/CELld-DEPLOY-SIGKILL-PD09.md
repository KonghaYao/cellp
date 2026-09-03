# celld deploy SIGKILL（PD-09）根因与修复

> **状态：** cellp 已合入 deploy 槽位（2026-09-03）  
> **关联：** [platform-defects-log.md](../platform-defects-log.md) PD-20260903-09

## 根因

| 因素 | 说明 |
|------|------|
| AD-1 fleet | 每个 ready version 一个 `celld --listen` 常驻进程 |
| deploy 子进程 | cellpd orch 执行 `celld deploy`（读 artifact、可选 esbuild、写 `s3://cellp-celld/{project}/{version}`） |
| 失败形态 | `deploy: celld deploy: signal: killed:`（**非** HTTP ctx cancel；`WithoutCancel` 已用于 deploy/diagnose） |
| 成功对比 | S29 **v9**、同目录手工 deploy：fleet 压力低时 **ready** |

## 代码修复

- `cellp/internal/runtime/deploy_limit.go` — 进程级 semaphore
- `manager.go` — `Deploy` 在槽位内调用 `runCelldDeploy`；SIGKILL 重试 1 次
- 环境变量：`CELLP_CELLD_DEPLOY_CONCURRENCY`（默认 `1`）

## 验证

```bash
cd cellp && go test ./internal/runtime/... -count=1
go build -o dev/data/cellpd ./cmd/cellpd   # 从 cellp 子目录或 repo 根按现有脚本
# 重启 cellpd 使二进制生效
./dev/scripts/health.sh
SUPPORT_SKIP_GIT_FETCH=1 npm_config_registry=https://registry.npmmirror.com \
  SUPPORT_VERSION=v7 SUPPORT_POLL_SECS=300 ./dev/scripts/deploy-support-app.sh S27
# 或 S28（deploy 通过后仍可能有 celld load/health 类 celld 缺陷，与 PD-09 无关）
```

## S28 备注

S28 历史失败含 `.assetsignore`、**health timeout**（`process.stdin` unenv）— 需在 deploy **ready** 后单独查 celld runtime，不属 PD-09。
