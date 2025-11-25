#!/bin/bash

# Better Monitor Agent 一键安装脚本
# 使用方法: curl -fsSL https://your-domain.com/install-agent.sh | bash -s -- --server-id 1 --secret-key your-secret-key --server https://your-dashboard.com

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 默认配置
INSTALL_DIR="/opt/better-monitor"
SERVICE_NAME="better-monitor-agent"
BINARY_NAME="better-monitor-agent"
GITHUB_REPO="EnderKC/BetterMonitor"  # 默认仓库，会从服务器获取

# 参数
SERVER_ID=""
SECRET_KEY=""
SERVER_URL=""

# 打印信息
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

# 解析命令行参数
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --server-id)
                SERVER_ID="$2"
                shift 2
                ;;
            --secret-key)
                SECRET_KEY="$2"
                shift 2
                ;;
            --server)
                SERVER_URL="$2"
                shift 2
                ;;
            --repo)
                GITHUB_REPO="$2"
                shift 2
                ;;
            *)
                error "未知参数: $1"
                ;;
        esac
    done

    # 验证必需参数
    if [ -z "$SERVER_ID" ] || [ -z "$SECRET_KEY" ] || [ -z "$SERVER_URL" ]; then
        error "缺少必需参数。使用方法: $0 --server-id <ID> --secret-key <KEY> --server <URL>"
    fi
}

# 检测系统架构
detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l)
            echo "armv7"
            ;;
        *)
            error "不支持的架构: $arch"
            ;;
    esac
}

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "$ID"
    elif [ "$(uname)" == "Darwin" ]; then
        echo "darwin"
    else
        error "无法检测操作系统"
    fi
}

# 从服务器获取发布仓库信息
get_release_repo() {
    info "从服务器获取配置信息..."
    local repo=$(curl -fsSL "${SERVER_URL}/api/public/settings" | grep -o '"agent_release_repo":"[^"]*"' | cut -d'"' -f4)
    if [ -n "$repo" ]; then
        GITHUB_REPO="$repo"
        info "使用仓库: $GITHUB_REPO"
    else
        warn "无法从服务器获取仓库信息，使用默认仓库: $GITHUB_REPO"
    fi
}

# 获取最新版本
get_latest_version() {
    info "获取最新版本..."
    local latest_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    local version=$(curl -fsSL "$latest_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "无法获取最新版本信息"
    fi

    echo "$version"
}

# 下载并安装agent
install_agent() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local version=$(get_latest_version)

    info "检测到系统: $os, 架构: $arch"
    info "最新版本: $version"

    # 构建下载URL
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/better-monitor-agent-${os}-${arch}"

    info "下载地址: $download_url"
    info "开始下载..."

    # 创建安装目录
    sudo mkdir -p "$INSTALL_DIR"

    # 下载二进制文件
    if ! sudo curl -fsSL -o "${INSTALL_DIR}/${BINARY_NAME}" "$download_url"; then
        error "下载失败，请检查网络连接或版本是否存在"
    fi

    # 添加执行权限
    sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

    info "Agent 下载完成"
}

# 创建systemd服务
create_systemd_service() {
    info "创建 systemd 服务..."

    sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null <<EOF
[Unit]
Description=Better Monitor Agent
After=network.target

[Service]
Type=simple
User=root
ExecStart=${INSTALL_DIR}/${BINARY_NAME} --server ${SERVER_URL} --server-id ${SERVER_ID} --secret-key ${SECRET_KEY}
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    # 重载systemd配置
    sudo systemctl daemon-reload

    info "systemd 服务创建完成"
}

# 启动服务
start_service() {
    info "启动服务..."

    sudo systemctl enable ${SERVICE_NAME}
    sudo systemctl start ${SERVICE_NAME}

    # 等待服务启动
    sleep 2

    # 检查服务状态
    if sudo systemctl is-active --quiet ${SERVICE_NAME}; then
        info "服务启动成功！"
        info "查看服务状态: sudo systemctl status ${SERVICE_NAME}"
        info "查看日志: sudo journalctl -u ${SERVICE_NAME} -f"
    else
        error "服务启动失败，请查看日志: sudo journalctl -u ${SERVICE_NAME} -n 50"
    fi
}

# 主函数
main() {
    info "Better Monitor Agent 一键安装脚本"
    info "================================"

    # 检查是否为root或有sudo权限
    if [ "$EUID" -ne 0 ] && ! sudo -n true 2>/dev/null; then
        error "此脚本需要 root 权限或 sudo 权限"
    fi

    # 解析参数
    parse_args "$@"

    # 获取发布仓库
    get_release_repo

    # 安装agent
    install_agent

    # 创建systemd服务
    create_systemd_service

    # 启动服务
    start_service

    info "================================"
    info "安装完成！"
    info "服务器ID: ${SERVER_ID}"
    info "服务器地址: ${SERVER_URL}"
    info ""
    info "常用命令:"
    info "  查看状态: sudo systemctl status ${SERVICE_NAME}"
    info "  停止服务: sudo systemctl stop ${SERVICE_NAME}"
    info "  重启服务: sudo systemctl restart ${SERVICE_NAME}"
    info "  查看日志: sudo journalctl -u ${SERVICE_NAME} -f"
}

# 执行主函数
main "$@"
