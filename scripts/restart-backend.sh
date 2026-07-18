#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="local"
CONFIG="configs/config.local.json"
PID_FILE="runtime/backend.pid"
LOG_FILE="logs/backend.log"
BINARY_FILE="runtime/gos-server"
BACKEND_SESSION="gos-backend"
FRONTEND_DIR="frontend"
FRONTEND_PORT=5174
FRONTEND_PID_FILE="runtime/frontend.pid"
FRONTEND_LOG_FILE="logs/frontend.log"
FRONTEND_API_BASE_URL="http://127.0.0.1:8081"
FRONTEND_SESSION="gos-frontend"
HEALTH_URL="http://127.0.0.1:8081/healthz"
HEALTH_TIMEOUT=60
STOP_TIMEOUT=10

usage() {
  cat <<'USAGE'
Usage: scripts/restart-backend.sh [options]

Options:
  --mode local                 Compatibility option; only non-container local startup is supported. Default: local
  --config PATH                Config file for local mode. Default: configs/config.local.json
  --health-url URL             Health endpoint to wait for. Default: http://127.0.0.1:8081/healthz
  --pid-file PATH              PID file for local mode. Default: runtime/backend.pid
  --log-file PATH              Log file for local mode. Default: logs/backend.log
  --timeout SECONDS            Health check timeout. Default: 60
  --kill-port                  Compatibility option; local mode already stops an existing healthy backend listener
  -h, --help                   Show this help

Examples:
  scripts/restart-backend.sh
USAGE
}

log() {
  printf '[restart-backend] %s\n' "$*"
}

