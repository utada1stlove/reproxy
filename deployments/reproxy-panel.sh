#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${REPROXY_ENV_FILE:-/opt/reproxy/deployments/env/reproxy.env}"
API_BASE="${REPROXY_API_BASE:-}"

load_env() {
  if [[ -f "${ENV_FILE}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${ENV_FILE}"
    set +a
  fi

  if [[ -z "${API_BASE}" ]]; then
    API_BASE="$(derive_api_base "${REPROXY_LISTEN_ADDR:-:8080}")"
  fi
}

derive_api_base() {
  local listen_addr="$1"
  listen_addr="${listen_addr#http://}"
  listen_addr="${listen_addr#https://}"

  case "${listen_addr}" in
    :*)
      printf 'http://127.0.0.1%s\n' "${listen_addr}"
      ;;
    0.0.0.0:*)
      printf 'http://127.0.0.1:%s\n' "${listen_addr##*:}"
      ;;
    *)
      printf 'http://%s\n' "${listen_addr}"
      ;;
  esac
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '"%s"' "${value}"
}

pretty_print() {
  local payload="${1:-}"

  if [[ -z "${payload}" ]]; then
    return
  fi

  if command -v python3 >/dev/null 2>&1; then
    if printf '%s\n' "${payload}" | python3 -m json.tool; then
      return
    fi
  fi

  printf '%s\n' "${payload}"
}

request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local response_file http_code payload

  response_file="$(mktemp)"
  payload=""

  if [[ -n "${body}" ]]; then
    http_code="$(curl -sS -X "${method}" "${API_BASE}${path}" -H 'Content-Type: application/json' -d "${body}" -o "${response_file}" -w '%{http_code}')"
  else
    http_code="$(curl -sS -X "${method}" "${API_BASE}${path}" -o "${response_file}" -w '%{http_code}')"
  fi

  if [[ -f "${response_file}" ]]; then
    payload="$(cat "${response_file}")"
    rm -f "${response_file}"
  fi

  if [[ "${http_code}" -ge 400 ]]; then
    printf 'API request failed with HTTP %s\n' "${http_code}" >&2
    if [[ -n "${payload}" ]]; then
      pretty_print "${payload}" >&2 || true
    fi
    return 1
  fi

  printf '%s' "${payload}"
}

run_request() {
  local response

  response="$(request "$@")" || return
  pretty_print "${response}"
}

pause() {
  printf '\n'
  read -r -p "Press Enter to continue..." _
}

prompt() {
  local label="$1"
  local default="${2:-}"
  local value

  if [[ -n "${default}" ]]; then
    read -r -p "${label} [${default}]: " value
    printf '%s' "${value:-$default}"
    return
  fi

  read -r -p "${label}: " value
  printf '%s' "${value}"
}

show_status() {
  printf '\n== Status ==\n'
  run_request GET "/status"
}

list_routes() {
  printf '\n== Routes ==\n'
  run_request GET "/routes"
}

create_domain_route() {
  printf '\nCreate a domain route\n'
  local domain name upstream_mode target_ip target_host target_port target_scheme host_header sni payload
  domain="$(prompt 'Domain' '')"
  name="$(prompt 'Route Name' "${domain}")"
  printf 'Upstream Mode:\n1) ip_port\n2) host\n'
  local mode_choice
  mode_choice="$(prompt 'Choose upstream mode' '1')"
  if [[ "${mode_choice}" == "2" ]]; then
    upstream_mode="host"
    target_host="$(prompt 'Target Host' '')"
    target_scheme="$(prompt 'Target Scheme (http/https)' 'https')"
    target_port="$(prompt 'Target Port (blank for default)' '')"
    host_header="$(prompt 'Upstream Host Header' "${target_host}")"
    sni=""
    if [[ "${target_scheme}" == "https" ]]; then
      sni="$(prompt 'Upstream SNI' "${target_host}")"
    fi

    payload=$(cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"domain","domain":$(json_escape "${domain}"),"upstream_mode":"host","target_host":$(json_escape "${target_host}"),"target_scheme":$(json_escape "${target_scheme}"),"target_port":${target_port:-0},"upstream_host_header":$(json_escape "${host_header}"),"upstream_sni":$(json_escape "${sni}")}
EOF
)
  else
    upstream_mode="ip_port"
    target_ip="$(prompt 'Target IP' '')"
    target_port="$(prompt 'Target Port' '')"
    host_header="$(prompt 'Upstream Host Header' '$host')"
    payload=$(cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"domain","domain":$(json_escape "${domain}"),"upstream_mode":"ip_port","target_ip":$(json_escape "${target_ip}"),"target_port":${target_port},"upstream_host_header":$(json_escape "${host_header}")}
