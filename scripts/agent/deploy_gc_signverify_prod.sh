#!/usr/bin/env bash
set -euo pipefail

PROJECT_PATH="{project_name}"
MODULE_NAME="$(basename "${PROJECT_PATH}")"
APP_DIR="/home/java/${PROJECT_PATH}"
TARGET_JAR_NAME="${MODULE_NAME}.jar"
IMAGE_VERSION="{image_version}"
DOWNLOAD_BASE_URL="https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate"
RUN_ENV="prod"
TIMESTAMP="$(date +%Y%m%d%H%M%S)"
BACKUP_JAR_NAME="${TARGET_JAR_NAME%.jar}-backup-${TIMESTAMP}.jar"
DOWNLOAD_OBJECT_PATH="${PROJECT_PATH}-${IMAGE_VERSION}.jar"
DOWNLOADED_JAR_NAME="${MODULE_NAME}-${IMAGE_VERSION}.jar"
DOWNLOAD_URL="${DOWNLOAD_BASE_URL}/${DOWNLOAD_OBJECT_PATH}"

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

resolve_jar_start_script() {
  local candidate=""
  for candidate in jar-start.sh jar-start; do
    if [ -f "${candidate}" ]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done

  echo "缺少 jar-start 启动脚本，无法重启应用: ${APP_DIR}/jar-start(.sh)" >&2
  return 1
}

if [ -z "${PROJECT_PATH}" ] || [ "${PROJECT_PATH}" = "{project_name}" ]; then
  echo "project_name 未配置，无法定位应用目录和目标包名" >&2
  exit 1
fi

if [ -z "${IMAGE_VERSION}" ] || [ "${IMAGE_VERSION}" = "{image_version}" ]; then
  echo "image_version 未配置，无法生成下载包名" >&2
  exit 1
fi

if [ ! -d "${APP_DIR}" ]; then
  echo "应用目录不存在: ${APP_DIR}" >&2
  exit 1
fi

echo "================= 参数信息 ================="
echo "PROJECT_PATH         = ${PROJECT_PATH}"
echo "MODULE_NAME          = ${MODULE_NAME}"
echo "APP_DIR              = ${APP_DIR}"
echo "TARGET_JAR_NAME      = ${TARGET_JAR_NAME}"
echo "IMAGE_VERSION        = ${IMAGE_VERSION}"
echo "DOWNLOAD_BASE_URL    = ${DOWNLOAD_BASE_URL}"
echo "DOWNLOAD_OBJECT_PATH = ${DOWNLOAD_OBJECT_PATH}"
echo "DOWNLOADED_JAR_NAME  = ${DOWNLOADED_JAR_NAME}"
echo "DOWNLOAD_URL         = ${DOWNLOAD_URL}"
echo "RUN_ENV              = ${RUN_ENV}"
echo "TIMESTAMP            = ${TIMESTAMP}"
echo "BACKUP_JAR_NAME      = ${BACKUP_JAR_NAME}"
echo "==========================================="

cd "${APP_DIR}"
echo "已切换到目录: ${APP_DIR}"

if [ -f "${TARGET_JAR_NAME}" ]; then
  mv "${TARGET_JAR_NAME}" "${BACKUP_JAR_NAME}"
  echo "已备份旧包: ${BACKUP_JAR_NAME}"
else
  echo "未找到旧包，跳过备份: ${TARGET_JAR_NAME}"
fi

echo "开始下载新包:"
echo "URL: ${DOWNLOAD_URL}"
wget -nv -O "${DOWNLOADED_JAR_NAME}" "${DOWNLOAD_URL}"

echo "下载完成: ${DOWNLOADED_JAR_NAME}"
ls -lh "${DOWNLOADED_JAR_NAME}"

mv "${DOWNLOADED_JAR_NAME}" "${TARGET_JAR_NAME}"
echo "已替换新包: ${TARGET_JAR_NAME}"

echo "开始重启应用"
JAVA_BIN="$(resolve_java_bin)"
export PATH="$(dirname "${JAVA_BIN}"):$PATH"
if [ -z "${JAVA_HOME:-}" ]; then
  export JAVA_HOME="$(cd "$(dirname "${JAVA_BIN}")/.." && pwd)"
fi

JAR_START_SCRIPT="$(resolve_jar_start_script)"

echo "使用 Java: ${JAVA_BIN}"
echo "执行命令: sh ${JAR_START_SCRIPT} ${TARGET_JAR_NAME} restart ${RUN_ENV}"
sh "${JAR_START_SCRIPT}" "${TARGET_JAR_NAME}" restart "${RUN_ENV}"

echo "重启完成"
