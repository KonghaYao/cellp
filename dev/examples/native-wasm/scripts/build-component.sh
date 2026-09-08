#!/usr/bin/env bash
# Reproducibly build the native-http-v1 fixture component into ../component.wasm.
set -euo pipefail
REPO="$(cd "$(dirname "$0")/../../../.." && pwd)"
GUEST="$REPO/dev/examples/native-wasm/guest"
OUT="$REPO/dev/examples/native-wasm/component.wasm"
WIT_SRC="$REPO/celld/crates/celld/native/wit"

need() { command -v "$1" >/dev/null || { echo "MISSING: $1" >&2; exit 1; }; }
need cargo
need rustup

rustup target add wasm32-wasip2 >/dev/null
mkdir -p "$GUEST/wit/deps"
rsync -a --delete "$WIT_SRC/deps/" "$GUEST/wit/deps/"
rm -f "$GUEST/wit/native-http-v1.wit"
cat >"$GUEST/wit/world.wit" <<'WIT'
package cellp:fixture@0.1.0;

world fixture {
  import cellp:config/config@0.1.0;
  import cellp:kv/kv@0.1.0;
  export wasi:http/incoming-handler@0.2.12;
}
WIT

cd "$GUEST"
cargo build --target wasm32-wasip2 --release --locked 2>/dev/null || cargo build --target wasm32-wasip2 --release
install -m 0644 "$GUEST/target/wasm32-wasip2/release/cellp_native_wasm_guest.wasm" "$OUT"
shasum -a 256 "$OUT" | awk '{print $1}' >"$REPO/dev/examples/native-wasm/component.wasm.sha256"
echo "built $OUT sha256=$(cat "$REPO/dev/examples/native-wasm/component.wasm.sha256")"