EOF
)
  fi

  run_request POST "/routes" "${payload}"
}

create_port_route() {
  printf '\nCreate a port listener route\n'
  local name listen_ip listen_port target_host target_ip target_port target_scheme host_header sni mode_choice payload
  listen_port="$(prompt 'Listen Port' '')"
  name="$(prompt 'Route Name' "port-${listen_port}")"
  listen_ip="$(prompt 'Listen IP (blank for all interfaces)' '')"
  printf 'Upstream Mode:\n1) ip_port\n2) host\n'
  mode_choice="$(prompt 'Choose upstream mode' '2')"

  if [[ "${mode_choice}" == "1" ]]; then
    target_ip="$(prompt 'Target IP' '')"
    target_port="$(prompt 'Target Port' '')"
    host_header="$(prompt 'Upstream Host Header' '$host')"
    payload=$(cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"port","listen_ip":$(json_escape "${listen_ip}"),"listen_port":${listen_port},"upstream_mode":"ip_port","target_ip":$(json_escape "${target_ip}"),"target_port":${target_port},"upstream_host_header":$(json_escape "${host_header}")}
EOF
)
  else
    target_host="$(prompt 'Target Host' '')"
    target_scheme="$(prompt 'Target Scheme (http/https)' 'https')"
    target_port="$(prompt 'Target Port (blank for default)' '')"
    host_header="$(prompt 'Upstream Host Header' "${target_host}")"
    sni=""
    if [[ "${target_scheme}" == "https" ]]; then
      sni="$(prompt 'Upstream SNI' "${target_host}")"
    fi

    payload=$(cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"port","listen_ip":$(json_escape "${listen_ip}"),"listen_port":${listen_port},"upstream_mode":"host","target_host":$(json_escape "${target_host}"),"target_scheme":$(json_escape "${target_scheme}"),"target_port":${target_port:-0},"upstream_host_header":$(json_escape "${host_header}"),"upstream_sni":$(json_escape "${sni}")}
EOF
)
  fi

  run_request POST "/routes" "${payload}"
}

update_route() {
  local name payload
  printf '\nCurrent route details\n'
  name="$(prompt 'Route Name to update' '')"
  run_request GET "/routes/${name}" || return

  printf '\nChoose update style:\n1) Replace as domain route\n2) Replace as port route\n'
  local choice
  choice="$(prompt 'Choice' '1')"
  if [[ "${choice}" == "2" ]]; then
    printf 'The current route will be replaced using port-listener settings.\n'
    payload="$(build_port_update_payload "${name}")"
  else
    printf 'The current route will be replaced using domain settings.\n'
    payload="$(build_domain_update_payload "${name}")"
  fi

  run_request PUT "/routes/${name}" "${payload}"
}

build_domain_update_payload() {
  local name="$1"
  local domain upstream_mode target_ip target_host target_port target_scheme host_header sni mode_choice
  domain="$(prompt 'Domain' '')"
  printf 'Upstream Mode:\n1) ip_port\n2) host\n'
  mode_choice="$(prompt 'Choose upstream mode' '1')"

  if [[ "${mode_choice}" == "2" ]]; then
    target_host="$(prompt 'Target Host' '')"
    target_scheme="$(prompt 'Target Scheme (http/https)' 'https')"
    target_port="$(prompt 'Target Port (blank for default)' '')"
    host_header="$(prompt 'Upstream Host Header' "${target_host}")"
    sni=""
    if [[ "${target_scheme}" == "https" ]]; then
      sni="$(prompt 'Upstream SNI' "${target_host}")"
    fi
    cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"domain","domain":$(json_escape "${domain}"),"upstream_mode":"host","target_host":$(json_escape "${target_host}"),"target_scheme":$(json_escape "${target_scheme}"),"target_port":${target_port:-0},"upstream_host_header":$(json_escape "${host_header}"),"upstream_sni":$(json_escape "${sni}")}
