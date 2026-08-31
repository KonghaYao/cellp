# ISSUE-01: Promote 必须在 offshoot promote 成功后才允许 CAS 切 prod

**优先级:** P0 · **类型:** bug / 一致性  
**关联:** AD-5、`cellp/internal/orch/orchestrator.go` Promote saga

## 问题

`offshoot_promote` 失败时仅 `log.Printf` warn，仍执行 `SetProdVersionCAS` 与 `activate_prod_route`。用户看到 prod 已切到新 version，但 offshoot `main` 可能未对齐 → **路由新、数据旧**。

## 验收标准

- [ ] `branch.Promote` 失败时：**不**执行 CAS；**运行**已有 compensation（恢复 old route active）
- [ ] API `POST …/promote` 返回明确错误（非 200），body 含可区分错误码/信息
- [ ] 新增 e2e：注入 offshoot promote 失败（或 `CELLP_STRICT` / mock），断言 prod_version_id 不变、prod URL 仍指向旧 version
- [ ] `docs/decisions.md` 或 AD-5 证据节补充「offshoot promote 为 promote 硬门禁」一句（若行为与原文冲突则更新 AD-5 表述）
- [ ] `cd cellp && go test ./...` 通过；相关 e2e 脚本可单独跑通

## 非目标

- 实现 offshoot promote 的逆向补偿（仍可无 reverse promote）
- 合并 celld bucket 数据（另 issue）

## 调研提示

- `branch/manager.go` Promote、`hasOffshoot()` no-op
- `e2e/scripts/v4-promote-cutover.sh`、`v5-saga-compensate.sh`（仅 deploy）
- `stress/scripts/chaos-offshoot-fail.sh` 是否可复用
