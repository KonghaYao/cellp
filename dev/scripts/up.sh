#!/usr/bin/env bash
# Start local dev stack
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if [[ ! -f dev/.env ]]; then
  cp dev/.env.example dev/.env
  echo "Created dev/.env from example"
fi

set -a
# shellcheck disable=SC1091
source dev/.env
set +a

mkdir -p dev/data/{artifacts,offshoot-store,offshoot-checkouts,celld-watch,pids,logs}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "MISSING: $1 — $2" >&2
    exit 1
  fi
}

need docker "https://docs.docker.com/get-docker/"
need curl
need jq

optional() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "WARN: $1 not found — $2"
    return 1
  fi
}

echo "==> docker compose up"
docker compose -f dev/docker-compose.yml --env-file dev/.env up -d

echo "==> wait for RustFS"
for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${S3_PORT:-9000}/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

# Platform: cellpd first; mock only when CELLP_USE_MOCK=1 (see docs/plans/phase-3-e2e.md P3-T3)
platform_running() {
  [[ -f dev/data/pids/platform.pid ]] && kill -0 "$(cat dev/data/pids/platform.pid)" 2>/dev/null
}

stop_platform() {
  if [[ -f dev/data/pids/platform.pid ]]; then
    kill "$(cat dev/data/pids/platform.pid)" 2>/dev/null || true
    rm -f dev/data/pids/platform.pid
  fi
}

CELLPD_BIN=""
CELLPD_BUILT=0
if [[ -d "${ROOT}/cellp/cmd/cellpd" ]]; then
  if [[ ! -x "${ROOT}/dev/data/cellpd" ]] || find "${ROOT}/cellp" -name '*.go' -newer "${ROOT}/dev/data/cellpd" 2>/dev/null | grep -q .; then
    echo "==> build cellpd"
    if "$ROOT/dev/scripts/build-cellpd.sh"; then
      CELLPD_BUILT=1
    else
      echo "WARN: cellpd build failed" >&2
    fi
  fi
fi
if [[ -x "${ROOT}/dev/data/cellpd" ]]; then
  CELLPD_BIN="${ROOT}/dev/data/cellpd"
fi

PLATFORM_MODE="none"
if [[ -n "$CELLPD_BIN" ]]; then
  PLATFORM_MODE="cellpd"
  if platform_running && [[ "$CELLPD_BUILT" -eq 1 ]]; then
    echo "==> restart cellpd (new binary)"
    stop_platform
  fi
  if platform_running && [[ "${CELLP_RESTART_CELLPD:-0}" == "1" ]]; then
    echo "==> restart cellpd (CELLP_RESTART_CELLPD=1)"
    stop_platform
  fi
  if platform_running; then
    echo "==> cellpd already running (pid $(cat dev/data/pids/platform.pid))"
  else
    stop_platform
    echo "==> start cellpd API :${PLATFORM_PORT} Gateway :${GATEWAY_PORT:-8787}"
    export CELLP_REGISTRY_DB="${CELLP_REGISTRY_DB:-${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}}"
    export CELLP_DEPLOY_TOKEN="${CELLP_DEPLOY_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
    export CELLP_ADMIN_TOKEN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
    export S3_ENDPOINT AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_REGION RUSTFS_ACCESS_KEY RUSTFS_SECRET_KEY OFFSHOOT_STORE
    export GATEWAY_TLS_PORT GATEWAY_TLS_CERT GATEWAY_TLS_KEY CELLP_PUBLIC_SCHEME_PREVIEW CELLP_PUBLIC_SCHEME_PROD CELLP_INGRESS_BASE_DOMAIN GATEWAY_URL CELLP_SKIP_CELLD_DIAGNOSE
    nohup "$CELLPD_BIN" >>dev/data/logs/cellpd.log 2>&1 &
    echo $! > dev/data/pids/platform.pid
  fi
