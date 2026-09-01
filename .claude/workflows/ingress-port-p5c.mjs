export const meta = {
  name: 'ingress-port-p5c',
  description:
    'INGRESS P5c: Gateway ReconcileListeners, prod_port hybrid, e2e port curl',
}

const cwd = '/Users/mino/code/remote/cellp'
const spec = `${cwd}/docs/plans/INGRESS-PORT-DEPLOYMENT.md`
const p5bReview = `${cwd}/docs/plans/INGRESS-PORT-P5b-review.md`
const p5cPlan = `${cwd}/docs/plans/INGRESS-PORT-P5c-impl-plan.md`
const p5cReview = `${cwd}/docs/plans/INGRESS-PORT-P5c-review.md`

phase('Plan-P5c')
const planResult = await agent(
  `Read ${spec} §5–§9 and ${p5bReview}. Write ${p5cPlan} for P5c only:
- Gateway ReconcileListeners on cellpd start + after binding changes (127.0.0.1:listen_port per active binding with owner_gateway_id match)
- Close listeners on Detach/archive signals (hook from orch or gateway poll/reconcile)
- prod_port mode: preview stays Host @ gateway port, prod uses stable listen port
- e2e script or e2e/ test: dedicated_port preview curl 200; promote prod port unchanged (optional in P5c)
- web: format.ts trust absolute API preview_url/prod_url when port != gateway port (minimal)
Do not implement in this phase.`,
  { description: 'P5c plan', subagent_type: 'plan', cwd },
)

phase('Implement-P5c')
const impl = await agent(
  `Implement P5c per ${p5cPlan}. Primary: cellp/internal/gateway/ and cellp/internal/serve/ (listener reconcile only — skip unrelated TLS unless plan requires).
Registry/orch already in P5b; extend only if reconcile needs callbacks.
Add gateway unit tests + registry integration if needed.
Run: cd ${cwd}/cellp && go test ./internal/gateway/... ./internal/orch/... -count=1
Write evidence snippet to docs/evidence/ingress-port-p5c.md if tests pass.`,
  { description: 'P5c gateway listeners', subagent_type: 'coder', cwd },
)

phase('Review-P5c')
const review = await agent(
  `Review P5c vs ${spec} R-ARCHIVE-TEARDOWN (TCP unreachable). Write ${p5cReview}: pass/fail, blocking, e2e status.`,
  { description: 'P5c review', subagent_type: 'verification', cwd },
)

phase('Fix-P5c')
const fix = await agent(
  `Fix blocking items in ${p5cReview}. Minimal diff.`,
  { description: 'P5c fix', subagent_type: 'coder', cwd },
)

phase('Verify-P5c')
const verify = await agent(
  `cd ${cwd}/cellp && go test ./... -count=1 2>&1 | tail -40
Append Verify section to ${p5cReview}.`,
  { description: 'verify-p5c', subagent_type: 'verification', cwd },
)

return { planResult, impl, review, fix, verify }
