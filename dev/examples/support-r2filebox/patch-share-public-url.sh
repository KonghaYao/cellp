#!/usr/bin/env bash
set -euo pipefail
WORKER="${1:?worker/src directory}"
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cp "${ROOT}/dev/examples/support-r2filebox/public-origin.ts" "${WORKER}/lib/public-origin.ts"

SHARE="${WORKER}/routes/share.ts"
FILE="$SHARE" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (s.includes("from '../lib/public-origin'")) process.exit(0);
if (!s.includes('publicShareUrl(c,')) {
  s = s.replace(
    /const url = `\$\{new URL\(c\.req\.url\)\.origin\}\/#\/share\/\$\{rawCode\}`/g,
    'const url = publicShareUrl(c, rawCode)',
  );
  s = s.replace(
    /const url = `\$\{new URL\(c\.req\.url\)\.origin\}\/#\/share\/\$\{code\}`/g,
    'const url = publicShareUrl(c, code)',
  );
}
s = s.replace(
  "import { Hono } from 'hono'\n",
  "import { Hono } from 'hono'\nimport { publicShareUrl } from '../lib/public-origin'\n",
);
fs.writeFileSync(p, s);
console.log('patched', p);
NODE
