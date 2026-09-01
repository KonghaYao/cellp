#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:?corpus root}"
SHARE="${ROOT}/worker/src/routes/share.ts"
OVERLAY="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "${ROOT}/worker/src/lib"
cp "${OVERLAY}/public-origin.ts" "${ROOT}/worker/src/lib/public-origin.ts"

FILE="$SHARE" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (!s.includes('publicShareUrl')) {
  s = s.replace(
    "import { Hono } from 'hono'\n",
    "import { Hono } from 'hono'\nimport { publicShareUrl } from '../lib/public-origin'\n",
  );
}
s = s.replace(
  /`\$\{new URL\(c\.req\.url\)\.origin\}\/#\/share\/\$\{rawCode\}`/g,
  'publicShareUrl(c, rawCode)',
);
s = s.replace(
  /`\$\{new URL\(c\.req\.url\)\.origin\}\/#\/share\/\$\{code\}`/g,
  'publicShareUrl(c, code)',
);
fs.writeFileSync(p, s);
NODE

echo "patched share URLs → publicOrigin"
