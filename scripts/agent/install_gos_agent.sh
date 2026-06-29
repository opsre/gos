#!/bin/bash
#
# GOS Agent 一键安装脚本（systemd 终极版）
#
# 用法：
#   bash install_gos_agent.sh \
#     --server-url http://127.0.0.1:8081 \
#     --token agboot-xxxxxxxx \
#     --work-dir /etc/gos-agent \
#     --name my-agent \
#     --tags production,web \
#     --version v1.0.0
#
# 远程执行：
#   wget -qO- https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/install_gos_agent.sh | bash -s -- \
#     --server-url http://your-gos-server:5174 \
#     --token agboot-xxxxxxxx \
#     --work-dir /etc/gos-agent \
#     --name prod-agent \
#     --tags production,web
#

set -euo pipefail

DEFAULT_WORK_DIR="/etc/gos-agent"
DEFAULT_AGENT_NAME="Auto Registered Agent"
DEFAULT_VERSION="v1.0.0"
DEFAULT_TAGS="linux"
DEFAULT_AGENT_BINARY_URL="https://gc-oa.oss-cn-shanghai.aliyuncs.com/tempUpdate/gos-agent-linux-amd64"
DEFAULT_SERVICE_NAME="gos-agent"

CONFIG_FILE="config.yaml"
AGENT_BINARY="gos-agent"

SERVER_URL=""
TOKEN=""
WORK_DIR="$DEFAULT_WORK_DIR"
AGENT_NAME="$DEFAULT_AGENT_NAME"
TAGS="$DEFAULT_TAGS"
VERSION="$DEFAULT_VERSION"
AGENT_BINARY_URL="$DEFAULT_AGENT_BINARY_URL"
SERVICE_NAME="$DEFAULT_SERVICE_NAME"

show_help() {
    cat << EOF
GOS Agent 一键安装脚本（systemd 终极版）

用法:
  ./install_gos_agent.sh --server-url <URL> --token <TOKEN> [选项]

必填参数:
  --server-url         GOS 平台服务地址
  --token              Agent 注册 Token

可选参数:
  --work-dir           工作目录，默认: $DEFAULT_WORK_DIR
  --name               Agent 名称，默认: $DEFAULT_AGENT_NAME
  --tags               标签，逗号分隔，默认: $DEFAULT_TAGS
  --version            Agent 版本，默认: $DEFAULT_VERSION
  --agent-binary-url   Agent 二进制下载地址
  --service-name       systemd 服务名，默认: $DEFAULT_SERVICE_NAME
  -h, --help           显示帮助

示例:
  ./install_gos_agent.sh \\
    --server-url http://your-gos-server:5174 \\
    --token agboot-xxxxxxxx \\
    --work-dir /etc/gos-agent \\
    --name prod-shicheng-10-5-5-200-gateway \\
    --tags production,web \\
    --version v1.0.0

EOF
}

log() {
    echo "[INFO] $*"
}

warn() {
    echo "[WARN] $*"
}

err() {
    echo "[ERROR] $*" >&2
    exit 1
}

require_root() {
    if [[ "${EUID}" -ne 0 ]]; then
        err "请使用 root 用户执行，或在命令前加 sudo"
    fi
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

download_file() {
    local url="$1"
    local output="$2"

    if command_exists curl; then
        curl -fsSL "$url" -o "$output"
    elif command_exists wget; then
        wget -qO "$output" "$url"
    else
        err "系统未安装 curl 或 wget，无法下载文件"
    fi
}

trim() {
    local var="$1"
    # shellcheck disable=SC2001
    echo "$(echo "$var" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --server-url)
            SERVER_URL="${2:-}"
            shift 2
            ;;
        --token)
            TOKEN="${2:-}"
            shift 2
            ;;
        --work-dir)
            WORK_DIR="${2:-}"
            shift 2
            ;;
        --name)
            AGENT_NAME="${2:-}"
            shift 2
            ;;
        --tags)
            TAGS="${2:-}"
            shift 2
            ;;
        --version)
            VERSION="${2:-}"
            shift 2
            ;;
        --agent-binary-url)
            AGENT_BINARY_URL="${2:-}"
            shift 2
            ;;
        --service-name)
            SERVICE_NAME="${2:-}"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            err "未知参数: $1"
            ;;
    esac