fail() {
  printf '[restart-backend] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      [[ $# -ge 2 ]] || fail "--mode requires a value"
      MODE="$2"
      shift 2
      ;;
    --config)
      [[ $# -ge 2 ]] || fail "--config requires a value"
      CONFIG="$2"
      shift 2
      ;;
    --health-url)
      [[ $# -ge 2 ]] || fail "--health-url requires a value"
      HEALTH_URL="$2"
      shift 2
      ;;
    --pid-file)
      [[ $# -ge 2 ]] || fail "--pid-file requires a value"
      PID_FILE="$2"
      shift 2
      ;;
    --log-file)
      [[ $# -ge 2 ]] || fail "--log-file requires a value"
      LOG_FILE="$2"
      shift 2
      ;;
    --timeout)
      [[ $# -ge 2 ]] || fail "--timeout requires a value"
      HEALTH_TIMEOUT="$2"
      shift 2
      ;;
    --kill-port)
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1"
      ;;
  esac
done

case "${MODE}" in
  local)
    ;;
  *)
    fail "unsupported mode: ${MODE}; only local non-container startup is supported"
    ;;
esac

cd "${ROOT_DIR}"

wait_for_health() {
  require_command curl

  local consecutive_successes=0
  local deadline
  deadline=$((SECONDS + HEALTH_TIMEOUT))

  while (( consecutive_successes < 3 )); do
    if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      consecutive_successes=$((consecutive_successes + 1))
    else
      consecutive_successes=0
    fi

    if (( SECONDS >= deadline )); then
      fail "backend did not become healthy within ${HEALTH_TIMEOUT}s: ${HEALTH_URL}"
    fi

    if (( consecutive_successes < 3 )); then
      sleep 1
    fi
  done

  log "health check passed: ${HEALTH_URL}"
}

read_pid_file() {
  if [[ -f "${PID_FILE}" ]]; then
    tr -dc '0-9' < "${PID_FILE}"
  fi
}

pid_is_running() {
  local pid="$1"
  [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1
}

stop_pid() {
  local pid="$1"
  local deadline

  if ! pid_is_running "${pid}"; then
    return 0
  fi

  log "stopping local backend process: ${pid}"
  if command -v pkill >/dev/null 2>&1; then
    pkill -TERM -P "${pid}" >/dev/null 2>&1 || true
  fi
  kill -TERM "${pid}" >/dev/null 2>&1 || true

  deadline=$((SECONDS + STOP_TIMEOUT))
  while pid_is_running "${pid}"; do
    if (( SECONDS >= deadline )); then
      log "process ${pid} did not stop in ${STOP_TIMEOUT}s; sending SIGKILL"
      if command -v pkill >/dev/null 2>&1; then
        pkill -KILL -P "${pid}" >/dev/null 2>&1 || true
      fi
      kill -KILL "${pid}" >/dev/null 2>&1 || true
      break
    fi
    sleep 1
  done
}

health_port() {
  local host_port="${HEALTH_URL#*://}"
  host_port="${host_port%%/*}"
  host_port="${host_port##*:}"
  if [[ "${host_port}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${host_port}"
  fi
}

listener_pids() {
  local port="$1"

  [[ -n "${port}" ]] || return 0
  lsof -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true
}

port_is_listening() {
  local port="$1"
  [[ -n "$(listener_pids "${port}")" ]]
}

stable_listener_pid() {
  local port="$1"
  local previous=""
  local current=""
  local deadline

  [[ -n "${port}" ]] || return 0
  command -v lsof >/dev/null 2>&1 || return 0

  deadline=$((SECONDS + 10))
  while (( SECONDS < deadline )); do
    current="$(listener_pids "${port}" | head -n 1)"
    if [[ -n "${current}" && "${current}" == "${previous}" ]]; then
      printf '%s\n' "${current}"
      return 0
    fi
    previous="${current}"
    sleep 1
  done

  if [[ -n "${previous}" ]]; then
    printf '%s\n' "${previous}"
  fi
}

stop_port_listener() {
  local port="$1"
  local pids

  [[ -n "${port}" ]] || return 0
  command -v lsof >/dev/null 2>&1 || fail "stopping an existing backend listener requires lsof"

  pids="$(listener_pids "${port}")"
  [[ -n "${pids}" ]] || return 0

  log "stopping process listening on port ${port}: ${pids//$'\n'/ }"
  while IFS= read -r pid; do
    [[ -n "${pid}" ]] || continue
    stop_pid "${pid}"
  done <<< "${pids}"
}

stop_screen_session() {
  local session="$1"

  if command -v screen >/dev/null 2>&1; then
    screen -S "${session}" -X quit >/dev/null 2>&1 || true
  fi
}

refresh_pid_file_from_listener() {
  local fallback_pid="${1:-}"
  local port
  local listener_pid=""

  port="$(health_port)"
  if [[ -n "${port}" ]] && command -v lsof >/dev/null 2>&1; then
    listener_pid="$(stable_listener_pid "${port}")"
  fi

  if [[ -n "${listener_pid}" ]]; then
    printf '%s\n' "${listener_pid}" > "${PID_FILE}"
    log "listener pid: ${listener_pid}"
  elif [[ -n "${fallback_pid}" ]]; then
    printf '%s\n' "${fallback_pid}" > "${PID_FILE}"
    log "pid: ${fallback_pid}"
  fi
}

refresh_frontend_pid_file_from_listener() {
  local listener_pid=""

  if command -v lsof >/dev/null 2>&1; then
    listener_pid="$(stable_listener_pid "${FRONTEND_PORT}")"
  fi

  if [[ -n "${listener_pid}" ]]; then
    printf '%s\n' "${listener_pid}" > "${FRONTEND_PID_FILE}"
    log "frontend listener pid: ${listener_pid}"
  fi
}

restart_local() {
  require_command go
  require_command curl

  [[ -f "${CONFIG}" ]] || fail "config file not found: ${CONFIG}"

  mkdir -p "$(dirname "${PID_FILE}")" "$(dirname "${LOG_FILE}")" "$(dirname "${BINARY_FILE}")"

  local old_pid
  old_pid="$(read_pid_file || true)"
  if [[ -n "${old_pid}" ]]; then
    if pid_is_running "${old_pid}"; then
      stop_pid "${old_pid}"
    else
      log "stale pid file: ${PID_FILE} (${old_pid})"
    fi
    rm -f "${PID_FILE}"
  fi

  if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
    log "backend is already healthy; stopping listener on port $(health_port)"
  else
    log "checking for existing listener on port $(health_port)"
  fi
  stop_port_listener "$(health_port)"

  log "building backend binary: ${BINARY_FILE}"
  go build -o "${BINARY_FILE}" ./cmd/server

  log "starting local backend with config: ${CONFIG}"
  : > "${LOG_FILE}"
  local backend_pid=""
  stop_screen_session "${BACKEND_SESSION}"
  if command -v screen >/dev/null 2>&1; then
    screen -dmS "${BACKEND_SESSION}" sh -c 'cd "$1" && exec "$2" -config "$3" >>"$4" 2>&1' sh "${ROOT_DIR}" "${ROOT_DIR}/${BINARY_FILE}" "${CONFIG}" "${ROOT_DIR}/${LOG_FILE}"
    log "backend session: ${BACKEND_SESSION}"
  else
    nohup "${BINARY_FILE}" -config "${CONFIG}" >>"${LOG_FILE}" 2>&1 &
    backend_pid=$!
    printf '%s\n' "${backend_pid}" > "${PID_FILE}"
    disown "${backend_pid}" >/dev/null 2>&1 || true
    log "launch pid: ${backend_pid}"
  fi

  log "log: ${LOG_FILE}"

  wait_for_health "${backend_pid}"
  refresh_pid_file_from_listener "${backend_pid}"
}

wait_for_frontend() {
  require_command curl

  local frontend_url="http://127.0.0.1:${FRONTEND_PORT}/"
  local consecutive_successes=0
  local deadline
  deadline=$((SECONDS + HEALTH_TIMEOUT))

  while (( consecutive_successes < 2 )); do
    if curl -fsS "${frontend_url}" >/dev/null 2>&1; then
      consecutive_successes=$((consecutive_successes + 1))
    else
      consecutive_successes=0
    fi

    if (( SECONDS >= deadline )); then
      fail "frontend did not become ready within ${HEALTH_TIMEOUT}s: ${frontend_url}"
    fi

    if (( consecutive_successes < 2 )); then
      sleep 1
    fi
  done

  log "frontend ready: ${frontend_url}"
}

ensure_frontend() {
  command -v lsof >/dev/null 2>&1 || fail "checking frontend port requires lsof"

  if port_is_listening "${FRONTEND_PORT}"; then
    log "frontend is already listening on port ${FRONTEND_PORT}; skipping start"
    return 0
  fi

  require_command npm
  require_command curl

  [[ -f "${FRONTEND_DIR}/package.json" ]] || fail "frontend package.json not found: ${FRONTEND_DIR}/package.json"

  mkdir -p "$(dirname "${FRONTEND_PID_FILE}")" "$(dirname "${FRONTEND_LOG_FILE}")"

  if [[ ! -d "${FRONTEND_DIR}/node_modules" ]]; then
    log "installing frontend dependencies"
    (cd "${FRONTEND_DIR}" && npm install)
  fi

  log "starting frontend dev server on port ${FRONTEND_PORT}"
  : > "${FRONTEND_LOG_FILE}"
  stop_screen_session "${FRONTEND_SESSION}"
  if command -v screen >/dev/null 2>&1; then
    screen -dmS "${FRONTEND_SESSION}" sh -c 'cd "$1" && exec env VITE_API_BASE_URL="$2" npm run dev >>"$3" 2>&1' sh "${ROOT_DIR}/${FRONTEND_DIR}" "${FRONTEND_API_BASE_URL}" "${ROOT_DIR}/${FRONTEND_LOG_FILE}"
    log "frontend session: ${FRONTEND_SESSION}"
  else
    (
      cd "${FRONTEND_DIR}"
      nohup env VITE_API_BASE_URL="${FRONTEND_API_BASE_URL}" npm run dev >>"${ROOT_DIR}/${FRONTEND_LOG_FILE}" 2>&1 &
      local frontend_pid=$!
      printf '%s\n' "${frontend_pid}" > "${ROOT_DIR}/${FRONTEND_PID_FILE}"
      disown "${frontend_pid}" >/dev/null 2>&1 || true
    )
    log "frontend pid: $(cat "${FRONTEND_PID_FILE}")"
  fi
  log "frontend log: ${FRONTEND_LOG_FILE}"

  wait_for_frontend
  refresh_frontend_pid_file_from_listener
}

restart_local
ensure_frontend

log "backend and frontend startup completed (${MODE})"
