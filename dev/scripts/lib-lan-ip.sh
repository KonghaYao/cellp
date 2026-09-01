#!/usr/bin/env bash
# Detect a likely LAN IPv4 for nip.io / LAN ingress (macOS + Linux).
set -euo pipefail

detect_lan_ip() {
  if [[ -n "${CELLP_LAN_IP:-}" ]]; then
    echo "$CELLP_LAN_IP"
    return 0
  fi

  if command -v ipconfig >/dev/null 2>&1; then
    for iface in en0 en1 en2 bridge0; do
      ip="$(ipconfig getifaddr "$iface" 2>/dev/null || true)"
      if [[ -n "$ip" && "$ip" != 127.0.0.1 ]]; then
        echo "$ip"
        return 0
      fi
    done
  fi

  if command -v ip >/dev/null 2>&1; then
    ip="$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") print $(i + 1)}' | head -1)"
    if [[ -n "$ip" && "$ip" != 127.0.0.1 ]]; then
      echo "$ip"
      return 0
    fi
  fi

  return 1
}

ip_to_nip_domain() {
  local ip="$1"
  echo "${ip//./-}.nip.io"
}

ip_to_sslip_domain() {
  local ip="$1"
  echo "${ip}.sslip.io"
}

magic_dns_domain() {
  local ip="$1"
  local provider="${2:-sslip}"
  case "$provider" in
    nip | nip.io) ip_to_nip_domain "$ip" ;;
    sslip | sslip.io) ip_to_sslip_domain "$ip" ;;
    *)
      echo "unknown magic DNS provider: $provider (use nip or sslip)" >&2
      return 1
      ;;
  esac
}

probe_magic_dns() {
  local ip="$1"
  local provider="$2"
  local domain
  domain="$(magic_dns_domain "$ip" "$provider")"
  command -v dig >/dev/null 2>&1 || return 1
  dig +short +time=2 +tries=1 "probe.${domain}" A 2>/dev/null | grep -qx "$ip"
}
