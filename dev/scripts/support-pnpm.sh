# shellcheck shell=bash
# Shared pnpm helpers for the cellp repo (root workspace + support corpus / dev/examples).
# Source from repo root: source dev/scripts/support-pnpm.sh

cellp_ensure_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    return 0
  fi
  if command -v corepack >/dev/null 2>&1; then
    corepack enable 2>/dev/null || true
    corepack prepare pnpm@11.11.0 --activate 2>/dev/null || true
  fi
  if ! command -v pnpm >/dev/null 2>&1; then
    echo "FAIL: pnpm not on PATH (install Node 20+ and enable corepack)" >&2
    return 1
  fi
}

cellp_pnpm_install_flags() {
  # Respect same env as legacy npm postinstall skip (workerd binary).
  if [[ "${NPM_CONFIG_IGNORE_SCRIPTS:-}" == "true" ]] || [[ "${npm_config_ignore_scripts:-}" == "true" ]]; then
    echo --ignore-scripts
  fi
}

cellp_pnpm_install() {
  cellp_ensure_pnpm || return 1
  local -a flags=()
  local ign
  ign="$(cellp_pnpm_install_flags)"
  [[ -n "$ign" ]] && flags+=("$ign")
  if [[ -f pnpm-lock.yaml ]]; then
    pnpm install --frozen-lockfile "${flags[@]}" 2>/dev/null || pnpm install "${flags[@]}"
  else
    pnpm install "${flags[@]}"
  fi
}
