export const meta = {
  name: 'doc-full-refresh',
  description:
    'Parallel public-doc update (site/docs + README) then parallel verification and VitePress build gate',
}

const REPO = args.repo ?? '/Users/mino/code/remote/cellp'
const EVIDENCE = `${REPO}/docs/evidence/doc-refresh`
const SCOPE_NOTE = args.scope ?? 'full refresh: align public docs with current product behavior'

const AUTHORITY = `
Authority (read-only for facts): ${REPO}/DESIGN.md, ${REPO}/docs/decisions.md, ${REPO}/celld/docs/cloudflare-compat.md.
User-facing rules: English on site/docs; no internal task IDs (Sxx, Axx, AD-x, e2e script names as primary navigation); no postgres/caddy/forgejo as cellp deps; dashboard is Vite SPA not Next.js.
Do NOT edit frozen RPC contracts under docs/plans/D1-*-RPC.md unless explicitly broken.
`

const SHARDS = [
  {
    id: 'onboarding',
    paths: 'site/docs/get-started/**, site/docs/guides/install.md, guides/dev.md, guides/self-hosting.md',
    globHint: 'get-started/*.md and core install/dev/self-host guides',
  },
  {
    id: 'concepts',
    paths:
      'site/docs/concepts/**, site/docs/how-it-works.md, site/docs/what-is-cellp.md, site/docs/why.md, site/docs/index.md',
    globHint: 'concepts + narrative top pages',
  },
  {
    id: 'bindings',
    paths: 'site/docs/bindings/**',
    globHint: 'D1/KV/R2/Queues/Workflows/Cron binding pages',
  },
  {
    id: 'migrate',
    paths: 'site/docs/migrate/**, site/docs/compare.md',
    globHint: 'migration + compare',
  },
  {
    id: 'reference',
    paths:
      'site/docs/reference/**, site/docs/guides/ci.md, guides/observability.md, guides/rollback.md, guides/environment-variables.md',
    globHint: 'reference + operational guides',
  },
  {
    id: 'research-readme',
    paths: 'site/docs/research/**, README.md at repo root',
    globHint: 'research pages + GitHub README (keep external-friendly tone)',
  },
]

const writerPrompt = (shard) => `You are the doc WRITER for shard "${shard.id}" in ${REPO}.
Scope: ${SCOPE_NOTE}
Files: ${shard.paths} (${shard.globHint})
${AUTHORITY}

Tasks:
1. Read inventory section for this shard from ${EVIDENCE}/inventory.md if it exists (from prior phase in same run).
2. Update and add pages so content matches current behavior; fix stale links; add missing sections called out in inventory.
3. Update site/docs/.vitepress/config.ts sidebar/nav ONLY if your shard adds/renames routes (minimal diff).
4. Write ${EVIDENCE}/writer-${shard.id}.md: files touched, gaps closed, open questions.

Do not run full e2e. Do not commit unless args.commit is true.`

const verifierPrompt = (shard, writerOut) => `You are the doc VERIFIER for shard "${shard.id}" in ${REPO}. READ-ONLY except writing the report file.
Scope files: ${shard.paths}
${AUTHORITY}

Writer handoff (may be truncated): ${String(writerOut ?? '').slice(0, 4000)}

Verify:
- Factual claims vs DESIGN/decisions/celld compat (cite file:line when wrong)
- No internal engineering codes in user-facing prose
- Links and paths exist; no fabricated URLs
- Tone consistent with neighboring pages

Write ${EVIDENCE}/verify-${shard.id}.md with PASS/FAIL, bullet findings, required fixes (file + one-line action).
Do NOT edit site/docs except the report.`

phase('inventory')
log(`doc-full-refresh: ${SCOPE_NOTE}`)
const inventory = await agent(
  `Doc inventory for ${REPO}. ${SCOPE_NOTE}
${AUTHORITY}

List every file under site/docs/**/*.md and README.md. For each shard ${SHARDS.map((s) => s.id).join(', ')}:
- stale/missing topics vs DESIGN/decisions
- suggested new pages
- nav/sidebar gaps in site/docs/.vitepress/config.ts

Write ${EVIDENCE}/inventory.md (structured markdown). Read-only on product code except creating evidence dir and inventory file.`,
  { label: 'inventory', model: 'sonnet', subagent_type: 'explorer' },
)

phase('write')
const writerResults = await parallel(
  SHARDS.map(
    (shard) => () =>
      agent(writerPrompt(shard), {
        label: `write:${shard.id}`,
        model: 'sonnet',
        subagent_type: 'coder',
      }),
  ),
)

phase('verify')
const verifyResults = await parallel(
  SHARDS.map(
    (shard, i) => () =>
      agent(verifierPrompt(shard, writerResults[i]), {
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
      `In ${REPO}: from repo root run pnpm --filter cellp-docs docs:build (timeout-friendly). Capture log to ${EVIDENCE}/vitepress-build.log. If fail, list first errors and which md files likely cause them. Report PASS/FAIL in ${EVIDENCE}/build-gate.md. Fix only vitepress config or broken links if trivial; else report only.`,
      { label: 'build-gate', model: 'haiku', subagent_type: 'verification' },
    ),
  () =>
    agent(
      `Read ${REPO}/site/docs/.vitepress/config.ts and all ${EVIDENCE}/verify-*.md. Check every site/docs md is reachable from nav or linked from index; orphan pages listed in ${EVIDENCE}/nav-gate.md with PASS/FAIL.`,
      { label: 'nav-gate', model: 'haiku', subagent_type: 'verification' },
    ),
])

phase('summary')
const summary = await agent(
  `Merge doc refresh results in ${REPO}:
- inventory: ${EVIDENCE}/inventory.md
- verify shards: ${EVIDENCE}/verify-*.md
- build: ${EVIDENCE}/build-gate.md
- nav: ${EVIDENCE}/nav-gate.md

Write ${EVIDENCE}/SUMMARY.md: executive status PASS/FAIL, blockers, recommended follow-up (single pass fixes). Read-only except evidence files.`,
  { label: 'summary', model: 'haiku' },
)

return {
  inventory,
  writers: SHARDS.map((s, i) => ({ shard: s.id, result: writerResults[i] })),
  verifiers: SHARDS.map((s, i) => ({ shard: s.id, result: verifyResults[i] })),
  buildGate,
  navGate,
  summary,
}
