#!/usr/bin/env bash
# Patch bundled worker after wrangler dry-run (cellp http://*.lvh.me).
set -euo pipefail
BUNDLE="${1:?index.js}"
node <<'NODE' "$BUNDLE"
const fs = require('fs');
const p = process.argv[1];
let s = fs.readFileSync(p, 'utf8');
const from = 'hostname3.endsWith(".test")';
const to = 'hostname3.endsWith(".test") || hostname3.endsWith(".lvh.me") || hostname3.endsWith(".ingress.local")';
if (s.includes('.lvh.me')) process.exit(0);
if (!s.includes(from)) {
  console.error('patch-bundle-local-hosts: pattern not found');
  process.exit(1);
}
s = s.replaceAll(from, to);
fs.writeFileSync(p, s);
console.log('patched bundle local hostnames');
NODE
