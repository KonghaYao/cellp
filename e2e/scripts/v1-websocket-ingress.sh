#!/usr/bin/env bash
# WS-M1 — Gateway Host ingress WebSocket upgrade → celld wsecho (101 + one text frame echo).
# Not in run-all.sh by default (requires wsecho fixture + running stack).
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "$0")/lib.sh"
# shellcheck disable=SC1091
source "$(dirname "$0")/lib-ingress.sh"

require_stack_or_skip

PROJECT="${DEV_PROJECT}"
VERSION="$(unique_id)"
WSECHO="${E2E_ROOT}/celld/examples/wsecho"
DEST="${ARTIFACTS_DIR}/${PROJECT}/${VERSION}"
LOG="${EVIDENCE_DIR}/v1-websocket-ingress-e2e.log"

mkdir -p "$EVIDENCE_DIR"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

log "WS-M1 websocket ingress project=${PROJECT} version=${VERSION}"

if [[ ! -d "$WSECHO" ]]; then
  fail "missing wsecho fixture: ${WSECHO} (see celld/examples/README.md)"
fi

ensure_project "$PROJECT"
cleanup_e2e_versions "$PROJECT"

stage_worker_example "$WSECHO" "$DEST"
create_version "$PROJECT" "$VERSION" | jq -r .id >/dev/null
poll_version "$PROJECT" "$VERSION" ready 120 >/dev/null

PREVIEW="$(preview_host "$PROJECT" "$VERSION")"
GW_BASE="${GATEWAY_URL%/}"

# --- 101 handshake via Gateway (product path) ---
WS_KEY="dGhlIHNhbXBsZSBub25jZQ=="
HDR_FILE=$(mktemp)
# shellcheck disable=SC2046
curl -sS -D "$HDR_FILE" -o /dev/null --max-time 8 $(gateway_curl_tls_flags) \
  -H "Host: ${PREVIEW}" \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: ${WS_KEY}" \
  "${GW_BASE}/" || fail "curl upgrade handshake failed"

STATUS=$(head -1 "$HDR_FILE" | awk '{print $2}')
rm -f "$HDR_FILE"
[[ "$STATUS" == "101" ]] || fail "Gateway Upgrade expected HTTP 101, got ${STATUS}"

# --- one text frame echo (stdlib Python RFC6455 client) ---
MSG="cellp-ws-m1"
REPLY=$(python3 - "$GW_BASE" "$PREVIEW" "$MSG" <<'PY'
import base64, hashlib, os, socket, struct, sys, urllib.parse

gateway_base, host, message = sys.argv[1], sys.argv[2], sys.argv[3]
parsed = urllib.parse.urlparse(gateway_base)
port = parsed.port or (443 if parsed.scheme == "https" else 80)
path = "/"

key = base64.b64encode(os.urandom(16)).decode()
req = (
    f"GET {path} HTTP/1.1\r\n"
    f"Host: {host}\r\n"
    f"Upgrade: websocket\r\n"
    f"Connection: Upgrade\r\n"
    f"Sec-WebSocket-Version: 13\r\n"
    f"Sec-WebSocket-Key: {key}\r\n"
    f"\r\n"
).encode()

s = socket.create_connection((parsed.hostname or "127.0.0.1", port), timeout=30)
s.sendall(req)
buf = b""
while b"\r\n\r\n" not in buf:
    chunk = s.recv(4096)
    if not chunk:
        raise SystemExit("handshake EOF")
    buf += chunk
if b" 101 " not in buf.split(b"\r\n", 1)[0]:
    raise SystemExit("not 101: " + buf[:200].decode("latin-1", "replace"))

payload = message.encode("utf-8")
mask = os.urandom(4)
masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
frame = bytes([0x81, 0x80 | len(payload)]) + mask + masked
s.sendall(frame)

data = s.recv(65536)
s.close()
if len(data) < 2:
    raise SystemExit("short frame")
plen = data[1] & 0x7F
off = 2
if plen == 126:
    plen = struct.unpack("!H", data[2:4])[0]
    off = 4
elif plen == 127:
    plen = struct.unpack("!Q", data[2:10])[0]
    off = 10
text = data[off : off + plen].decode("utf-8", "replace")
print(text)
PY
)

echo "$REPLY" | jq -e --arg m "$MSG" '.echo == $m and (.count | type) == "number"' >/dev/null \
  || fail "echo frame mismatch: ${REPLY}"

log "WS-M1 websocket ingress PASS (101 + echo)"
pass "v1-websocket-ingress"
