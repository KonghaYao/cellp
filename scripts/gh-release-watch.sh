#!/usr/bin/env bash
# Watch GitHub Actions for a release tag (CI + Release + Docker Publish).
set -euo pipefail

REPO="${CELLP_GH_REPO:-KonghaYao/cellp}"
TAG="${1:-v1.0.0}"

if ! command -v gh >/dev/null 2>&1; then
  echo "gh CLI required: brew install gh && gh auth login" >&2
  exit 1
fi

echo "Repository: $REPO"
echo "Tag: $TAG"
echo "Open: https://github.com/${REPO}/releases/tag/${TAG}"
echo ""

for workflow in CI Release "Docker Publish"; do
  echo "==> Latest run: $workflow"
  gh run list -R "$REPO" -w "$workflow" -L 3
  echo ""
done

echo "Watching in-flight runs (Ctrl+C to stop)..."
while true; do
  pending="$(gh run list -R "$REPO" -L 20 --json status,workflowName,headBranch,conclusion \
    -q '.[] | select(.status=="in_progress" or .status=="queued") | "\(.workflowName) \(.headBranch)"' || true)"
  if [[ -z "$pending" ]]; then
    echo "All tracked runs finished."
    gh run list -R "$REPO" -w Release -L 1
    gh run list -R "$REPO" -w "Docker Publish" -L 1
    exit 0
  fi
  echo "$(date -u +%H:%M:%S)Z still running:"
  echo "$pending"
  sleep 30
done
