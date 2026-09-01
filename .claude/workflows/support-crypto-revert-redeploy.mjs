export const meta = {
  name: "support-crypto-revert-redeploy",
  description: "Revert EdgeEver crypto deploy patches, rebuild celld, redeploy S08/S05, smoke login",
};

const ROOT = "/Users/mino/code/remote/cellp";

phase("Verify celld");
const celldBuild = await agent(
  `In ${ROOT}/celld: cargo build -p celld --profile lab 2>&1 | tail -15. Report success. Remind user to install binary to ~/.local/bin/celld and restart cellpd if needed.`,
  { label: "build-celld", allowedTools: ["Bash"] },
);

phase("Revert EdgeEver patches");
const revert = await agent(
  `Task in ${ROOT}:
1. Restore dev/support-corpus/support-edgeever/apps/api/src/auth-crypto.ts to Web Crypto subtle PBKDF2 only (remove node:buffer/node:crypto imports; use crypto.subtle.deriveBits for PBKDF2).
2. Remove or disable deploy hooks: dev/examples/support-edgeever/patch-auth-crypto-pbkdf2.sh and patch-build-worker-node-externals.sh from being required — edit deploy-support-app.sh to NOT call PATCH_CRYPTO/PATCH_BUILD for support-edgeever OR delete those patch scripts and revert prepare-artifact.sh esbuild --external lines.
3. Revert dev/support-corpus/support-edgeever/scripts/build-cloudflare-worker.mjs external array if present.
4. Document in docs/support-validation-lessons.md one line: EdgeEver uses subtle PBKDF2 after celld runtime fix.

Do NOT touch celld submodule crypto.rs/js except if already committed. Only cellp dev/examples + corpus auth-crypto.`,
  { label: "revert-patches", allowedTools: ["Read", "Edit", "Grep", "Glob"] },
);

phase("Redeploy");
const deploy = await agent(
  `In ${ROOT}:
1. cd dev/support-corpus/support-edgeever && bun run build:web && bun run build:worker && bash dev/examples/support-edgeever/prepare-artifact.sh .
2. SUPPORT_SKIP_GIT_FETCH=1 SUPPORT_SKIP_BUILD=1 ./dev/scripts/deploy-support-app.sh S08
3. Clear stale admin in D1 if login 401: celld d1 execute from artifact dir DELETE users WHERE username='admin'
4. curl login admin / cellp-dev-edgeever on support-edgeever.lvh.me:8787
Report HTTP codes and session authenticated field.`,
  { label: "deploy-s08", allowedTools: ["Bash", "Read"] },
);

phase("FlareMo smoke");
const flaremo = await agent(
  `Optional quick smoke ${ROOT}: curl -sS http://support-flaremo.lvh.me:8787/api/auth/get-session -H 'Host: support-flaremo.lvh.me' | head -c 300. No redeploy unless broken.`,
  { label: "smoke-s05", allowedTools: ["Bash"] },
);

return { celldBuild, revert, deploy, flaremo };
