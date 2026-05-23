#!/usr/bin/env bash
set -euo pipefail

APP_DIR="/home/java/gateway"
TARGET_JAR_NAME="gateway.jar"
LEGACY_JAR_NAME="gatewa.jar"
DOWNLOAD_URL="{artifact_url}"
RUN_ENV="prod"
TIMESTAMP="$(date +%Y%m%d%H%M%S)"
VERSIONED_JAR_NAME="${TARGET_JAR_NAME%.jar}-${TIMESTAMP}.jar"
BACKUP_JAR_NAME="${TARGET_JAR_NAME%.jar}-backup-${TIMESTAMP}.jar"

load_shell_profiles() {
  local profile=""
  for profile in /etc/profile /etc/bashrc "${HOME:-}/.bash_profile" "${HOME:-}/.bashrc" "${HOME:-}/.profile"; do
    if [ -n "${profile}" ] && [ -f "${profile}" ]; then
      # shellcheck disable=SC1090
      . "${profile}"
    fi
  done
}

resolve_java_bin() {
  load_shell_profiles

  if command -v java >/dev/null 2>&1; then
    command -v java
    return 0
  fi

  if [ -n "${JAVA_HOME:-}" ] && [ -x "${JAVA_HOME}/bin/java" ]; then
    printf '%s\n' "${JAVA_HOME}/bin/java"
    return 0
  fi

  local candidate=""
  for candidate in /usr/bin/java /usr/local/bin/java /usr/java/latest/bin/java /opt/java/bin/java /opt/jdk/bin/java; do
    if [ -x "${candidate}" ]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done

  for candidate in /usr/lib/jvm/*/bin/java; do
    if [ -x "${candidate}" ]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done

  echo "未找到可用的 java 命令，请检查 JAVA_HOME 或 PATH 配置" >&2
  return 1
}

if [ -z "${DOWNLOAD_URL}" ] || [ "${DOWNLOAD_URL}" = "{artifact_url}" ]; then
  echo "artifact_url 未配置，无法下载新包" >&2
  exit 1
fi

if [ ! -d "${APP_DIR}" ]; then
  echo "应用目录不存在: ${APP_DIR}" >&2
  exit 1
fi

cd "${APP_DIR}"

if [ -f "${TARGET_JAR_NAME}" ]; then
  if [ -L "${TARGET_JAR_NAME}" ]; then
    rm -f "${TARGET_JAR_NAME}"
    echo "已移除旧版本软链: ${TARGET_JAR_NAME}"
  else
    mv "${TARGET_JAR_NAME}" "${BACKUP_JAR_NAME}"
    echo "已备份旧包: ${BACKUP_JAR_NAME}"
  fi
elif [ -f "${LEGACY_JAR_NAME}" ]; then
  mv "${LEGACY_JAR_NAME}" "${BACKUP_JAR_NAME}"
  echo "已备份旧包(兼容旧文件名): ${BACKUP_JAR_NAME}"
else
  echo "未找到历史 jar，跳过备份"
fi

echo "开始下载新包: ${DOWNLOAD_URL}"
wget -O "${VERSIONED_JAR_NAME}" "${DOWNLOAD_URL}"

echo "下载完成，文件信息:"
ls -lh "${VERSIONED_JAR_NAME}"

ln -sfn "${VERSIONED_JAR_NAME}" "${TARGET_JAR_NAME}"
echo "已更新当前运行软链: ${TARGET_JAR_NAME} -> ${VERSIONED_JAR_NAME}"

echo "开始重启应用"
if [ ! -f "jar-start" ]; then
  echo "缺少 jar-start 脚本，无法重启应用: ${APP_DIR}/jar-start" >&2
  exit 1
fi

JAVA_BIN="$(resolve_java_bin)"
export PATH="$(dirname "${JAVA_BIN}"):$PATH"
if [ -z "${JAVA_HOME:-}" ]; then
  export JAVA_HOME="$(cd "$(dirname "${JAVA_BIN}")/.." && pwd)"
fi

echo "使用 Java: ${JAVA_BIN}"
sh jar-start "${TARGET_JAR_NAME}" restart "${RUN_ENV}"

echo "重启命令已执行完成"
