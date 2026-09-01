#!/usr/bin/env bash
# Apply wrangler migrations_dir to a ready version's celld D1 bucket (dev).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROJECT="${1:?project}"
VERSION="${2:?version}"
ART="${ROOT}/dev/data/artifacts/${PROJECT}/${VERSION}"
# shellcheck disable=SC1091
source "${ROOT}/dev/.env"
export AWS_ACCESS_KEY_ID="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
export AWS_SECRET_ACCESS_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ENDPOINT_URL="${S3_ENDPOINT:-http://127.0.0.1:19000}"
[[ -f "${ART}/wrangler.jsonc" ]] || { echo "no artifact ${ART}" >&2; exit 1; }
DB="$(node -e "
const fs=require('fs');
const p=process.argv[1];
let raw=fs.readFileSync(p,'utf8');
raw=raw.replace(/\/\/[^\n]*/g,'').replace(/\/\*[\s\S]*?\*\//g,'');
raw=raw.replace(/,(\s*[}\]])/g,'\$1');
const j=JSON.parse(raw);
console.log(j.d1_databases[0].database_name);
" "${ART}/wrangler.jsonc")"
BUCKET="s3://cellp-celld/${PROJECT}/${VERSION}"
echo "==> d1 migrations apply ${DB} ${BUCKET}"
cd "${ART}"
celld d1 migrations apply "${DB}" --bucket "${BUCKET}"
