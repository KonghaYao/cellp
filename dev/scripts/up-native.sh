#!/usr/bin/env bash
# Native dev harness — RustFS via compose or native binary; cellpd if built; celld + offshoot on host
# TP-DEV-1: replaces mock when cellpd exists; documents dependency otherwise
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
  command -v "$1" >/dev/null 2>&1 || {
    echo "MISSING: $1 — $2" >&2
    exit 1
  }
}

need curl
need jq

optional() {
  command -v "$1" >/dev/null 2>&1
}

S3_HEALTH_URL="http://127.0.0.1:${S3_PORT:-19000}/health"
RUSTFS_PID_FILE="dev/data/pids/rustfs.pid"
RUSTFS_BIN="/tmp/rustfs-bin/rustfs"
RUSTFS_DATA="${ROOT}/dev/data/rustfs"

rustfs_healthy() {
  curl -sf "$S3_HEALTH_URL" >/dev/null 2>&1
}

init_s3_buckets() {
  local aws_bin=""
  if optional aws; then
    aws_bin="aws"
  elif [[ -x "${HOME}/.local/bin/aws" ]]; then
    aws_bin="${HOME}/.local/bin/aws"
  else
    echo "WARN: aws CLI not found — skipping S3 bucket init (may fail celld deploy)" >&2
    return 0
  fi
  export AWS_ACCESS_KEY_ID="${RUSTFS_ACCESS_KEY:-rustfsadmin}"
  export AWS_SECRET_ACCESS_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}"
  export AWS_REGION="${AWS_REGION:-us-east-1}"
  local ep="${S3_ENDPOINT:-http://127.0.0.1:${S3_PORT:-19000}}"
  for bucket in cellp-celld cellp-offshoot cellp-artifacts; do
    "$aws_bin" --endpoint-url "$ep" s3 mb "s3://${bucket}" 2>/dev/null || true
  done
}

start_native_rustfs() {
  if [[ ! -x "$RUSTFS_BIN" ]]; then
    echo "WARN: native RustFS binary not found at ${RUSTFS_BIN}" >&2
    return 1
  fi
  mkdir -p "$RUSTFS_DATA"
  if [[ -f "$RUSTFS_PID_FILE" ]] && kill -0 "$(cat "$RUSTFS_PID_FILE")" 2>/dev/null; then
    echo "==> RustFS already running (pid $(cat "$RUSTFS_PID_FILE"))"
    return 0
  fi
  echo "==> start native RustFS on :${S3_PORT:-19000} (${RUSTFS_BIN})"
  RUSTFS_VOLUMES="$RUSTFS_DATA" \
  RUSTFS_ADDRESS="0.0.0.0:${S3_PORT:-19000}" \
  RUSTFS_CONSOLE_ADDRESS="0.0.0.0:${S3_CONSOLE_PORT:-9001}" \
  RUSTFS_CONSOLE_ENABLE=true \
  RUSTFS_ACCESS_KEY="${RUSTFS_ACCESS_KEY:-rustfsadmin}" \
  RUSTFS_SECRET_KEY="${RUSTFS_SECRET_KEY:-rustfsadmin}" \
  RUSTFS_UNSAFE_BYPASS_DISK_CHECK=true \
    "$RUSTFS_BIN" "$RUSTFS_DATA" >>dev/data/logs/rustfs.log 2>&1 &
  echo $! >"$RUSTFS_PID_FILE"
}

ensure_rustfs() {
  if rustfs_healthy; then
    echo "==> RustFS already healthy at ${S3_HEALTH_URL}"
    init_s3_buckets
    return 0
  fi

  if optional docker; then
    echo "==> docker compose up (rustfs + s3-init only)"
    docker compose -f dev/docker-compose.yml --env-file dev/.env up -d rustfs s3-init
  elif [[ -x "$RUSTFS_BIN" ]]; then
    start_native_rustfs || true
  else
    echo "WARN: docker unavailable and ${RUSTFS_BIN} missing — RustFS may be down" >&2
  fi

  echo "==> wait for RustFS"
  for i in $(seq 1 60); do
    if rustfs_healthy; then
      init_s3_buckets
      return 0
    fi
    sleep 1
  done

  echo "WARN: RustFS not healthy at ${S3_HEALTH_URL} — celld S3 paths may fail" >&2
}

ensure_rustfs

CELLPD_BIN=""
CELLPD_SRC="${ROOT}/cellp/cmd/cellpd"
if [[ -d "$CELLPD_SRC" ]]; then
  echo "==> build cellpd"
  if "$ROOT/dev/scripts/build-cellpd.sh"; then
    CELLPD_BIN="${ROOT}/dev/data/cellpd"
  else
    echo "WARN: cellpd build failed — falling back to mock platform" >&2
  fi
else
  cat >&2 <<'EOF'
DEPENDENCY: cellpd not present (expected at cellp/cmd/cellpd).
  Until cellpd lands, this script starts mock-platform for API+Gateway.
  See docs/plans/phase-1-backend-core.md and DESIGN.md §13.
EOF
fi

