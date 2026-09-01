export const meta = {
  name: "cf-web-crypto-ultracode",
  description: "Plan → parallel review → implement guidance → verify for celld CF Web Crypto 100%",
};

const PLAN_PATH = "celld/docs/plans/CF-WEB-CRYPTO-100.md";

phase("Plan");
const planReview = await agent(
  `Read ${PLAN_PATH} and celld/docs/testing.md. Output a 1-page sprint backlog for Phase 0 only (C1–C3 + test harness). List exact files to touch. No code edits.`,
  { label: "plan-phase0", subagent_type: "plan", allowedTools: ["Read", "Grep", "Glob"] },
);

phase("Parallel review");
const [security, workerdAlign, testGap] = await parallel([
  () =>
    agent(
      `Review celld crypto attack surface in crates/celld/js/crypto.js and crypto.rs. Focus: timing leaks, weak alg acceptance, key export. Read-only. Bullet findings.`,
      { label: "review-security", subagent_type: "explorer", allowedTools: ["Read", "Grep"] },
    ),
  () =>
    agent(
      `Compare celld crypto.js sign/verify/encrypt/decrypt/wrap gaps vs Cloudflare Workers Web Crypto doc matrix. Reference ${PLAN_PATH}. Read-only.`,
      { label: "review-cf-matrix", subagent_type: "explorer", allowedTools: ["Read", "Grep", "WebFetch"] },
    ),
  () =>
    agent(
      `Propose minimal crypto-conformance test layout under celld/tests/crypto-conformance/ per ${PLAN_PATH}. List first 8 fixture names and runner interface. Read-only.`,
      { label: "review-tests", subagent_type: "plan", allowedTools: ["Read", "Grep", "Glob"] },
    ),
]);

phase("Implement guidance");
const implementBrief = await agent(
  `Synthesize Phase 0 implementation order from:\nPlan:\n${planReview}\n\nSecurity:\n${security}\n\nCF:\n${workerdAlign}\n\nTests:\n${testGap}\n\nOutput: ordered checklist for human/coder agent with verify command per step.`,
  { label: "implement-brief", subagent_type: "coder", allowedTools: ["Read"] },
);

phase("Verify");
const verify = await agent(
  `From repo root cellp: run cd celld && cargo test -p celld --lib 2>&1 | tail -20. Report pass/fail. If crypto-conformance script missing, say so and suggest creating per ${PLAN_PATH}.`,
  { label: "verify-celld", subagent_type: "verification", allowedTools: ["Bash", "Read", "Glob"] },
);

return {
  planReview,
  security,
  workerdAlign,
  testGap,
  implementBrief,
  verify,
};
