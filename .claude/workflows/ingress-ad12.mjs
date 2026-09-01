export const meta = {
  name: 'ingress-ad12',
  description: 'AD-12 Host ingress: parallel plan/implement/review/fix/test/commit',
}

const cwd = '/Users/mino/code/remote/cellp'
const spec = `${cwd}/docs/plans/INGRESS-ROUTING.md`
const ad = `${cwd}/docs/decisions.md AD-12`

const shared = `
必读：${spec} 全文 + ${ad}。
遵守 AGENTS.md：不改冻结 D1 RPC；Dashboard 不直连 celld。
默认 CELLP_INGRESS_TIER_B=host（单 Gateway 口 + Host 路由），P0 范围。
实现后跑：cd ${cwd}/cellp && go test ./...
`

phase('Plan-parallel')
const [planReg, planGw, planOrch, planE2e] = await parallel([
  () =>
    agent(
      `${shared} 只读规划：ingress_bindings schema、sqlite 迁移、store API、openapi 字段。输出：文件清单 + 接口签名 + 测试点。不写代码。`,
      { label: 'plan-registry', subagent_type: 'plan', cwd },
    ),
  () =>
    agent(
      `${shared} 只读规划：gateway 废弃 path 路由、Host lookup、Forwarded 头、proxy Host=synthetic_host。输出：改哪些文件、伪代码、单测矩阵。不写代码。`,
      { label: 'plan-gateway', subagent_type: 'plan', cwd },
    ),
  () =>
    agent(
      `${shared} 只读规划：orchestrator ready 写 binding、preview_url、PUBLIC_BASE_URL env、CELld trust forwarded、VerifyGatewayRoute 改用 preview_url。输出：调用链 + 文件。不写代码。`,
      { label: 'plan-orch', subagent_type: 'plan', cwd },
    ),
  () =>
    agent(
      `${shared} 只读规划：e2e lib gateway URL、过渡 path deprecated、新 host smoke 脚本名。输出：脚本改动列表。不写代码。`,
      { label: 'plan-e2e', subagent_type: 'plan', cwd },
    ),
])

phase('Implement-parallel')
const [implReg, implGw, implOrch, implCfg] = await parallel([
  () =>
    agent(
      `${shared}
依据 plan-registry 与规范 §3：实现 registry ingress_bindings（sqlite + store 方法 + 单测）。
只改 cellp/internal/registry/** 与必要 migration 常量；不要改 gateway.go。
完成后 go test ./internal/registry/...`,
      { label: 'impl-registry', subagent_type: 'coder', cwd, model: 'sonnet' },
    ),
  () =>
    agent(
      `${shared}
依据 plan-gateway：改 cellp/internal/gateway/* 实现 Host 路由、正确 Forwarded、path 业务路由移除或 deprecated 开关 INGRESS_HOST_ONLY=1。
只改 gateway 包与 gateway 测试；registry 调用用 store 已有/新增接口。
完成后 go test ./internal/gateway/...`,
      { label: 'impl-gateway', subagent_type: 'coder', cwd, model: 'sonnet' },
    ),
  () =>
    agent(
      `${shared}
依据 plan-orch：改 orchestrator/archive/promote 路径写 ingress binding、preview_url、runtime trust forwarded、api server previewURL()、config 新 env。
主要 cellp/internal/orch/** cellp/internal/runtime/** cellp/internal/api/** cellp/internal/config/**。
完成后 go test ./internal/orch/... ./internal/api/...`,
      { label: 'impl-orch', subagent_type: 'coder', cwd, model: 'sonnet' },
    ),
  () =>
    agent(
      `${shared}
实现 config.Load 新变量（CELLP_INGRESS_BASE_DOMAIN、CELLP_INGRESS_TIER_B、scheme 分角色等）与 serve  wiring；openapi.yaml preview_url/ingress 相关字段若已有则对齐。
只改 config + serve + openapi 片段。
go test ./internal/config/... ./internal/serve/...`,
      { label: 'impl-config', subagent_type: 'coder', cwd, model: 'sonnet' },
    ),
])

phase('Review-parallel')
const [revSec, revGw, revInt] = await parallel([
  () =>
    agent(
      `在 ${cwd} 对抗审查 AD-12 实现：对照 ${spec} §9 R-* 规则。只读 Grep/Read。输出 blocking 问题列表（文件:行）。不改代码。`,
      { label: 'review-security', subagent_type: 'verification', cwd },
    ),
  () =>
    agent(
      `在 ${cwd} 审查 gateway+registry ingress 集成：竞态、cache invalidate、path 残留。只读。输出必须修项。不改代码。`,
      { label: 'review-integration', subagent_type: 'code-reviewer', cwd },
    ),
  () =>
    agent(
      `在 ${cwd} 运行 cd cellp && go test ./... 并汇报失败包；若失败定位根因。可修测试期望，大改先列出。`,
      { label: 'review-tests', subagent_type: 'verification', cwd },
    ),
])

phase('Fix')
const fix = await agent(
  `${shared}
综合以下审查输出，修复 blocking 问题并保证 cd cellp && go test ./... 通过：
plan summaries: registry/gw/orch done.
Review security: ${String(revSec).slice(0, 8000)}
Review integration: ${String(revGw).slice(0, 8000)}
Review tests: ${String(revInt).slice(0, 8000)}

不要改 celld submodule。e2e 脚本若时间不够可只改 e2e/scripts/lib.sh 的 host URL helper + 注释 path deprecated。`,
  { label: 'fix-all', subagent_type: 'coder', cwd, model: 'sonnet' },
)

phase('Test-final')
const testFinal = await agent(
  `在 ${cwd} 运行 cd cellp && go test ./... ；失败则修到绿。不要 commit。`,
  { label: 'test-final', subagent_type: 'verification', cwd },
)

phase('Commit')
const commit = await agent(
  `在 ${cwd}：git status；若 AD-12 ingress 相关改动存在则提交，message 形如 feat(gateway): AD-12 host-based ingress (P0)

正文简述：ingress_bindings、Host 路由、Forwarded、preview_url/PUBLIC_BASE_URL。

Append: Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>

不要提交 docs/evidence 临时 log。`,
  { label: 'commit', subagent_type: 'coder', cwd },
)

return {
  plan: { planReg, planGw, planOrch, planE2e },
  impl: { implReg, implGw, implOrch, implCfg },
  review: { revSec, revGw, revInt },
  fix,
  testFinal,
  commit,
}
