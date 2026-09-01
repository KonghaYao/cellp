#!/usr/bin/env bash
exec "$(dirname "$0")/ingress-magic-dns-enable.sh" --nip "$@"