done

[[ -n "$SERVER_URL" ]] || err "--server-url 是必填参数"
[[ -n "$TOKEN" ]] || err "--token 是必填参数"
[[ -n "$WORK_DIR" ]] || err "--work-dir 不能为空"
[[ -n "$SERVICE_NAME" ]] || err "--service-name 不能为空"

require_root

AGENT_NAME="$(trim "$AGENT_NAME")"
SERVICE_NAME="$(trim "$SERVICE_NAME")"
WORK_DIR="$(trim "$WORK_DIR")"

CONFIG_PATH="$WORK_DIR/$CONFIG_FILE"
AGENT_BINARY_PATH="$WORK_DIR/$AGENT_BINARY"
TMP_BINARY_PATH="$WORK_DIR/${AGENT_BINARY}.tmp"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

log "=========================================="
log "开始安装 GOS Agent（systemd 终极版）"
log "=========================================="

log "[1/8] 创建目录"
mkdir -p "$WORK_DIR"
mkdir -p "$WORK_DIR/work"

log "[2/8] 下载 Agent 二进制"
download_file "$AGENT_BINARY_URL" "$TMP_BINARY_PATH"
mv "$TMP_BINARY_PATH" "$AGENT_BINARY_PATH"
chmod +x "$AGENT_BINARY_PATH"

log "[3/8] 生成配置文件: $CONFIG_PATH"
cat > "$CONFIG_PATH" << EOF
server:
    base_url: $SERVER_URL
agent:
    registration_token: $TOKEN
    name: $AGENT_NAME
    work_dir: $WORK_DIR
    heartbeat_interval: 15s
    poll_interval: 5s
    version: $VERSION
    tags:
EOF

IFS=',' read -ra TAG_ARRAY <<< "$TAGS"
HAS_TAG=0
for tag in "${TAG_ARRAY[@]}"; do
    tag="$(trim "$tag")"
    if [[ -n "$tag" ]]; then
        echo "        - $tag" >> "$CONFIG_PATH"
        HAS_TAG=1
    fi
done

if [[ "$HAS_TAG" -eq 0 ]]; then
    echo "        - linux" >> "$CONFIG_PATH"
fi

log "[4/8] 生成 systemd 服务文件: $SERVICE_FILE"
cat > "$SERVICE_FILE" << EOF
[Unit]
Description=GOS Agent Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$WORK_DIR
ExecStart=$AGENT_BINARY_PATH --config $CONFIG_PATH
Restart=always
RestartSec=5
StartLimitIntervalSec=0

# 安全退出时间
TimeoutStopSec=15

# 日志走 journald
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

log "[5/8] 重新加载 systemd"
systemctl daemon-reload

log "[6/8] 设置开机自启"
systemctl enable "$SERVICE_NAME"

log "[7/8] 重启服务"
systemctl restart "$SERVICE_NAME"

log "[8/8] 检查服务状态"
if systemctl is-active --quiet "$SERVICE_NAME"; then
    log "服务启动成功: $SERVICE_NAME"
else
    warn "服务未处于 active 状态，请执行以下命令排查："
    echo "  systemctl status $SERVICE_NAME -l --no-pager"
    echo "  journalctl -u $SERVICE_NAME -n 200 --no-pager"
    exit 1
fi

echo
echo "=========================================="
echo "安装完成"
echo "=========================================="
echo
echo "服务名:"
echo "  $SERVICE_NAME"
echo
echo "配置文件:"
echo "  $CONFIG_PATH"
echo
echo "二进制文件:"
echo "  $AGENT_BINARY_PATH"
echo
echo "常用命令:"
echo "  systemctl status $SERVICE_NAME -l --no-pager"
echo "  systemctl restart $SERVICE_NAME"
echo "  systemctl stop $SERVICE_NAME"
echo "  systemctl disable $SERVICE_NAME"
echo "  journalctl -u $SERVICE_NAME -f"
echo
echo "配置内容预览:"
echo "------------------------------------------"
cat "$CONFIG_PATH"
echo "------------------------------------------"
echo
echo "当前服务状态:"
systemctl --no-pager --full status "$SERVICE_NAME" || true
