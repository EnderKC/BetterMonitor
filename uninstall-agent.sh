#!/bin/bash

# Better Monitor Agent 一键卸载脚本（Linux/macOS/Android）
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
ANDROID_MODE="auto"

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
  --android-mode <auto|termux|root>
                    Android 卸载模式（默认: auto；Termux/Root 可手动指定）
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
            --android-mode|--android)
                ANDROID_MODE="$2"
                shift 2
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

    case "${ANDROID_MODE}" in
        auto|termux|root)
            ;;
        *)
            error "无效的 --android-mode: ${ANDROID_MODE}（可选: auto|termux|root）"
            ;;
    esac
}

is_android() {
    if command -v getprop >/dev/null 2>&1; then
        return 0
    fi
    if [[ -n "${ANDROID_ROOT:-}" || -n "${ANDROID_DATA:-}" ]]; then
        return 0
    fi
    if [[ -d "/system" && -d "/system/bin" ]]; then
        return 0
    fi
    return 1
}

is_termux() {
    if [[ -n "${TERMUX_VERSION:-}" ]]; then
        return 0
    fi
    if [[ -n "${PREFIX:-}" && "${PREFIX}" == /data/data/com.termux/* ]]; then
        return 0
    fi
    if command -v termux-info >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

apply_android_defaults() {
    local mode="${ANDROID_MODE}"
    if [[ "$mode" == "auto" ]]; then
        if is_termux; then
            mode="termux"
        else
            mode="root"
        fi
    fi

    case "$mode" in
        termux)
            if ! is_termux; then
                error "未检测到 Termux 环境，无法使用 --android-mode termux（可改用 --android-mode root）"
            fi
            if [[ -z "${PREFIX:-}" ]]; then
                PREFIX="/data/data/com.termux/files/usr"
            fi

            INSTALL_ROOT="${PREFIX}/opt/better-monitor"
            BIN_DIR="${INSTALL_ROOT}/bin"
            CONFIG_DIR="${INSTALL_ROOT}/etc"
            CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
            LOG_DIR="${INSTALL_ROOT}/logs"
            LOG_FILE="${LOG_DIR}/agent.log"
            SUDO_CMD=""
            ;;
        root)
            if [[ "$EUID" -ne 0 ]]; then
                error "Android root 模式需要 root 权限，请以 root/su 运行，或改用 --android-mode termux（Termux 无需 root）"
            fi

            local base="/data/adb/better-monitor"
            if [[ ! -d "/data/adb" ]]; then
                base="/data/local/better-monitor"
            fi

            INSTALL_ROOT="$base"
            BIN_DIR="${INSTALL_ROOT}/bin"
            CONFIG_DIR="${INSTALL_ROOT}/etc"
            CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
            LOG_DIR="${INSTALL_ROOT}/logs"
            LOG_FILE="${LOG_DIR}/agent.log"
            SUDO_CMD=""
            ;;
        *)
            error "未知 Android 模式: ${mode}"
            ;;
    esac

    ANDROID_MODE="$mode"
}

prepare_env() {
    local kernel
    kernel="$(uname -s)"

    if [[ "$kernel" == Linux* ]] && is_android; then
        apply_android_defaults
        return
    fi

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
    if is_android; then
        if [[ "${ANDROID_MODE}" == "termux" ]]; then
            warn "  - 服务定义: Termux(runit)/Termux:Boot（若存在）"
        else
            warn "  - 服务定义: Magisk service.d（若存在）"
        fi
    else
        warn "  - 服务定义: systemd/OpenRC/launchd（若存在）"
    fi

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

stop_and_remove_termux() {
    local prefix="${PREFIX:-/data/data/com.termux/files/usr}"
    local service_dir="${prefix}/var/service/${SERVICE_NAME}"
    local boot_script=""
    if [[ -n "${HOME:-}" ]]; then
        boot_script="${HOME}/.termux/boot/${SERVICE_NAME}.sh"
    fi

    if command -v sv >/dev/null 2>&1 && [[ -d "$service_dir" ]]; then
        sv down "$service_dir" >/dev/null 2>&1 || true
        sv kill "$service_dir" >/dev/null 2>&1 || true
    fi

    if command -v pkill >/dev/null 2>&1; then
        pkill -f "${BIN_DIR}/${BINARY_NAME}" >/dev/null 2>&1 || true
        pkill -f "${BINARY_NAME} --config ${CONFIG_FILE}" >/dev/null 2>&1 || true
    fi

    rm -rf "$service_dir" >/dev/null 2>&1 || true
    if [[ -n "$boot_script" && -f "$boot_script" ]]; then
        rm -f "$boot_script" >/dev/null 2>&1 || true
    fi
}

stop_and_remove_magisk() {
    if command -v pkill >/dev/null 2>&1; then
        pkill -f "${BIN_DIR}/${BINARY_NAME}" >/dev/null 2>&1 || true
        pkill -f "${BINARY_NAME} --config ${CONFIG_FILE}" >/dev/null 2>&1 || true
    fi
    if command -v killall >/dev/null 2>&1; then
        killall "${BINARY_NAME}" >/dev/null 2>&1 || true
    fi

    rm -f "/data/adb/service.d/${SERVICE_NAME}.sh" >/dev/null 2>&1 || true
}

remove_files() {
    $SUDO_CMD rm -f "${BIN_DIR}/${BINARY_NAME}"
    $SUDO_CMD rm -f "$CONFIG_FILE"
    $SUDO_CMD rm -f "$PID_FILE"
    if [[ "$KEEP_LOGS" != "true" ]]; then
        $SUDO_CMD rm -f "$LOG_FILE"
        $SUDO_CMD rm -f "${LOG_DIR}/agent.stdout.log" "${LOG_DIR}/agent.stderr.log"
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
    if is_android; then
        if [[ "${ANDROID_MODE}" == "termux" ]]; then
            stop_and_remove_termux
        else
            stop_and_remove_magisk
        fi
    else
        stop_and_remove_systemd
        stop_and_remove_openrc
        stop_and_remove_launchd
    fi

    info "删除文件..."
    remove_files

    info "===================================="
    info "卸载完成！"
}

main "$@"
