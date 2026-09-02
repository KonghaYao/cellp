#!/usr/bin/env bash
# Point hello-agent at OpenCode Zen free tier (public API key + https://opencode.ai/zen).
set -euo pipefail
APP_DIR="${1:?hello-agent dir}"
SRC="${APP_DIR}/src/index.ts"
[[ -f "$SRC" ]] || { echo "patch-opencode-zen: missing $SRC" >&2; exit 1; }

export PATCH_SRC="$SRC"
node <<'NODE'
const fs = require('fs');
const p = process.env.PATCH_SRC;
let s = fs.readFileSync(p, 'utf8');
if (s.includes('OPENROUTER_API_KEY')) {
  s = s.replace(/OPENROUTER_API_KEY/g, 'OPENCODE_API_KEY');
}
s = s.replace(
  /getModel\("(?:openrouter|opencode)",\s*"[^"]+"\)/,
  'getModel("opencode", "nemotron-3-ultra-free")'
);
fs.writeFileSync(p, s);
console.log('patch-opencode-zen: OPENCODE_API_KEY=public + opencode/nemotron-3-ultra-free');
NODE