if [[ -n "$CELLPD_BIN" ]]; then
  if [[ -f dev/data/pids/platform.pid ]]; then
    kill "$(cat dev/data/pids/platform.pid)" 2>/dev/null || true
    rm -f dev/data/pids/platform.pid
  fi
  echo "==> start cellpd API :${PLATFORM_PORT} Gateway :${GATEWAY_PORT:-8787}"
  export CELLP_REGISTRY_DB="${CELLP_REGISTRY_DB:-${REGISTRY_DB:-./dev/data/cellp-registry.sqlite}}"
  export CELLP_DEPLOY_TOKEN="${CELLP_DEPLOY_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
  export CELLP_ADMIN_TOKEN="${CELLP_ADMIN_TOKEN:-${PLATFORM_TOKEN:-dev-local-token}}"
  export S3_ENDPOINT AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_REGION RUSTFS_ACCESS_KEY RUSTFS_SECRET_KEY OFFSHOOT_STORE
  export GATEWAY_TLS_PORT GATEWAY_TLS_CERT GATEWAY_TLS_KEY CELLP_PUBLIC_SCHEME_PREVIEW CELLP_PUBLIC_SCHEME_PROD CELLP_INGRESS_BASE_DOMAIN GATEWAY_URL
  "$CELLPD_BIN" >>dev/data/logs/cellpd.log 2>&1 &
  echo $! > dev/data/pids/platform.pid
else
  if [[ ! -f dev/data/pids/platform.pid ]] || ! kill -0 "$(cat dev/data/pids/platform.pid)" 2>/dev/null; then
    echo "==> start mock platform (API + Gateway) :${PLATFORM_PORT} / :${GATEWAY_PORT:-8787}"
    node dev/mock-platform/server.mjs >>dev/data/logs/platform.log 2>&1 &
    echo $! > dev/data/pids/platform.pid
  fi
fi

for i in $(seq 1 30); do
  curl -sf "${PLATFORM_URL}/v1/health" >/dev/null 2>&1 && break
  sleep 1
done

if optional celld; then
  if [[ -d dev/examples/counter ]] && [[ ! -x dev/examples/counter/node_modules/.bin/esbuild ]]; then
    echo "==> install counter esbuild (celld deploy dependency)"
    (cd dev/examples/counter && npm install --silent) || echo "WARN: npm install esbuild failed" >&2
  fi
  if [[ ! -f dev/data/pids/celld.pid ]] || ! kill -0 "$(cat dev/data/pids/celld.pid)" 2>/dev/null; then
    echo "==> celld deploy example"
    export CELLD_VAR_PROJECT_ID="${DEV_PROJECT:-demo-app}"
    export CELLD_VAR_VERSION_ID="v-dev"
    (
      cd dev/examples/counter
      celld deploy . --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
        2>>"$ROOT/dev/data/logs/celld-deploy.log" || true
    )
    echo "==> celld storage probe"
    celld diagnose --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
      >>dev/data/logs/celld-diagnose.log 2>&1 || \
      echo "WARN: celld diagnose failed — see dev/data/logs/celld-diagnose.log"
    echo "==> start celld :${CELLD_PORT}"
    celld --bucket "$CELLD_BUCKET" --endpoint "$S3_ENDPOINT" --region "$AWS_REGION" \
      --listen "127.0.0.1:${CELLD_PORT}" >>dev/data/logs/celld.log 2>&1 &
    echo $! > dev/data/pids/celld.pid
    for i in $(seq 1 60); do
      curl -sf "http://127.0.0.1:${CELLD_PORT}/.well-known/celld/health" >/dev/null 2>&1 && break
      sleep 1
    done
  fi
else
  cat >&2 <<'EOF'
DEPENDENCY: celld not installed.
  Install: curl -fsSL https://celld.dev/install.sh | sh
  Required for TP-V0a, TP-VE-*, and TP-V1–V7 worker routes.
EOF
fi

if optional offshoot; then
  mkdir -p "$OFFSHOOT_STORE" "$OFFSHOOT_CHECKOUTS"
  if [[ ! -d "$OFFSHOOT_STORE/offshoot.json" ]] && [[ ! -f "$OFFSHOOT_STORE/offshoot.json" ]]; then
    offshoot init "$OFFSHOOT_STORE" 2>/dev/null || \
      offshoot -store "$OFFSHOOT_STORE" create "${DEV_PROJECT:-demo-app}" 2>/dev/null || true
  fi
  echo "==> offshoot local store at $OFFSHOOT_STORE"
else
  cat >&2 <<'EOF'
DEPENDENCY: offshoot not installed.
  Install: go install github.com/sricola/offshoot/cmd/offshoot@latest
  Required for TP-V0b/V0d (RustFS) and TP-V1/V2/V6 data paths.
EOF
fi

echo ""
echo "Native dev stack up (up-native.sh)."
echo "  Gateway:  ${GATEWAY_URL}"
echo "  Platform: ${PLATFORM_URL}/v1/health"
echo "  cellpd:   ${CELLPD_BIN:-mock-platform (see DEPENDENCY above)}"
echo "  Next:     ./dev/scripts/health.sh && ./e2e/scripts/run-all.sh"
