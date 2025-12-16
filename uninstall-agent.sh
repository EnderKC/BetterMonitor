#!/bin/bash

# Better Monitor Agent 一键卸载脚本（Linux/macOS）
# 使用方法:
#   curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/refs/heads/main/uninstall-agent.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/refs/heads/main/uninstall-agent.sh | bash -s -- --yes

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

INSTALL_ROOT="/opt/better-monitor"
BIN_DIR="${INSTALL_ROOT}/bin"
BINARY_NAME="better-monitor-agent"
SERVICE_NAME="better-monitor-agent"
CONFIG_DIR="/etc/better-monitor"
CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
LOG_DIR="/var/log/better-monitor"
LOG_FILE="${LOG_DIR}/agent.log"

LAUNCHD_LABEL="com.better-monitor.agent"
LAUNCHD_PLIST="/Library/LaunchDaemons/${LAUNCHD_LABEL}.plist"

SYSTEMD_UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
OPENRC_INIT_FILE="/etc/init.d/${SERVICE_NAME}"
PID_FILE="/run/${SERVICE_NAME}.pid"

SUDO_CMD=""
ASSUME_YES="false"
KEEP_LOGS="false"

usage() {
    cat <<'EOF'
用法: uninstall-agent.sh [选项]

可选参数:
  -y, --yes          不提示确认，直接卸载
  --keep-logs        保留日志文件 (/var/log/better-monitor/agent.log)
  -h, --help         显示帮助

默认会删除：
  - 二进制: /opt/better-monitor/bin/better-monitor-agent
  - 配置: /etc/better-monitor/agent.yaml
  - 日志: /var/log/better-monitor/agent.log
  - 服务定义: systemd/OpenRC/launchd（若存在）
EOF
}

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -y|--yes)
                ASSUME_YES="true"
                shift
                ;;
            --keep-logs)
                KEEP_LOGS="true"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "未知参数: $1"
                ;;
        esac
    done
}

prepare_env() {
    if [[ "$EUID" -ne 0 ]]; then
        if ! command -v sudo >/dev/null 2>&1; then
            error "脚本需要 root 权限或 sudo，请以 root 运行或安装 sudo"
        fi
        SUDO_CMD="sudo"
    fi
}

confirm_uninstall() {
    if [[ "$ASSUME_YES" == "true" ]]; then
        return
    fi

    warn "即将卸载 Better Monitor Agent，这将删除本机 Agent 相关文件："
    warn "  - 二进制: ${BIN_DIR}/${BINARY_NAME}"
    warn "  - 配置: ${CONFIG_FILE}"
    if [[ "$KEEP_LOGS" == "true" ]]; then
        warn "  - 日志: (保留) ${LOG_FILE}"
    else
        warn "  - 日志: ${LOG_FILE}"
    fi
    warn "  - 服务定义: systemd/OpenRC/launchd（若存在）"

    local reply=""
    if [[ -r /dev/tty ]]; then
        read -r -p "确认卸载? [y/N] " reply </dev/tty || true
    else
        error "当前环境无法交互确认，请使用 --yes 继续"
    fi

    if [[ ! "$reply" =~ ^[Yy]$ ]]; then
        info "已取消卸载"
        exit 0
    fi
}

stop_and_remove_systemd() {
    # 仅在 systemd 运行时才尝试 stop/disable；卸载文件本身不依赖 systemd。
    if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
        $SUDO_CMD systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
        $SUDO_CMD systemctl disable "${SERVICE_NAME}" >/dev/null 2>&1 || true
        $SUDO_CMD systemctl reset-failed "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi

    $SUDO_CMD rm -f "$SYSTEMD_UNIT_FILE"

    if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
        $SUDO_CMD systemctl daemon-reload >/dev/null 2>&1 || true
    fi
}

stop_and_remove_openrc() {
    if command -v rc-service >/dev/null 2>&1; then
        $SUDO_CMD rc-service "${SERVICE_NAME}" stop >/dev/null 2>&1 || true
    fi
    if command -v rc-update >/dev/null 2>&1; then
        $SUDO_CMD rc-update del "${SERVICE_NAME}" default >/dev/null 2>&1 || true
        $SUDO_CMD rc-update del "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi
    $SUDO_CMD rm -f "$OPENRC_INIT_FILE"
}

stop_and_remove_launchd() {
    if ! command -v launchctl >/dev/null 2>&1; then
        return
    fi

    if [[ -f "$LAUNCHD_PLIST" ]]; then
        $SUDO_CMD launchctl bootout system "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
        $SUDO_CMD launchctl unload -w "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
        $SUDO_CMD rm -f "$LAUNCHD_PLIST"
    fi
    $SUDO_CMD launchctl disable "system/${LAUNCHD_LABEL}" >/dev/null 2>&1 || true
}

remove_files() {
    $SUDO_CMD rm -f "${BIN_DIR}/${BINARY_NAME}"
    $SUDO_CMD rm -f "$CONFIG_FILE"
    $SUDO_CMD rm -f "$PID_FILE"
    if [[ "$KEEP_LOGS" != "true" ]]; then
        $SUDO_CMD rm -f "$LOG_FILE"
    fi

    # 尽量清理空目录（不会影响同目录下的其他文件）。
    $SUDO_CMD rmdir "$BIN_DIR" >/dev/null 2>&1 || true
    $SUDO_CMD rmdir "$INSTALL_ROOT" >/dev/null 2>&1 || true
    $SUDO_CMD rmdir "$CONFIG_DIR" >/dev/null 2>&1 || true
    if [[ "$KEEP_LOGS" != "true" ]]; then
        $SUDO_CMD rmdir "$LOG_DIR" >/dev/null 2>&1 || true
    fi
}

main() {
    info "Better Monitor Agent 卸载程序"
    parse_args "$@"
    prepare_env
    confirm_uninstall

    info "停止并移除服务（若存在）..."
    stop_and_remove_systemd
    stop_and_remove_openrc
    stop_and_remove_launchd

    info "删除文件..."
    remove_files

    info "===================================="
    info "卸载完成！"
}

main "$@"

