export const meta = {
  name: 'doc-full-refresh-verify',
  description:
    'Re-run parallel shard verification + VitePress/nav gates after doc edits (no writers)',
}

// Shared with doc-full-refresh.mjs — import via duplicate SHARDS for workflow sandbox (no import).

const REPO = args.repo ?? '/Users/mino/code/remote/cellp'
const EVIDENCE = `${REPO}/docs/evidence/doc-refresh`

const AUTHORITY = `
Authority: ${REPO}/DESIGN.md, ${REPO}/docs/decisions.md, ${REPO}/celld/docs/cloudflare-compat.md.
User-facing: no Sxx/Axx/AD-x in prose.
`

const SHARDS = [
  { id: 'onboarding', paths: 'site/docs/get-started/**, guides/install|dev|self-hosting' },
  { id: 'concepts', paths: 'site/docs/concepts/**, how-it-works, what-is-cellp, why, index.md' },
  { id: 'bindings', paths: 'site/docs/bindings/**' },
  { id: 'migrate', paths: 'site/docs/migrate/**, compare.md' },
  { id: 'reference', paths: 'site/docs/reference/**, operational guides' },
  { id: 'research-readme', paths: 'site/docs/research/**, README.md' },
]

const verifierPrompt = (shard) => `Doc VERIFIER shard "${shard.id}" in ${REPO}. READ-ONLY except ${EVIDENCE}/verify-${shard.id}.md.
Files: ${shard.paths}
${AUTHORITY}
Cross-check writer-${shard.id}.md if present. PASS/FAIL + file:line fixes.`

phase('verify')
const verifyResults = await parallel(
  SHARDS.map(
    (shard) => () =>
      agent(verifierPrompt(shard), {
        label: `verify:${shard.id}`,
        model: 'sonnet',
        subagent_type: 'verification',
      }),
  ),
)

phase('gate')
const [buildGate, navGate] = await parallel([
  () =>
    agent(
      `In ${REPO}: pnpm --filter cellp-docs docs:build → ${EVIDENCE}/vitepress-build.log and build-gate.md PASS/FAIL`,
      { label: 'build-gate', model: 'haiku', subagent_type: 'verification' },
    ),
  () =>
    agent(
      `Nav orphan check vs ${REPO}/site/docs/.vitepress/config.ts → ${EVIDENCE}/nav-gate.md`,
      { label: 'nav-gate', model: 'haiku', subagent_type: 'verification' },
    ),
])

phase('summary')
const summary = await agent(
  `Update ${EVIDENCE}/SUMMARY.md from verify-*.md, build-gate, nav-gate. Overall PASS/FAIL.`,
  { label: 'summary', model: 'haiku' },
)

return { verifyResults, buildGate, navGate, summary }
