export const meta = {
  name: "support-batch-deploy-verify",
  description: "Sequential deploy+verify S21-S14 (skip S20 if running), update support-batch-results.md",
};

const ROOT = "/Users/mino/code/remote/cellp";
const QUEUE = ["S21", "S18", "S07", "S09", "S15", "S19", "S10", "S14"];
const PROJECT = {
  S21: "support-fileworker",
  S18: "support-webhookflare",
  S07: "support-monolith",
  S09: "support-sonicjs",
  S15: "support-workflows",
  S19: "support-requestbin",
  S10: "support-nodewarden",
  S14: "support-cfbase",
};

for (const sid of QUEUE) {
  const project = PROJECT[sid];
  phase(`Deploy ${sid}`);
  const deploy = await agent(
    `Deploy ${sid} (${project}) in ${ROOT}.
- Overlay path: dev/examples/${project}/wrangler.cellp.jsonc — create if missing from corpus wrangler.toml/json.
- Run: cd ${ROOT} && SUPPORT_SKIP_GIT_FETCH=1 ./dev/scripts/deploy-support-app.sh ${sid}
- Timeout up to 600000ms for build.
Return JSON-ish: status, version, prod_url, prod_http, blockers.`,
    { label: `deploy-${sid}` },
  );

  phase(`Verify ${sid}`);
  const verify = await agent(
    `Verify ${sid} ${project}. Deploy output: ${String(deploy).slice(0, 3000)}
Use curl -H Host ${project.replace("support-", "support-")}.lvh.me http://127.0.0.1:8787/
Return: verdict pass|fail, prod_http, one_line_summary.`,
    { label: `verify-${sid}`, subagent_type: "verification" },
  );

  await agent(
    `Update ${ROOT}/docs/support-batch-results.md: set row ${sid} deploy/verify columns and add ### ${sid} section. Keep S06 intact.`,
    { label: `doc-${sid}` },
  );
}

return { queue: QUEUE };
