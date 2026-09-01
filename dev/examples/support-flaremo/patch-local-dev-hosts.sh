#!/usr/bin/env bash
# cellp dev: allow http://*.lvh.me and *.ingress.local for FLAREMO_PUBLIC_URL.
set -euo pipefail
AUTH="${1:?auth.ts path}"
node <<'NODE' "$AUTH"
const fs = require('fs');
const p = process.argv[1];
let s = fs.readFileSync(p, 'utf8');
const needle = 'hostname.endsWith(".test")';
const patch = `hostname.endsWith(".test") ||
    hostname.endsWith(".lvh.me") ||
    hostname.endsWith(".ingress.local")`;
if (s.includes('.lvh.me")) {
  console.log('already patched', p);
  process.exit(0);
}
if (!s.includes(needle)) {
  console.error('patch-local-dev-hosts: pattern not found in', p);
  process.exit(1);
}
s = s.replace(needle, patch);
fs.writeFileSync(p, s);
console.log('patched local dev hostnames in', p);
NODE
