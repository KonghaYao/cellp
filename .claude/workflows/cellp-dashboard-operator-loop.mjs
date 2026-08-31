export const meta = {
  name: 'cellp-dashboard-operator-loop',
  description: 'TP-UI-14/15 门禁：顺序验证（避免 parallel 长时间 playwright install 被 kill）',
}

const cwd = '/Users/mino/code/remote/cellp'

phase('vitest')
const vitestGate = await agent(
  `在 ${cwd} 运行 cd web && npm run test；失败则修 web 测试。不要 commit。`,
  { label: 'vitest', subagent_type: 'verification', cwd },
)

phase('verify-script')
const verifyLoop = await agent(
  `在 ${cwd} 运行 bash web/scripts/verify-user-loop.sh。不要 commit。返回 log 路径。`,
  { label: 'verify-loop', subagent_type: 'verification', cwd },
)

phase('playwright-mock')
const playMock = await agent(
  `在 ${cwd}/web 运行 npm run test:e2e（勿设 CI=1 若 4173 已被占用）。缺浏览器则 npx playwright install chromium 一次。失败修 spec/mock。不要 commit。`,
  { label: 'playwright-mock', subagent_type: 'verification', cwd },
)

phase('playwright-live')
const liveE2e = await agent(
  `在 ${cwd}：health.sh 通过则 cd web && npm run test:e2e:live，日志追加 docs/evidence/user-loop-live-e2e.log。栈未起则 SKIP。不要 commit。`,
  { label: 'playwright-live', subagent_type: 'verification', cwd },
)

phase('site-build')
const siteBuild = await agent(
  `在 ${cwd}/site 运行 npm run docs:build。失败修文档。不要 commit。`,
  { label: 'site-build', subagent_type: 'verification', cwd },
)

phase('evidence-commit')
const evidenceCommit = await agent(
  `在 ${cwd}：若有 docs/evidence 新文件则 commit docs(evidence): user-loop gates。Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>。否则 SKIP。`,
  { label: 'commit-evidence', subagent_type: 'coder', cwd },
)

return { vitestGate, verifyLoop, playMock, liveE2e, siteBuild, evidenceCommit }
