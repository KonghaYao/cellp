# ISSUE-04: 多 ready version 时 Cron 重复触发 — 架构缓解（一期）

**优先级:** P1 · **类型:** 架构 / 安全  
**关联:** AD-7（Cron 不 branch）、AD-1 每 version 一 celld

## 问题

每个 ready preview + prod 各自 arm cron → 同一表达式 N 倍副作用（webhook、清队列等）。与「preview 环境」预期不符。

## 验收标准（一期最小可行）

- [ ] 书面决策写入 `docs/decisions.md` 草案或 AD 附录：一期策略（推荐之一：仅 prod version arm cron；或 preview 默认不 arm 除非 env 标志）
- [ ] 若选「仅 prod」：cellp orchestrator 在 `Start` celld 时传 env/vars 或 deploy 后 reconcile 仅 prod route 的 cron（需调研 celld wrangler triggers 行为）
- [ ] e2e 或脚本：两 ready version，断言 cron 仅触发一次（或 preview 不触发 — 按决策）
- [ ] `site/docs` 说明 preview 与 cron 行为

## 非目标

- Workflow branch
- 分布式 cron 选举（二期）

## 调研提示

- celld deploy / `triggers.crons`、`manager.go` CELLD_VARS
- 是否可在非 prod version 剥离 wrangler cron 段
