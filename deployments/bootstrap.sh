#!/usr/bin/env bash

set -euo pipefail

REPO_OWNER="${REPROXY_REPO_OWNER:-utada1stlove}"
REPO_NAME="${REPROXY_REPO_NAME:-reproxy}"
REPO_REF="${REPROXY_REPO_REF:-main}"
INSTALL_DIR="${REPROXY_INSTALL_DIR:-/opt/reproxy}"
SERVICE_PATH="${REPROXY_SERVICE_PATH:-/etc/systemd/system/reproxy.service}"
BOOTSTRAP_SOURCE_DIR="${REPROXY_BOOTSTRAP_SOURCE_DIR:-}"
SKIP_DEP_INSTALL="${REPROXY_SKIP_DEP_INSTALL:-0}"
SKIP_START="${REPROXY_SKIP_START:-0}"
INSTALL_NGINX="${REPROXY_INSTALL_NGINX:-1}"
INSTALL_CERTBOT="${REPROXY_INSTALL_CERTBOT:-1}"

require_root_if_needed() {
  if [[ "${EUID}" -eq 0 ]]; then
    return
  fi

  if [[ "${SKIP_DEP_INSTALL}" != "1" ]]; then
    echo "This installer needs root privileges for package installation. Re-run with: curl ... | sudo bash" >&2
    exit 1
  fi

  if [[ ! -w "$(dirname "${INSTALL_DIR}")" || ! -w "$(dirname "${SERVICE_PATH}")" ]]; then
    echo "This installer needs root privileges for ${INSTALL_DIR} or ${SERVICE_PATH}. Re-run with: curl ... | sudo bash" >&2
    exit 1
  fi
}

detect_package_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    echo apt
    return
  fi

  if command -v dnf >/dev/null 2>&1; then
    echo dnf
    return
  fi

  if command -v yum >/dev/null 2>&1; then
    echo yum
    return
  fi

  if command -v pacman >/dev/null 2>&1; then
    echo pacman
    return
  fi

  echo none
}

install_packages() {
  local manager="$1"
  shift

  if [[ "$#" -eq 0 ]]; then
    return
  fi

  case "${manager}" in
    apt)
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
      ;;
    dnf)
      dnf install -y "$@"
      ;;
    yum)
      yum install -y "$@"
      ;;
    pacman)
      pacman -Sy --noconfirm "$@"
      ;;
    *)
      echo "No supported package manager found. Please install these packages manually: $*" >&2
      exit 1
      ;;
  esac
}

ensure_dependencies() {
  if [[ "${SKIP_DEP_INSTALL}" == "1" ]]; then
    return
  fi

  local manager
  manager="$(detect_package_manager)"
  local packages=()

  if ! command -v tar >/dev/null 2>&1; then
    case "${manager}" in
      apt|dnf|yum|pacman) packages+=("tar") ;;
    esac
  fi

  if ! command -v go >/dev/null 2>&1; then
    case "${manager}" in
      apt) packages+=("golang-go") ;;
      dnf|yum|pacman) packages+=("go") ;;
    esac
  fi

  if [[ "${INSTALL_NGINX}" == "1" ]] && ! command -v nginx >/dev/null 2>&1; then
    packages+=("nginx")
  fi

  if [[ "${INSTALL_CERTBOT}" == "1" ]] && ! command -v certbot >/dev/null 2>&1; then
    packages+=("certbot")
  fi

  install_packages "${manager}" "${packages[@]}"
}

fetch_source() {
  if [[ -n "${BOOTSTRAP_SOURCE_DIR}" ]]; then
    printf '%s\n' "${BOOTSTRAP_SOURCE_DIR}"
    return
  fi

  local workdir archive_url extracted
  workdir="$(mktemp -d)"
  archive_url="https://codeload.github.com/${REPO_OWNER}/${REPO_NAME}/tar.gz/refs/heads/${REPO_REF}"

  curl -fsSL "${archive_url}" | tar -xz -C "${workdir}"
  extracted="${workdir}/${REPO_NAME}-${REPO_REF}"

  if [[ ! -d "${extracted}" ]]; then
    extracted="$(find "${workdir}" -maxdepth 1 -mindepth 1 -type d | head -n 1)"
  fi

  if [[ -z "${extracted}" || ! -d "${extracted}" ]]; then
    echo "Failed to unpack ${archive_url}" >&2
    exit 1
  fi

  printf '%s\n' "${extracted}"
}

start_service() {
  if [[ "${SKIP_START}" == "1" ]]; then
    echo "Skipping service enable/start because REPROXY_SKIP_START=1"
    return
  fi

  if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemctl not found. Install completed, but service was not enabled." >&2
    return
  fi

  systemctl daemon-reload
  systemctl enable --now reproxy
}

main() {
  require_root_if_needed
  ensure_dependencies

  local source_dir
  source_dir="$(fetch_source)"

  bash "${source_dir}/deployments/install.sh" "${INSTALL_DIR}" "${SERVICE_PATH}"
  start_service

  cat <<EOF

Bootstrap finished.
Install dir: ${INSTALL_DIR}
Service file: ${SERVICE_PATH}
Environment file: ${INSTALL_DIR}/deployments/env/reproxy.env

Quick checks:
- systemctl status reproxy
- curl http://127.0.0.1:8080/status
EOF
}

main "$@"