elif [[ "${CELLP_USE_MOCK:-0}" == "1" ]]; then
  PLATFORM_MODE="mock"
  if platform_running; then
    echo "==> mock platform already running (pid $(cat dev/data/pids/platform.pid))"
  else
    echo "==> start mock platform (API + Gateway) :${PLATFORM_PORT} / :${GATEWAY_PORT:-8787}"
    nohup node dev/mock-platform/server.mjs >>dev/data/logs/platform.log 2>&1 &
    echo $! > dev/data/pids/platform.pid
  fi
else
  echo "SKIP: platform/gateway not started (no cellpd; set CELLP_USE_MOCK=1 for mock)"
fi

if [[ "$PLATFORM_MODE" != "none" ]]; then
  echo "==> wait for platform health"
  for i in $(seq 1 30); do
    curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1 && break
    sleep 1
  done
fi

# celld
if optional celld "curl -fsSL https://celld.dev/install.sh | sh"; then
  if [[ ! -f dev/data/pids/celld.pid ]] || ! kill -0 "$(cat dev/data/pids/celld.pid)" 2>/dev/null; then
    echo "==> celld deploy example (first run may take a minute)"
    export CELLD_VAR_PROJECT_ID="${DEV_PROJECT:-demo-app}"
    export CELLD_VAR_VERSION_ID="v-dev"
    (
      cd dev/examples/counter
      celld deploy . --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
        2>>"$ROOT/dev/data/logs/celld-deploy.log" || true
    )
    echo "==> celld storage probe (required for private RustFS)"
    if ! celld diagnose --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
      >>dev/data/logs/celld-diagnose.log 2>&1; then
      echo "WARN: celld diagnose failed — see dev/data/logs/celld-diagnose.log (RustFS conditional writes)"
    fi
    echo "==> start celld :${CELLD_PORT}"
    nohup celld --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
      --listen "127.0.0.1:${CELLD_PORT}" >>dev/data/logs/celld.log 2>&1 &
    echo $! > dev/data/pids/celld.pid
    for i in $(seq 1 60); do
      if curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1; then
        break
      fi
      sleep 1
    done
  fi
else
  echo "SKIP: celld not installed — gateway and platform will run; run simulate-cd after installing celld"
fi

# offshoot daemon (optional for data fork path)
if optional offshoot "go install github.com/sricola/offshoot/cmd/offshoot@latest"; then
  if [[ ! -f dev/data/pids/offshoot.pid ]] || ! kill -0 "$(cat dev/data/pids/offshoot.pid)" 2>/dev/null; then
    mkdir -p "$OFFSHOOT_STORE" "$OFFSHOOT_CHECKOUTS"
    if [[ ! -d "$OFFSHOOT_STORE/offshoot.json" ]] && [[ ! -f "$OFFSHOOT_STORE/offshoot.json" ]]; then
      (cd "$ROOT" && offshoot init "$OFFSHOOT_STORE" 2>/dev/null || offshoot --store "$OFFSHOOT_STORE" create "${DEV_PROJECT:-demo-app}" 2>/dev/null || true)
    fi
    echo "==> offshoot local store at $OFFSHOOT_STORE"
  fi
fi

echo ""
echo "Dev stack up."
if [[ "$PLATFORM_MODE" == "cellpd" ]]; then
  echo "  Platform: cellpd (API ${PLATFORM_URL}/v1/health · Gateway ${GATEWAY_URL})"
elif [[ "$PLATFORM_MODE" == "mock" ]]; then
  echo "  Platform: mock (CELLP_USE_MOCK=1 · API ${PLATFORM_URL}/v1/health · Gateway ${GATEWAY_URL})"
else
  echo "  Platform: (not started — build cellpd or CELLP_USE_MOCK=1 ./dev/scripts/up.sh)"
fi
echo "  Next:     ./dev/scripts/health.sh && ./dev/scripts/seed-demo.sh"
