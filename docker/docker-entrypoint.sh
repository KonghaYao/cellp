#!/bin/sh
set -eu

CERT_DIR="${CELLP_ELASTIC_CERT_DIR:-/data/certs/elastic}"

if [ ! -f "${CERT_DIR}/agent-server.pem" ]; then
  echo "==> generating embedded Node Agent mTLS certs in ${CERT_DIR}"
  mkdir -p "${CERT_DIR}"
  genelasticdevcerts --dir "${CERT_DIR}"
fi

exec cellpd "$@"
