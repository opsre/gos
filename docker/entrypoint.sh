#!/bin/sh
set -eu

mkdir -p /app/configs /app/data /gitops

if [ -n "${GOS_GITOPS_PATH_MAPS:-}" ]; then
  OLD_IFS="$IFS"
  IFS=';'
  for pair in $GOS_GITOPS_PATH_MAPS; do
    pair=$(printf '%s' "$pair" | sed 's/^ *//;s/ *$//')
    [ -z "$pair" ] && continue
    source_path=${pair%%=*}
    target_path=${pair#*=}
    source_path=$(printf '%s' "$source_path" | sed 's/^ *//;s/ *$//')
    target_path=$(printf '%s' "$target_path" | sed 's/^ *//;s/ *$//')
    if [ -z "$source_path" ] || [ -z "$target_path" ]; then
      echo "[gos-entrypoint] skip invalid GOS_GITOPS_PATH_MAPS entry: $pair" >&2
      continue
    fi
    mkdir -p "$(dirname "$source_path")"
    rm -rf "$source_path"
    ln -s "$target_path" "$source_path"
    echo "[gos-entrypoint] mapped gitops path: $source_path -> $target_path"
  done
  IFS="$OLD_IFS"
fi

python3 <<'PY'
import json
import os
from pathlib import Path

def env(name, default):
    return os.environ.get(name, default)

def env_bool(name, default):
    return str(os.environ.get(name, str(default))).strip().lower() in ("1", "true", "yes", "on")

def env_int(name, default):
    raw = str(os.environ.get(name, default)).strip()
    try:
        return int(raw)
    except ValueError:
        return int(default)

def env_list(name, default):
    raw = str(os.environ.get(name, default)).strip()
    return [item.strip() for item in raw.split(',') if item.strip()]

cfg = {
    "environment": env("GOS_ENVIRONMENT", "docker"),
    "server": {
        "addr": env("GOS_SERVER_ADDR", ":8081"),
        "read_header_timeout_sec": env_int("GOS_SERVER_READ_HEADER_TIMEOUT_SEC", 5),
        "read_timeout_sec": env_int("GOS_SERVER_READ_TIMEOUT_SEC", 120),
        "write_timeout_sec": env_int("GOS_SERVER_WRITE_TIMEOUT_SEC", 120),
        "idle_timeout_sec": env_int("GOS_SERVER_IDLE_TIMEOUT_SEC", 60),
        "shutdown_timeout_sec": env_int("GOS_SERVER_SHUTDOWN_TIMEOUT_SEC", 10),
    },
    "database": {
        "driver": env("GOS_DB_DRIVER", "mysql"),
        "mysql_dsn": env("GOS_MYSQL_DSN", ""),
        "sqlite_path": env("GOS_SQLITE_PATH", "/app/data/demo.db"),
        "max_open_conns": env_int("GOS_DB_MAX_OPEN_CONNS", 10),
        "max_idle_conns": env_int("GOS_DB_MAX_IDLE_CONNS", 5),
        "conn_max_lifetime_sec": env_int("GOS_DB_CONN_MAX_LIFETIME_SEC", 1800),
        "conn_max_idle_time_sec": env_int("GOS_DB_CONN_MAX_IDLE_TIME_SEC", 600),
        "ping_timeout_sec": env_int("GOS_DB_PING_TIMEOUT_SEC", 5),
        "startup_max_retries": env_int("GOS_DB_STARTUP_MAX_RETRIES", 20),
        "startup_retry_interval_sec": env_int("GOS_DB_STARTUP_RETRY_INTERVAL_SEC", 3),
    },
    "jenkins": {
        "enabled": env_bool("GOS_JENKINS_ENABLED", False),
        "base_url": env("GOS_JENKINS_BASE_URL", "http://127.0.0.1:8080/"),
        "username": env("GOS_JENKINS_USERNAME", "admin"),
        "api_token": env("GOS_JENKINS_API_TOKEN", ""),
        "timeout_sec": env_int("GOS_JENKINS_TIMEOUT_SEC", 60),
        "startup_check_enabled": env_bool("GOS_JENKINS_STARTUP_CHECK_ENABLED", False),
        "startup_max_retries": env_int("GOS_JENKINS_STARTUP_MAX_RETRIES", 5),
        "startup_retry_interval_sec": env_int("GOS_JENKINS_STARTUP_RETRY_INTERVAL_SEC", 2),
        "auto_sync_enabled": env_bool("GOS_JENKINS_AUTO_SYNC_ENABLED", False),
        "auto_sync_interval_sec": env_int("GOS_JENKINS_AUTO_SYNC_INTERVAL_SEC", 300),
        "release_track_enabled": env_bool("GOS_JENKINS_RELEASE_TRACK_ENABLED", False),
        "release_track_interval_sec": env_int("GOS_JENKINS_RELEASE_TRACK_INTERVAL_SEC", 10),
    },
    "release": {
        "env_options": env_list("GOS_RELEASE_ENV_OPTIONS", "dev,test,prod"),
        "concurrency": {
            "enabled": env_bool("GOS_RELEASE_CONCURRENCY_ENABLED", True),
            "lock_scope": env("GOS_RELEASE_LOCK_SCOPE", "application_env"),
            "conflict_strategy": env("GOS_RELEASE_CONFLICT_STRATEGY", "reject"),
            "lock_timeout_sec": env_int("GOS_RELEASE_LOCK_TIMEOUT_SEC", 1800),
        },
    },
    "auth": {
        "session_ttl_hours": env_int("GOS_AUTH_SESSION_TTL_HOURS", 24),
        "admin_username": env("GOS_AUTH_ADMIN_USERNAME", "admin"),
        "admin_display_name": env("GOS_AUTH_ADMIN_DISPLAY_NAME", "Administrator"),
        "admin_password": env("GOS_AUTH_ADMIN_PASSWORD", ""),
    },
    "security": {
        "encryption_key": env("GOS_SECURITY_ENCRYPTION_KEY", ""),
    },
}

missing = []
if cfg["database"]["driver"] == "mysql" and not cfg["database"]["mysql_dsn"].strip():
    missing.append("GOS_MYSQL_DSN")
if not cfg["auth"]["admin_password"].strip():
    missing.append("GOS_AUTH_ADMIN_PASSWORD")
if not cfg["security"]["encryption_key"].strip():
    missing.append("GOS_SECURITY_ENCRYPTION_KEY")
if missing:
    raise SystemExit("[gos-entrypoint] missing required environment variables: " + ", ".join(missing))

out = Path('/app/configs/config.runtime.json')
out.write_text(json.dumps(cfg, ensure_ascii=False, indent=2))
print(f'generated config: {out}')
print(f'database driver: {cfg["database"]["driver"]}')
print(f'jenkins enabled: {cfg["jenkins"]["enabled"]}')
print(f'admin username: {cfg["auth"]["admin_username"]}')
PY

exec "$@"
