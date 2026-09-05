#!/usr/bin/env bash
# Remove local node_modules under dev/examples and dev/support-corpus (gitignored).
# Frees disk from duplicate npm-installed workerd/wrangler native binaries.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

removed=0
while IFS= read -r -d '' d; do
  # Skip inside pnpm store if ever nested
  [[ "$d" == *"/.pnpm/"* ]] && continue
  echo "rm -rf ${d#"$ROOT"/}"
  rm -rf "$d"
  removed=$((removed + 1))
done < <(
  find "$ROOT/dev/examples" "$ROOT/dev/support-corpus" \
    -name node_modules -type d -prune -print0 2>/dev/null || true
)

echo "clean-support-node-modules: removed ${removed} node_modules trees"
if command -v du >/dev/null 2>&1; then
  du -sh "$HOME/.local/share/pnpm/store" 2>/dev/null || true
fi
