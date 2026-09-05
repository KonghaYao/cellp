export const meta = {
  name: 'surge-phased-delivery',
  description: 'Sequential E2-E5+ADOPT+acceptance with git commit per phase',
}

const REPO = '/Users/mino/code/remote/cellp'
const CO = 'Co-Authored-By: composer-2.5 <noreply@anthropic.com>'

const phaseCommit = async (phase, commitMsg, planDoc, evidenceDir, scope) => {
  phase(phase)
  const out = await agent(
    `You are delivering SURGE AD-15 ${phase} in ${REPO}.

Read first: ${planDoc} and docs/decisions.md §20. Default CELLP_ELASTIC_RUNTIME=0; new behavior MUST be flag-gated.

Implement ${scope} with tests. Then:
1. cd ${REPO}/cellp && go test ./... -count=1 (fix failures in scope only).
2. Write ${REPO}/${evidenceDir}/handoff.md with what changed and test result.
3. git add only files you changed for this phase (under cellp/, web/, docs/evidence/surge/, docs/ as needed).
4. git commit -m "${commitMsg}" -m "${CO}"
5. Return: commit hash, PASS/FAIL, handoff path.

Do NOT modify celld submodule, peri/, dev/support scripts unless phase requires. Do NOT run SP lab without args.run_sp.`,
    { label: phase, model: 'sonnet' },
  )
  return out
}

const e2 = await phaseCommit(
  'E2',
  'feat(surge): E2 activator cold-start (flag-gated)',
  'docs/plans/04-activator-cold-start.md',
  'docs/evidence/surge/e2/activator',
  'WP-GW-ACT: cellp/internal/gateway activator package — singleflight, EnsureCapacity client stub to registry desire bump, 503+Retry-After for cold deploy_ready when CELLP_ELASTIC_RUNTIME=1; unit tests',
)

const e3 = await phaseCommit(
  'E3',
  'feat(surge): E3 runtime node and agent scaffolding',
  'docs/plans/02-runtime-backend-node-agent.md',
  'docs/evidence/surge/e3/agent',
  'RuntimeNode store wiring, node agent command handlers (start/stop/probe) stubs under cellp/internal/elastic/agent; flag-gated; tests',
)

const e4 = await phaseCommit(
  'E4',
  'feat(surge): E4 autoscaler and background policy',
  'docs/plans/06-autoscaler-policy.md',
  'docs/evidence/surge/e4/autoscale',
  'Autoscaler loop stub: read policies/desires, compare desired vs ready replicas (registry), no-op when flag=0; background_mode validation hooks; tests',
)

const e5 = await phaseCommit(
  'E5',
  'feat(surge): E5 promote snapshot revision coupling',
  'docs/plans/09-promote-archive-migration.md',
  'docs/evidence/surge/e5/promote',
  'Promote path bumps route revision in same transaction where feasible; deploy_ready transition hooks in orch (flag-gated); tests',
)

phase('ADOPT')
const adopt = await agent(
  `In ${REPO}: update DESIGN.md with short AD-15 elastic serving section (pointer to decisions §20 and docs/plans/SURGE-DESIGN-INDEX.md). Do not duplicate full spec. git commit -m "docs(surge): AD-15 DESIGN adoption pointer" -m "${CO}". Evidence: docs/evidence/surge/e5/adopt-handoff.md`,
  { label: 'ADOPT', model: 'sonnet' },
)

phase('ACCEPT')
const accept = await agent(
  `In ${REPO}: run cd cellp && go test ./... and cd web && npm run test -- --run. If ./dev/scripts/health.sh passes, run ./e2e/scripts/run-all.sh (or note SKIPPED). Write docs/evidence/surge/final/ACCEPTANCE-RUN.md with PASS/FAIL. git commit -m "test(surge): final acceptance evidence" -m "${CO}" for evidence files only.`,
  { label: 'ACCEPT', model: 'sonnet' },
)

return { e2, e3, e4, e5, adopt, accept }
