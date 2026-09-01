#!/usr/bin/env bash
# One-shot: dev stack + demo-app prod ingress + Dashboard for manual UI check (AD-12).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [[ ! -f dev/.env ]]; then
  cp dev/.env.example dev/.env
fi
set -a
# shellcheck disable=SC1091
source dev/.env
set +a

export CELLP_INGRESS_BASE_DOMAIN="${CELLP_INGRESS_BASE_DOMAIN:-ingress.local}"
export CELLP_LENIENT_DEPLOY="${CELLP_LENIENT_DEPLOY:-1}"
grep -q CELLP_INGRESS_BASE_DOMAIN dev/.env || echo "CELLP_INGRESS_BASE_DOMAIN=ingress.local" >> dev/.env
grep -q CELLP_LENIENT_DEPLOY dev/.env || echo "CELLP_LENIENT_DEPLOY=1" >> dev/.env
grep -q VITE_CELLP_INGRESS_BASE_DOMAIN web/.env 2>/dev/null || echo "VITE_CELLP_INGRESS_BASE_DOMAIN=ingress.local" >> web/.env

echo "==> build cellpd"
./dev/scripts/build-cellpd.sh >/dev/null

echo "==> dev stack (docker + cellpd)"
./dev/scripts/up.sh 2>&1 | tail -5

TOKEN="${ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
PROJECT="${DEV_PROJECT:-demo-app}"

echo "==> ensure project ${PROJECT}"
curl -sf -X POST "${PLATFORM_URL}/v1/projects" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"id\":\"${PROJECT}\"}" >/dev/null 2>&1 || true

PROD=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${PLATFORM_URL}/v1/projects/${PROJECT}" | jq -r '.prod_version_id // empty')
if [[ -z "$PROD" ]]; then
  echo "==> seed demo (first time)"
  ./dev/scripts/seed-demo.sh || true
  PROD=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${PLATFORM_URL}/v1/projects/${PROJECT}" | jq -r '.prod_version_id // empty')
fi

if [[ -n "$PROD" ]]; then
  echo "==> promote ${PROD} (refresh prod ingress binding)"
  curl -sf -X POST "${PLATFORM_URL}/v1/projects/${PROJECT}/versions/${PROD}/promote" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null || true
fi

echo "==> health"
./dev/scripts/health.sh

PROD_CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: ${PROJECT}.ingress.local" "${GATEWAY_URL}/health" 2>/dev/null || echo 000)
PREV_CODE=000
if [[ -n "${PROD:-}" ]]; then
  PREV_CODE=$(curl -s -o /dev/null -w '%{http_code}' -H "Host: ${PROD}.${PROJECT}.ingress.local" "${GATEWAY_URL}/" 2>/dev/null || echo 000)
fi

echo "==> gateway Host probe: prod=${PROD_CODE} preview=${PREV_CODE}"

DASH_PORT="${DASHBOARD_PORT:-5190}"
for p in "$DASH_PORT" 5191; do
  pid=$(lsof -ti "tcp:${p}" 2>/dev/null || true)
  [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
done
sleep 1

echo "==> dashboard :${DASH_PORT}"
cd web
nohup env DASHBOARD_PORT="$DASH_PORT" npm run dev >> "$ROOT/dev/data/logs/dashboard.log" 2>&1 &
echo $! > "$ROOT/dev/data/pids/dashboard.pid"
cd "$ROOT"
for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${DASH_PORT}/" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

cat <<EOF

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Ready for UI check

  Dashboard:  http://127.0.0.1:${DASH_PORT}/
  Project:    http://127.0.0.1:${DASH_PORT}/projects/${PROJECT}
  commerce:   http://127.0.0.1:${DASH_PORT}/projects/commerce-store

  API:        ${PLATFORM_URL}
  Gateway:    ${GATEWAY_URL}  (Host routing)

  Optional /etc/hosts (direct gateway in browser):
    127.0.0.1 ${PROJECT}.ingress.local
EOF
if [[ -n "${PROD:-}" ]]; then
  echo "    127.0.0.1 ${PROD}.${PROJECT}.ingress.local"
fi
cat <<EOF

  Logs: dev/data/logs/cellpd.log · dashboard.log
  Stop dashboard: kill \$(cat dev/data/pids/dashboard.pid)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