EOF
    return
  fi

  target_ip="$(prompt 'Target IP' '')"
  target_port="$(prompt 'Target Port' '')"
  host_header="$(prompt 'Upstream Host Header' '$host')"
  cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"domain","domain":$(json_escape "${domain}"),"upstream_mode":"ip_port","target_ip":$(json_escape "${target_ip}"),"target_port":${target_port},"upstream_host_header":$(json_escape "${host_header}")}
EOF
}

build_port_update_payload() {
  local name="$1"
  local listen_ip listen_port target_host target_ip target_port target_scheme host_header sni mode_choice
  listen_port="$(prompt 'Listen Port' '')"
  listen_ip="$(prompt 'Listen IP (blank for all interfaces)' '')"
  printf 'Upstream Mode:\n1) ip_port\n2) host\n'
  mode_choice="$(prompt 'Choose upstream mode' '2')"

  if [[ "${mode_choice}" == "1" ]]; then
    target_ip="$(prompt 'Target IP' '')"
    target_port="$(prompt 'Target Port' '')"
    host_header="$(prompt 'Upstream Host Header' '$host')"
    cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"port","listen_ip":$(json_escape "${listen_ip}"),"listen_port":${listen_port},"upstream_mode":"ip_port","target_ip":$(json_escape "${target_ip}"),"target_port":${target_port},"upstream_host_header":$(json_escape "${host_header}")}
EOF
    return
  fi

  target_host="$(prompt 'Target Host' '')"
  target_scheme="$(prompt 'Target Scheme (http/https)' 'https')"
  target_port="$(prompt 'Target Port (blank for default)' '')"
  host_header="$(prompt 'Upstream Host Header' "${target_host}")"
  sni=""
  if [[ "${target_scheme}" == "https" ]]; then
    sni="$(prompt 'Upstream SNI' "${target_host}")"
  fi
  cat <<EOF
{"name":$(json_escape "${name}"),"frontend_mode":"port","listen_ip":$(json_escape "${listen_ip}"),"listen_port":${listen_port},"upstream_mode":"host","target_host":$(json_escape "${target_host}"),"target_scheme":$(json_escape "${target_scheme}"),"target_port":${target_port:-0},"upstream_host_header":$(json_escape "${host_header}"),"upstream_sni":$(json_escape "${sni}")}
EOF
}

delete_route() {
  local name
  name="$(prompt 'Route Name to delete' '')"
  read -r -p "Delete ${name}? [y/N]: " confirmed
  if [[ "${confirmed}" != "y" && "${confirmed}" != "Y" ]]; then
    echo "Cancelled."
    return
  fi

  run_request DELETE "/routes/${name}"
}

main_menu() {
  while true; do
    clear
    cat <<EOF
reproxy SSH Panel
API: ${API_BASE}

1) View status
2) List routes
3) Create domain route
4) Create port listener route
5) Update route
6) Delete route
7) Show web panel URL
0) Exit
EOF
    local choice
    choice="$(prompt 'Choose an option' '1')"

    case "${choice}" in
      1) show_status; pause ;;
      2) list_routes; pause ;;
      3) create_domain_route; pause ;;
      4) create_port_route; pause ;;
      5) update_route; pause ;;
      6) delete_route; pause ;;
      7)
        printf '\nWeb panel: %s/panel/\n' "${API_BASE}"
        pause
        ;;
      0) exit 0 ;;
      *)
        printf '\nInvalid choice.\n'
        pause
        ;;
    esac
  done
}

load_env
main_menu
