import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, statSync } from 'node:fs'

const scriptPath = new URL('../scripts/restart-backend.sh', import.meta.url)
const restartScript = readFileSync(scriptPath, 'utf8')

test('backend restart script is executable and defaults to local Go server restart', () => {
  const mode = statSync(scriptPath).mode

  assert.equal((mode & 0o111) !== 0, true, 'restart script should be executable')
  assert.match(restartScript, /MODE="local"/, 'local mode should be the default')
  assert.match(restartScript, /BINARY_FILE="runtime\/gos-server"/, 'local mode should use a stable runtime binary path')
  assert.match(restartScript, /go build -o "\$\{BINARY_FILE\}" \.\/cmd\/server/, 'local mode should build the backend binary before starting')
  assert.match(restartScript, /nohup "\$\{BINARY_FILE\}" -config "\$\{CONFIG\}"/, 'local mode should run the built backend with the selected config')
  assert.match(restartScript, /configs\/config\.local\.json/, 'local mode should use the local config by default')
  assert.match(restartScript, /runtime\/backend\.pid/, 'local mode should persist the backend pid')
  assert.match(restartScript, /logs\/backend\.log/, 'local mode should write backend logs')
})

test('local restart clears stale pid files and the current port listener before starting', () => {
  assert.match(restartScript, /stale pid file/, 'script should identify stale pid files')
  assert.match(restartScript, /stop_port_listener "\$\(health_port\)"/, 'local mode should stop the current backend listener on the health port')
  assert.doesNotMatch(restartScript, /rerun with --kill-port/, 'local mode should not require an extra flag for the common restart case')
})

test('local restart confirms stable health and records the listener pid best effort', () => {
  assert.match(restartScript, /listener_pids\(\)/, 'script should expose a reusable listener pid resolver')
  assert.match(restartScript, /stable_listener_pid\(\)/, 'script should wait until the listener pid is stable')
  assert.match(restartScript, /refresh_pid_file_from_listener\(\)/, 'script should refresh the pid file from the actual listening process')
  assert.match(restartScript, /consecutive_successes/, 'script should require consecutive health checks before reporting success')
  assert.match(restartScript, /refresh_pid_file_from_listener "\$\{backend_pid\}"/, 'local mode should update the pid file after the backend becomes healthy')
})

test('backend restart script supports docker compose restarts and health checks', () => {
  assert.match(restartScript, /docker compose -f "\$\{COMPOSE_FILE\}" up -d --build backend frontend/, 'docker mode should start backend and frontend services through compose')
  assert.match(restartScript, /docker-compose -f "\$\{COMPOSE_FILE\}" up -d --build backend frontend/, 'docker mode should support legacy docker-compose')
  assert.match(restartScript, /docker-compose\.prod\.yml/, 'prod mode should select the production compose file')
  assert.match(restartScript, /curl -fsS "\$\{HEALTH_URL\}"/, 'script should verify the backend health endpoint')
  assert.match(restartScript, /--mode local\|docker\|prod/, 'help text should document supported modes')
})

test('local mode starts frontend only when it is not already listening', () => {
  assert.match(restartScript, /FRONTEND_PORT=5174/, 'frontend mode should target the Vite dev server port')
  assert.match(restartScript, /FRONTEND_LOG_FILE="logs\/frontend\.log"/, 'frontend startup should write a dedicated log file')
  assert.match(restartScript, /FRONTEND_PID_FILE="runtime\/frontend\.pid"/, 'frontend startup should persist its pid')
  assert.match(restartScript, /frontend is already listening on port \$\{FRONTEND_PORT\}; skipping start/, 'frontend should be skipped when the port is already listening')
  assert.doesNotMatch(restartScript, /frontend port \$\{FRONTEND_PORT\} is occupied by another process/, 'frontend should not fail just because another process already listens on the frontend port')
  assert.match(restartScript, /VITE_API_BASE_URL="\$\{FRONTEND_API_BASE_URL\}" npm run dev/, 'frontend should start with the backend API base URL')
  assert.match(restartScript, /ensure_frontend/, 'local mode should call the frontend ensure step')
})
