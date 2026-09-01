export const meta = {
  name: 'ingress-port-p5b',
  description:
    'INGRESS P5b: projects columns, orchestrator Attach/Detach, port preview_url, e2e',
}

const cwd = '/Users/mino/code/remote/cellp'
const spec = `${cwd}/docs/plans/INGRESS-PORT-DEPLOYMENT.md`
const plan = `${cwd}/docs/plans/INGRESS-PORT-P5-impl-plan.md`
const p5bPlan = `${cwd}/docs/plans/INGRESS-PORT-P5b-impl-plan.md`
const reviewOut = `${cwd}/docs/plans/INGRESS-PORT-P5b-review.md`

phase('plan')

const planResult = await agent(
  `你在 ${cwd}。只读，写 P5b 实现计划 ${p5bPlan}。

依据：${spec} §5–§6、${plan} §7 P5b 行、${cwd}/cellp/internal/orch/ingress.go、registry/port_ledger.go。

计划须含：
- projects 迁移：ingress_tier_b、prod_listen_port（对齐设计 §3.2）
- config：CELLP_INGRESS_TIER_B 解析 dedicated_port | prod_port
- orchestrator：ready 时 dedicated_port 下 preview Attach(ephemeral)；prod Attach(stable) 或 Reserve→prod binding；archive Detach+Release ephemeral；promote **不改** prod listen_port（R-PROD-PORT-STABLE）
- preview_url/prod_url 含 127.0.0.1:port 当 listen 模式
- ready 失败 rollback ephemeral 台账
- e2e：最小脚本或 env 说明（可 defer 到 fix 若超时）
- 验收：cd cellp && go test ./...；点名 e2e 脚本

不写代码。中文。`,
  { label: 'plan-p5b', subagent_type: 'plan', cwd },
)

phase('implement')

const impl = await agent(
  `你在 ${cwd} 按 ${p5bPlan} 实现 **P5b**（registry 已 land P5a）。

约束：
- 禁止绕过 AttachIngressListenPort 写 listen_port（生产路径）
- 不改 Gateway ReconcileListeners（P5c）
- OpenAPI 字段可最小 PATCH project prod_listen_port（若 plan 要求）
- ${p5bPlan} 末尾追加「## 实施记录」
- 运行 cd cellp && go test ./... -count=1

AGENTS.md · 不改 D1 冻结 RPC。`,
  { label: 'coder-p5b', subagent_type: 'coder', cwd },
)

phase('review')

const review = await agent(
  `审查 P5b：${spec} R-PROD-PORT-STABLE、R-ARCHIVE-TEARDOWN、${p5bPlan}。

输出 ${reviewOut}（pass/fail、阻塞、测试缺口）。可读可修小 bug。中文。`,
  { label: 'review-p5b', subagent_type: 'verification', cwd },
)

phase('fix')

const fix = await agent(
  `读 ${reviewOut}，修 P5b 阻塞项；go test ./internal/orch/... ./internal/registry/... 必须通过。
无阻塞则记录「Fix 阶段：无」。不启动 P5c Gateway listener 大改。`,
  { label: 'fix-p5b', subagent_type: 'coder', cwd },
)

phase('verify')

const verify = await agent(
  `在 ${cwd}：
1. cd cellp && go test ./... -count=1
2. 若有 e2e 脚本新增，bash -n 并说明是否需 ./dev/scripts/up.sh
摘要写入 ${reviewOut}「## Verify」。不 git commit。`,
  { label: 'verify-p5b', subagent_type: 'verification', cwd },
)

return { planResult, impl, review, fix, verify }
