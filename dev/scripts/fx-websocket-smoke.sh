#!/usr/bin/env bash
# fx-on-workers WebSocket smoke — Gateway /session → DO FxSession (101 + session frames).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC1091
source dev/.env 2>/dev/null || source dev/.env.example

HOST="${FX_WS_HOST:-support-fx-on-workers.lvh.me}"
KEY="${FX_WS_KEY:-cellp-dev-fx-on-workers}"
SESSION_ID="${FX_WS_SESSION_ID:-smoke-test}"
GW_BASE="${GATEWAY_URL:-http://127.0.0.1:8787}"
GW_BASE="${GW_BASE%/}"
EVIDENCE="${ROOT}/docs/evidence/fx-websocket-worker-smoke.log"
mkdir -p "$(dirname "$EVIDENCE")"

log() { echo "[$(date -Iseconds)] $*"; }

RUN_ID="$(date -Iseconds)"
{
  echo "=== fx-websocket-smoke ${RUN_ID} ==="
  log "host=${HOST} session=${SESSION_ID} gateway=${GW_BASE}"

  WS_PATH="/session?id=${SESSION_ID}&key=${KEY}"
  WS_KEY_HDR="dGhlIHNhbXBsZSBub25jZQ=="
  HDR_FILE=$(mktemp)
  trap 'rm -f "$HDR_FILE"' EXIT

  # curl may time out after 101 while the WS stays open; headers are enough for this check.
  HTTP_CODE=$(curl -sS -D "$HDR_FILE" -o /dev/null --max-time 4 \
    -H "Host: ${HOST}" \
    -H "Connection: Upgrade" \
    -H "Upgrade: websocket" \
    -H "Sec-WebSocket-Version: 13" \
    -H "Sec-WebSocket-Key: ${WS_KEY_HDR}" \
    "${GW_BASE}${WS_PATH}" 2>/dev/null || echo "000")

  STATUS=$(head -1 "$HDR_FILE" 2>/dev/null | awk '{print $2}')
  log "curl Upgrade HTTP status=${STATUS:-?} code=${HTTP_CODE}"

  if [[ "${STATUS:-}" == "502" || "${HTTP_CODE}" == "502" ]]; then
    log "FAIL: Gateway returned 502 on WebSocket upgrade"
    echo "RESULT: FAIL (502)"
    exit 1
  fi
  if [[ "${STATUS:-}" != "101" ]]; then
    log "FAIL: expected HTTP 101, got ${STATUS:-none}"
    head -20 "$HDR_FILE" || true
    echo "RESULT: FAIL (not 101)"
    exit 1
  fi
  log "OK: HTTP 101 Switching Protocols"

  PY_OUT=$(python3 - "$GW_BASE" "$HOST" "$WS_PATH" <<'PY'
import base64, json, os, socket, struct, sys, time, urllib.parse

gateway_base, host, path = sys.argv[1], sys.argv[2], sys.argv[3]
parsed = urllib.parse.urlparse(gateway_base)
port = parsed.port or (443 if parsed.scheme == "https" else 80)
hostname = parsed.hostname or "127.0.0.1"

def send_text(sock, message: str) -> None:
    payload = message.encode("utf-8")
    mask = os.urandom(4)
    masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
    n = len(payload)
    if n < 126:
        header = bytes([0x81, 0x80 | n])
    elif n < 65536:
        header = bytes([0x81, 0xFE]) + struct.pack("!H", n)
    else:
        header = bytes([0x81, 0xFF]) + struct.pack("!Q", n)
    sock.sendall(header + mask + masked)

def recv_exact(sock, n: int) -> bytes:
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise EOFError("connection closed")
        buf += chunk
    return buf

def recv_server_frame(sock):
    b1, b2 = recv_exact(sock, 2)
    opcode = b1 & 0x0F
    masked = (b2 & 0x80) != 0
    length = b2 & 0x7F
    if length == 126:
        length = struct.unpack("!H", recv_exact(sock, 2))[0]
    elif length == 127:
        length = struct.unpack("!Q", recv_exact(sock, 8))[0]
    mask_key = recv_exact(sock, 4) if masked else b""
    payload = recv_exact(sock, length)
    if masked:
        payload = bytes(b ^ mask_key[i % 4] for i, b in enumerate(payload))
    return opcode, payload

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

s = socket.create_connection((hostname, port), timeout=30)
s.settimeout(2.0)
try:
    s.sendall(req)
    buf = b""
    while b"\r\n\r\n" not in buf:
        chunk = s.recv(4096)
        if not chunk:
            raise SystemExit(json.dumps({"error": "handshake_eof"}))
        buf += chunk
    status_line = buf.split(b"\r\n", 1)[0]
    if b" 101 " not in status_line:
        raise SystemExit(json.dumps({"error": "not_101", "line": status_line.decode("latin-1", "replace")}))

    prompt = json.dumps({"type": "prompt", "text": "echo cellp-ws-ok"})
    send_text(s, prompt)

    binary_count = 0
    text_events = []
    deadline = time.time() + 90
    needs_ai = False

    while time.time() < deadline:
        try:
            opcode, payload = recv_server_frame(s)
        except (socket.timeout, TimeoutError):
            continue
        except EOFError:
            break
        if opcode == 0x8:
            break
        if opcode == 0x9:
            # pong ping
            continue
        if opcode == 0x2:
            binary_count += 1
            continue
        if opcode == 0x1:
            text = payload.decode("utf-8", "replace")
            try:
                ev = json.loads(text)
            except json.JSONDecodeError:
                text_events.append({"type": "non_json", "raw": text[:200]})
                continue
            text_events.append(ev)
            msg = str(ev.get("message", ""))
            if "AI_GATEWAY_API_KEY" in msg:
                needs_ai = True
            continue

    good_json = [e for e in text_events if isinstance(e, dict) and e.get("type") not in (None, "malformed")]
    passed = binary_count >= 1 or len(good_json) >= 1

    out = {
        "binary_frames": binary_count,
        "text_event_count": len(text_events),
        "sample_events": text_events[:8],
        "needs_ai_gateway": needs_ai,
        "pass": passed,
    }
    print(json.dumps(out))
    sys.exit(0 if passed else 2)
finally:
    try:
        s.close()
    except OSError:
        pass
PY
) || PY_EXIT=$?

PY_EXIT=${PY_EXIT:-0}
echo "$PY_OUT"

if [[ "${PY_EXIT:-1}" -eq 2 ]] || [[ -z "${PY_OUT:-}" ]]; then
  log "FAIL: no session frames within 90s (or connection reset before frames)"
  echo "RESULT: FAIL (no frames)"
  exit 1
fi

BINARY=$(echo "$PY_OUT" | tail -1 | python3 -c "import json,sys; print(json.load(sys.stdin).get('binary_frames',0))")
TEXT_N=$(echo "$PY_OUT" | tail -1 | python3 -c "import json,sys; print(json.load(sys.stdin).get('text_event_count',0))")
NEEDS_AI=$(echo "$PY_OUT" | tail -1 | python3 -c "import json,sys; print(1 if json.load(sys.stdin).get('needs_ai_gateway') else 0)")
SAMPLES=$(echo "$PY_OUT" | tail -1 | python3 -c "import json,sys; print(json.dumps(json.load(sys.stdin).get('sample_events',[]), ensure_ascii=False))")

log "binary_frames=${BINARY} text_events=${TEXT_N}"
log "sample_events=${SAMPLES}"

if [[ "$NEEDS_AI" == "1" ]]; then
  log "NEEDS_AI_GATEWAY=1 (WS worker path OK; full fx turn needs AI_GATEWAY_API_KEY)"
  echo "NEEDS_AI_GATEWAY=1"
fi

echo "RESULT: PASS binary=${BINARY} text_events=${TEXT_N}"
echo "VERDICT: PASS worker_websocket_session=ok needs_ai_gateway=${NEEDS_AI}"
} 2>&1 | tee -a "$EVIDENCE"

# Propagate failure from subshell — tee masks exit code; re-check last RESULT line
if tail -5 "$EVIDENCE" | grep -q "RESULT: FAIL"; then
  exit 1
fi
exit 0
