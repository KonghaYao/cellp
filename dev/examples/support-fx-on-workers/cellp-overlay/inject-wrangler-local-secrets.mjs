#!/usr/bin/env node
/** Merge local dev secrets into wrangler.jsonc vars (artifact only, not committed). */
import fs from "node:fs";

const file = process.argv[2];
if (!file) {
  console.error("usage: inject-wrangler-local-secrets.mjs <wrangler.jsonc>");
  process.exit(1);
}
const key = process.env.AI_GATEWAY_API_KEY?.trim();
if (!key) {
  console.log("inject-wrangler-local-secrets: skip (no AI_GATEWAY_API_KEY)");
  process.exit(0);
}
const model = process.env.FX_MODEL?.trim() || "minimax/minimax-m3-free";
let raw = fs.readFileSync(file, "utf8");
const j = JSON.parse(raw.replace(/\/\/[^\n]*/g, "").replace(/,\s*([}\]])/g, "$1"));
j.vars ??= {};
j.vars.AI_GATEWAY_API_KEY = key;
j.vars.FX_MODEL = model;
fs.writeFileSync(file, JSON.stringify(j, null, 2) + "\n");
console.log("inject-wrangler-local-secrets: AI_GATEWAY_API_KEY + FX_MODEL in wrangler vars");
