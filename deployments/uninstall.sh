#!/usr/bin/env bash

set -euo pipefail

INSTALL_DIR="${REPROXY_INSTALL_DIR:-/opt/reproxy}"
SERVICE_PATH="${REPROXY_SERVICE_PATH:-/etc/systemd/system/reproxy.service}"
ENV_PATH="${INSTALL_DIR}/deployments/env/reproxy.env"
KEEP_STATE="${REPROXY_KEEP_STATE:-0}"
SKIP_SYSTEMD="${REPROXY_SKIP_SYSTEMD:-0}"
BACKUP_DIR=""

if [[ "${EUID}" -ne 0 ]]; then
  if [[ "${SKIP_SYSTEMD}" == "1" && -w "$(dirname "${INSTALL_DIR}")" && -w "$(dirname "${SERVICE_PATH}")" ]]; then
    :
  else
    echo "This uninstaller needs root privileges. Re-run with sudo or as root." >&2
    exit 1
  fi
fi

if [[ -f "${ENV_PATH}" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "${ENV_PATH}"
  set +a
fi

if [[ "${SKIP_SYSTEMD}" != "1" ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now reproxy >/dev/null 2>&1 || true
fi

if [[ "${KEEP_STATE}" == "1" ]]; then
  BACKUP_DIR="$(mktemp -d /tmp/reproxy-state-XXXXXX)"
  if [[ -f "${ENV_PATH}" ]]; then
    mkdir -p "${BACKUP_DIR}/deployments/env"
    cp -a "${ENV_PATH}" "${BACKUP_DIR}/deployments/env/reproxy.env"
  fi
  if [[ -d "${INSTALL_DIR}/data" ]]; then
    mkdir -p "${BACKUP_DIR}"
    cp -a "${INSTALL_DIR}/data" "${BACKUP_DIR}/data"
  fi
fi

if [[ -n "${REPROXY_NGINX_CONFIG_PATH:-}" ]]; then
  rm -f "${REPROXY_NGINX_CONFIG_PATH}"
fi

rm -f "${SERVICE_PATH}"
rm -rf "${INSTALL_DIR}"

if [[ "${SKIP_SYSTEMD}" != "1" ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
  systemctl reload nginx >/dev/null 2>&1 || true
fi

cat <<EOF
Removed reproxy from ${INSTALL_DIR}
Removed service file ${SERVICE_PATH}
EOF

if [[ -n "${BACKUP_DIR}" ]]; then
  echo "Preserved env/data in ${BACKUP_DIR}"
fi
