export const meta = {
  name: 'cellp-dashboard-operator-loop',
  description:
    '完成 TP-UI-14/15：并行验证 Vitest/Playwright/verify 脚本 → live E2E → site build → 证据提交',
}

const cwd = '/Users/mino/code/remote/cellp'

phase('verify-parallel')

const [vitestGate, verifyLoop, playMock] = await parallel([
  () =>
    agent(
      `仓库 ${cwd}。运行 \`cd web && npm run test\`。失败则修复 web 测试/源码直至绿。不要 commit。返回测试摘要。`,
      { label: 'vitest-gate', subagent_type: 'verification', cwd },
    ),
  () =>
    agent(
      `仓库 ${cwd}。运行 \`bash web/scripts/verify-user-loop.sh\`（会写 docs/evidence/user-loop-vitest-*.log）。失败则修 web 直至通过。不要 commit。返回 log 路径。`,
      { label: 'verify-user-loop', subagent_type: 'verification', cwd },
    ),
  () =>
    agent(
      `仓库 ${cwd}。运行 \`cd web && CI=1 npm run test:e2e\`。若缺 Playwright 浏览器则 \`npx playwright install chromium\` 后重试一次。失败则修 mock-api 或 spec。不要 commit。返回 pass/fail 计数。`,
      { label: 'playwright-mock', subagent_type: 'verification', cwd },
    ),
])

phase('live-e2e')

const liveE2e = await agent(
  `仓库 ${cwd}。
1. \`./dev/scripts/health.sh\` 或 curl :8790/v1/health；若栈未起则 SKIP live（说明需 ./dev/scripts/up.sh），不要强行 up。
2. 栈健康则 \`cd web && npm run test:e2e:live\`。
3. 将 stdout 追加写入 docs/evidence/user-loop-live-e2e.log（mkdir -p docs/evidence）。
不要 commit。返回 SKIP 或 pass/fail。`,
  { label: 'playwright-live', subagent_type: 'verification', cwd },
)

phase('site-build')

const siteBuild = await agent(
  `仓库 ${cwd}。运行 \`cd site && npm ci && npm run docs:build\`（若 node_modules 已有可省略 ci）。失败则修 site 文档链接/ frontmatter。不要 commit。`,
  { label: 'site-docs-build', subagent_type: 'verification', cwd },
)

phase('evidence-commit')

const evidenceCommit = await agent(
  `仓库 ${cwd}。
1. \`git status --short docs/evidence web\`
2. 若有 docs/evidence/user-loop*.log 或其它新证据且未提交，单独 commit：
   docs(evidence): user-loop vitest and live e2e logs
   Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>
3. 若无变更返回 SKIP
4. 不要 amend 已有 5 个 dashboard commit`,
  { label: 'commit-evidence', subagent_type: 'coder', cwd },
)

phase('summary')

const summary = await agent(
  `仓库 ${cwd}。只读汇总 TP-UI-14/15 完成情况：
- 读 docs/test-plan.md TP-UI-14/15
- 读 docs/plans/user-behavior-closed-loop.md
- 列出 verify/live/playwright 结果（从上一 phase 上下文或 evidence log）
写入 docs/evidence/user-loop-workflow-summary.md（中文简短）。
若有未通过门禁，列出阻塞项。可 commit 该 summary 文件（同 Co-Authored-By）。`,
  { label: 'workflow-summary', subagent_type: 'general-purpose', cwd },
)

return { vitestGate, verifyLoop, playMock, liveE2e, siteBuild, evidenceCommit, summary }
