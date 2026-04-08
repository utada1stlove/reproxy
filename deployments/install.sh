#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="${1:-/opt/reproxy}"
SERVICE_PATH="${2:-/etc/systemd/system/reproxy.service}"
ENV_PATH="${INSTALL_DIR}/deployments/env/reproxy.env"
BINARY_SOURCE="${ROOT_DIR}/bin/reproxy"

mkdir -p "${INSTALL_DIR}/bin" "${INSTALL_DIR}/data" "${INSTALL_DIR}/deployments/env"

if [[ -x "${BINARY_SOURCE}" ]]; then
  install -m 0755 "${BINARY_SOURCE}" "${INSTALL_DIR}/bin/reproxy"
else
  (
    cd "${ROOT_DIR}"
    env \
      GOCACHE="${GOCACHE:-/tmp/reproxy-go-cache}" \
      GOMODCACHE="${GOMODCACHE:-/tmp/reproxy-go-mod-cache}" \
      go build -o "${INSTALL_DIR}/bin/reproxy" ./cmd/reproxy
  )
fi

if [[ ! -f "${ENV_PATH}" ]]; then
  install -m 0644 "${ROOT_DIR}/deployments/env/reproxy.env.example" "${ENV_PATH}"
fi

install -m 0644 "${ROOT_DIR}/deployments/systemd/reproxy.service" "${SERVICE_PATH}"

cat <<EOF
Installed reproxy into ${INSTALL_DIR}
Environment file: ${ENV_PATH}
Systemd service: ${SERVICE_PATH}

Next steps:
1. Edit ${ENV_PATH}
2. systemctl daemon-reload
3. systemctl enable --now reproxy
EOF
