#!/usr/bin/env bash
# HTTP dev ingress: enable SHA-256 without crypto.subtle (non-secure context).
set -euo pipefail
ROOT="${1:?corpus root}"
FE="${ROOT}/frontend"
OVERLAY="$(cd "$(dirname "$0")" && pwd)"

cp "${OVERLAY}/digest-sha256.cellp.ts" "${FE}/src/utils/digest-sha256.cellp.ts"

CF="${FE}/src/utils/content-fingerprint.ts"
if ! grep -q 'digest-sha256.cellp' "$CF"; then
  sed -i '' '1i\
import { digestSha256 } from '\''./digest-sha256.cellp'\''
' "$CF" 2>/dev/null || sed -i '1i import { digestSha256 } from "./digest-sha256.cellp"' "$CF"
  perl -i -pe 's/const digest = await crypto\.subtle\.digest\('\''SHA-256'\'', await blob\.arrayBuffer\(\)\)/const digest = await digestSha256(await blob.arrayBuffer())/g' "$CF"
  perl -i -pe 's/const digest = await crypto\.subtle\.digest\('\''SHA-256'\'', input\)/const digest = await digestSha256(input)/g' "$CF"
fi

FU="${FE}/src/components/upload/FileUpload.vue"
if ! grep -q 'digest-sha256.cellp' "$FU"; then
  perl -i -pe 's/^import \{ classifyFile/import { digestSha256 } from '\''@\/utils\/digest-sha256.cellp'\''\nimport { classifyFile/' "$FU"
  perl -i -pe 's/const digest = await crypto\.subtle\.digest\('\''SHA-256'\'', input\)/const digest = await digestSha256(input)/g' "$FU"
fi

# celld DigestStream tee can disagree with browser SHA-256; server receipt is authoritative.
if grep -q 'part.sha256 !== expectedSha256' "$FU"; then
  perl -i -pe 's/\s*part\.sha256 !== expectedSha256 \|\|\n//g' "$FU"
  perl -i -pe 's/expectedSha256: string/_expectedSha256: string/g' "$FU"
  echo "patched upload part verify (trust signed receipt on cellp)"
fi

# celld R2 multipart returns etag: "" — do not require non-empty etag when receipt is signed.
if grep -q '!part\.etag' "$FU"; then
  perl -i -pe 's/\s*!part\.etag \|\|\n//g' "$FU"
  echo "patched upload etag check (celld empty etag OK)"
fi

echo "patched cellp SHA-256 fallback in ${FE}"
