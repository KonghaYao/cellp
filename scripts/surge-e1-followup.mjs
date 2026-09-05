export const meta = {
  name: 'surge-e1-followup',
  description: 'E1 parallel: OpenAPI deploy_ready, web status, gateway snapshot notes',
}

phase('E1/API')
const apiResult = await agent(
  `WP-API task (cellp repo /Users/mino/code/remote/cellp):
1. Read cellp/api/openapi.yaml Version status enum (~line 1230).
2. Add additive enum value deploy_ready after deploying, before ready. Update description only if needed; keep ready semantics unchanged.
3. Do NOT change runtime behavior. Run: cd cellp && go test ./internal/api/ -count=1 if tests exist for openapi.
Return: files changed and diff summary.`,
  { label: 'api-openapi', model: 'sonnet' },
)

phase('E1/WEB')
const webResult = await agent(
  `WP-WEB task (/Users/mino/code/remote/cellp/web):
1. Add deploy_ready to web/src/lib/status.ts VERSION_STATUSES, STATUS_DOT, STATUS_TEXT, STATUS_TIMELINE (between deploying and ready).
2. Ensure version-actions: promote/archive still only on ready; deploy_ready shows distinct styling.
3. Run: cd web && npm test -- --run src/lib/status 2>/dev/null || npm run test -- --run 2>/dev/null | tail -20
Return: summary of edits.`,
  { label: 'web-status', model: 'sonnet' },
)

phase('E1/GW-doc')
const gwResult = await agent(
  `Read-only design note for WP-GW (/Users/mino/code/remote/cellp):
Write docs/evidence/surge/e1/2026-09-05-wp-reg/gateway-snapshot-integration.md explaining how gateway should poll registry.GetRouteRevision + BuildLegacyRouteSnapshot (already on Store). No gateway code changes unless trivial comment. Max 40 lines.`,
  { label: 'gw-notes', model: 'haiku' },
)

return { apiResult, webResult, gwResult }
