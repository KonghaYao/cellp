export const meta = {
  name: 'cellp-dashboard-operator-loop',
  description:
    'Dashboard 用户闭环 + 巡检：验证 → 分段提交（文档 / Vitest / E2E live / Inspect）',
}

const cwd = '/Users/mino/code/remote/cellp'

const segments = [
  {
    id: 'docs-site',
    message: 'docs(site): operator journey and closed-loop PM record',
    paths: [
      'site/docs/get-started/operator-journey.md',
      'site/docs/.vitepress/config.ts',
      'site/docs/get-started/dashboard.md',
      'site/docs/get-started/index.md',
      'docs/plans/user-behavior-closed-loop.md',
    ],
    verify: null,
  },
  {
    id: 'vitest-flows',
    message: 'feat(web): vitest user flows and create project UI',
    paths: [
      'web/vitest.config.ts',
      'web/src/test',
      'web/src/flows',
      'web/src/lib/cellp-api.ts',
      'web/src/components/create-project-dialog.tsx',
      'web/src/pages/ProjectsPage.tsx',
      'web/e2e/create-project.spec.ts',
      'web/e2e/mock-api-server.mjs',
      'web/playwright.config.ts',
      'web/package.json',
      'web/package-lock.json',
      'web/tsconfig.json',
    ],
    verify: 'cd web && npm run test',
  },
  {
    id: 'e2e-live-checklist',
    message: 'feat(web): operator checklist and live Playwright E2E',
    paths: [
      'web/playwright.live.config.ts',
      'web/e2e/live',
      'web/scripts',
      'web/src/components/operator-checklist.tsx',
    ],
    verify: 'cd web && npm run test',
  },
  {
    id: 'inspect-monitoring',
    message: 'feat(web): project inspect page and platform monitoring',
    paths: [
      'web/src/lib/inspection.ts',
      'web/src/lib/inspection.test.ts',
      'web/src/pages/ProjectInspectPage.tsx',
      'web/src/components/version-runtime-health.tsx',
      'web/src/components/deployments-status-summary.tsx',
      'web/src/App.tsx',
      'web/src/components/layout/app-sidebar.tsx',
      'web/src/components/version-detail-view.tsx',
      'web/src/lib/routes.ts',
      'web/src/pages/DeploymentsPage.tsx',
      'web/src/pages/PlatformPage.tsx',
      'web/src/pages/ProjectOverviewPage.tsx',
    ],
    verify: 'cd web && npm run test',
  },
  {
    id: 'test-plan-agents',
    message: 'docs: TP-UI-14/15 test plan and web AGENTS verify commands',
    paths: ['docs/test-plan.md', 'web/AGENTS.md'],
    verify: null,
  },
]

phase('verify-all')
const verify = await agent(
  `在 ${cwd} 运行 \`cd web && npm run test\`，exit 0 才继续。若失败则修复 web/ 相关测试后重跑。不要 commit。`,
  { label: 'vitest-gate', subagent_type: 'verification', cwd },
)

phase('commits')
const commitResults = await pipeline(
  segments,
  (seg) =>
    agent(
      `在 ${cwd} 做**单次** git 提交（仅本 segment）。

Segment id: ${seg.id}
Commit message（第一行，勿 amend）:
${seg.message}

仅 stage 这些路径（不存在则跳过）:
${seg.paths.map((p) => `- ${p}`).join('\n')}

规则:
- 不要 stage cellp/coverage.out 或 .claude/workflow-runs
- 不要 amend；新建 commit
- message 末尾追加:
Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>
- 提交前若 verify 命令非空则运行: ${seg.verify ?? '(skip)'}
- 返回: commit hash 或 SKIP 原因`,
      { label: `commit-${seg.id}`, subagent_type: 'coder', cwd },
    ),
)

return { verify, commitResults, segments: segments.map((s) => s.id) }
