export const meta = {
  name: 'cellp-playwright-mock-evidence',
  description: '仅 Playwright mock E2E + 证据/summary commit',
}

const cwd = '/Users/mino/code/remote/cellp'

phase('playwright-mock')

const playMock = await agent(
  `仓库 ${cwd}/web。

1. 若 \`lsof -i :4173\` 有进程且不是当前测试，记录并继续（playwright.config 已 reuseExistingServer）。
2. 缺 Chromium：\`npx playwright install chromium\`（最多一次，超时 5 分钟）。
3. 运行 \`npm run test:e2e\`（不要设 CI=1）。
4. 将完整 stdout/stderr 写入 \`${cwd}/docs/evidence/user-loop-playwright-mock.log\`（mkdir -p docs/evidence）。
5. 失败则修 mock-api 或 spec 后重跑直至绿。不要 commit 代码除非为修测试。

返回：passed/total 与 log 路径。`,
  { label: 'playwright-mock', subagent_type: 'verification', cwd: `${cwd}/web` },
)

phase('evidence-commit')

const evidenceCommit = await agent(
  `仓库 ${cwd}。

1. 更新 \`docs/evidence/user-loop-workflow-summary.md\`：追加 Playwright mock 结果与日期（中文一行）。
2. \`git status --short docs/evidence\`
3. 仅 commit 可跟踪文件（**不要** add *.log，gitignore）：
   - docs/evidence/user-loop-workflow-summary.md
   message: docs(evidence): playwright mock gate summary
   Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>
4. 若无变更 SKIP

返回 commit hash 或 SKIP。`,
  { label: 'commit-evidence', subagent_type: 'coder', cwd },
)

return { playMock, evidenceCommit }
