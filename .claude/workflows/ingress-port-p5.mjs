export const meta = {
  name: 'ingress-port-p5',
  description:
    'INGRESS-PORT-DEPLOYMENT P5: plan → P5a registry → review → fix → go test',
}

const cwd = '/Users/mino/code/remote/cellp'
const spec = `${cwd}/docs/plans/INGRESS-PORT-DEPLOYMENT.md`
const planOut = `${cwd}/docs/plans/INGRESS-PORT-P5-impl-plan.md`
const reviewOut = `${cwd}/docs/plans/INGRESS-PORT-P5-review.md`

phase('plan')

const plan = await agent(
  `你在 cellp 仓库 ${cwd}。只读，写出 P5 首期实现计划（聚焦 P5a + 最小可测闭环）。

1. 精读 ${spec} 与 ${cwd}/docs/plans/INGRESS-ROUTING.md §3
2. Read cellp/internal/registry/{sqlite.go,ingress.go,store.go}、orch/ingress.go、gateway/ingress_resolve.go
3. 写入 ${planOut}，结构：
   - P5a 范围：port_allocations 表迁移、Store 接口、AllocateIngressListenPort / ReleasePort / ReserveStablePort
   - 与 ingress_bindings.listen_port 同步规则（R-PORT-LEDGER）
   - 单元测试清单（并发分配、stable 预留冲突、release 后可复用）
   - P5b/c/d 明确 defer 到后续 PR
   - 验收命令：cd cellp && go test ./internal/registry/...
4. 不改产品代码。中文。`,
  { label: 'plan-p5', subagent_type: 'plan', cwd },
)

phase('implement')

const impl = await agent(
  `你在 ${cwd} 按 ${planOut} 实现 **P5a only**（registry 层）。

约束：
- 对齐 ${spec} §3.1 表结构与 blocking 规则 R-PORT-LEDGER、R-PORT-UNIQUE
- 扩展 registry.Store 接口 + SQLiteStore 实现；migrate 新表
- 不要改 orchestrator ready 路径（P5b）；可留 TODO 注释
- 在 ${planOut} 末尾追加「## 实施记录」
- 运行：cd cellp && go test ./internal/registry/... -count=1

遵循 AGENTS.md；不改 D1 冻结契约。`,
  { label: 'coder-p5a', subagent_type: 'coder', cwd },
)

phase('review')

const review = await agent(
  `你在 ${cwd} 审查 P5a 实现。

输入：${spec}、${planOut}、git diff cellp/internal/registry/

输出 ${reviewOut}：
- 对照 R-PORT-* 与 plan 验收（逐条 pass/fail）
- 阻塞问题 vs 建议
- 是否缺测试

可读可修明显小问题；中文。`,
  { label: 'review-p5a', subagent_type: 'verification', cwd },
)

phase('fix')

const fix = await agent(
  `你在 ${cwd} 阅读 ${reviewOut}。

- 若有 **阻塞** 项：修 registry 代码/测试直至 cd cellp && go test ./internal/registry/... 通过
- 若无阻塞：在 ${reviewOut} 追加「## Fix 阶段」说明无需改动
- 更新 ${planOut} 实施记录

不要扩大 scope 到 orchestrator（P5b）。`,
  { label: 'fix-p5a', subagent_type: 'coder', cwd },
)

phase('verify')

const verify = await agent(
  `在 ${cwd} 运行 cd cellp && go test ./... -count=1（可跳过 celld submodule 若无关）。
摘要写入 ${reviewOut} 末尾「## Verify」：exit code、失败包名。
只读为主，不提交 git。`,
  { label: 'verify-p5', subagent_type: 'verification', cwd },
)

return { plan, impl, review, fix, verify }
