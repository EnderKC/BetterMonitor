#!/bin/bash

# Better Monitor Agent 一键安装脚本
# 使用方法:
#   curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
#     | bash -s -- --server-id 1 --secret-key your-secret --server https://dashboard.example.com

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
SERVICE_MANAGER=""
LAUNCHD_LABEL="com.better-monitor.agent"
LAUNCHD_PLIST="/Library/LaunchDaemons/${LAUNCHD_LABEL}.plist"
GITHUB_REPO="EnderKC/BetterMonitor"
RELEASE_CHANNEL="stable"
DOWNLOAD_MIRROR=""
SERVER_ID=""
SECRET_KEY=""
SERVER_URL=""
SERVER_API_URL=""
REGISTER_TOKEN=""
HEARTBEAT_INTERVAL="10s"
MONITOR_INTERVAL="30s"
TMP_DIR=""
SUDO_CMD=""

usage() {
    cat <<'EOF'
用法: install-agent.sh --server-id <ID> --secret-key <KEY> --server <URL> [选项]

必填参数:
  --server-id <ID>        Dashboard 中服务器的 ID
  --secret-key <KEY>      Dashboard 中服务器的密钥
  --server <URL>          Dashboard 地址 (例如: https://dashboard.example.com:3333)

可选参数:
  --repo <OWNER/REPO>     自定义 Agent Release 仓库 (默认: EnderKC/BetterMonitor)
  --channel <stable|prerelease|nightly>
                          指定 release 通道 (默认: stable)
  --mirror <URL>          GitHub Release 下载镜像地址
  --token <TOKEN>         注册令牌，写入配置文件备用
  -h, --help              显示帮助
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

cleanup() {
    if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
        rm -rf "$TMP_DIR"
    fi
}

create_tmp_dir() {
    # macOS(BSD mktemp) 需要 template；Linux/GNU mktemp 则可直接 mktemp -d。
    # 使用多种兼容写法依次尝试，避免在 set -e 下直接退出。
    TMP_DIR="$(
        mktemp -d 2>/dev/null \
            || mktemp -d -t better-monitor-agent 2>/dev/null \
            || mktemp -d -t better-monitor-agent.XXXXXX 2>/dev/null \
            || true
    )"
    if [[ -z "$TMP_DIR" || ! -d "$TMP_DIR" ]]; then
        error "无法创建临时目录 (mktemp)，请检查系统环境"
    fi
    trap cleanup EXIT
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        error "未找到命令: $1，请先安装后再执行脚本"
    fi
}

parse_args() {
    if [[ $# -eq 0 ]]; then
        usage
        exit 1
    fi

    while [[ $# -gt 0 ]]; do
        case "$1" in
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
            --channel)
                RELEASE_CHANNEL="$2"
                shift 2
                ;;
            --mirror)
                DOWNLOAD_MIRROR="$2"
                shift 2
                ;;
            --token|--register-token)
                REGISTER_TOKEN="$2"
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

    if [[ -z "$SERVER_ID" || -z "$SECRET_KEY" || -z "$SERVER_URL" ]]; then
        usage
        error "缺少必需参数，请提供 --server-id、--secret-key 和 --server"
    fi
}

normalize_server_url() {
    SERVER_URL="${SERVER_URL%/}"
    if [[ -z "$SERVER_URL" ]]; then
        error "服务器地址不能为空"
    fi

    if [[ "$SERVER_URL" =~ ^https?:// ]]; then
        SERVER_API_URL="$SERVER_URL"
    else
        SERVER_API_URL="http://${SERVER_URL}"
    fi
    info "服务器地址: ${SERVER_API_URL}"
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l|armv6l|armv8l|armhf)
            echo "arm"
            ;;
        i386|i686)
            echo "386"
            ;;
        *)
            error "不支持的架构: $arch"
            ;;
    esac
}

detect_os() {
    local kernel
    kernel="$(uname -s)"
    case "$kernel" in
        Linux*)
            echo "linux"
            ;;
        Darwin*)
            echo "darwin"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "windows"
            ;;
        *)
            error "不支持的操作系统: $kernel"
            ;;
    esac
}

detect_service_manager() {
    local os
    os="$(detect_os)"

    if [[ "$os" == "darwin" ]]; then
        if command -v launchctl >/dev/null 2>&1; then
            echo "launchd"
        else
            echo "none"
        fi
        return
    fi

    # Linux: 优先识别 systemd（且需系统实际运行 systemd），其次 OpenRC。
    if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
        echo "systemd"
        return
    fi

    if command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
        if [[ -x /sbin/openrc-run || -x /usr/bin/openrc-run ]]; then
            echo "openrc"
            return
        fi
    fi

    echo "none"
}

init_service_manager() {
    if [[ -n "${SERVICE_MANAGER}" ]]; then
        return
    fi
    SERVICE_MANAGER="$(detect_service_manager)"
}

stop_existing_service() {
    init_service_manager

    case "$SERVICE_MANAGER" in
        systemd)
            if command -v systemctl >/dev/null 2>&1; then
                # 服务不存在/未运行时也不会影响安装流程
                $SUDO_CMD systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
            fi
            ;;
        openrc)
            if [[ -x "/etc/init.d/${SERVICE_NAME}" ]]; then
                $SUDO_CMD rc-service "${SERVICE_NAME}" stop >/dev/null 2>&1 || true
            fi
            ;;
        launchd)
            if [[ -f "$LAUNCHD_PLIST" ]]; then
                $SUDO_CMD launchctl bootout system "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
                $SUDO_CMD launchctl unload -w "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
            fi
            ;;
        *)
            return
            ;;
    esac
}

fetch_public_release_config() {
    info "从 Dashboard 获取公共设置..."
    local response
    if ! response=$(curl -fsSL "${SERVER_API_URL}/api/public/settings" 2>/dev/null); then
        warn "无法从 Dashboard 获取公共配置，使用默认仓库 ${GITHUB_REPO}"
        return
    fi

    local parsed
    if ! parsed=$(
        BM_JSON="$response" python3 <<'PY' 2>/dev/null
import json, os
data=json.loads(os.environ.get("BM_JSON") or "{}")
repo=data.get("agent_release_repo") or ""
channel=data.get("agent_release_channel") or ""
mirror=data.get("agent_release_mirror") or ""
print(f"{repo}|{channel}|{mirror}")
PY
    ); then
        warn "解析 Dashboard 公共设置失败，使用默认配置"
        return
    fi

    IFS='|' read -r repo channel mirror <<<"$parsed"
    if [[ -n "$repo" ]]; then
        GITHUB_REPO="$repo"
    fi
    if [[ -n "$channel" ]]; then
        RELEASE_CHANNEL="$channel"
    fi
    if [[ -n "$mirror" ]]; then
        DOWNLOAD_MIRROR="$mirror"
    fi
    info "Release 仓库: ${GITHUB_REPO} (channel: ${RELEASE_CHANNEL})"
}

fetch_agent_settings() {
    info "同步服务器配置..."
    local response
    if ! response=$(curl -fsSL \
        -H "X-Secret-Key: ${SECRET_KEY}" \
        "${SERVER_API_URL}/api/servers/${SERVER_ID}/settings" 2>/dev/null); then
        warn "无法通过 API 获取服务器设置，将使用默认心跳/监控间隔"
        return
    fi

    local parsed
    if ! parsed=$(
        BM_JSON="$response" python3 <<'PY' 2>/dev/null
import json, os
data=json.loads(os.environ.get("BM_JSON") or "{}")
if isinstance(data, dict) and data.get("success") is False:
    raise SystemExit(data.get("message") or "dashboard 返回错误")
hb=data.get("heartbeat_interval") or ""
mon=data.get("monitor_interval") or ""
repo=data.get("agent_release_repo") or ""
channel=data.get("agent_release_channel") or ""
mirror=data.get("agent_release_mirror") or ""
print(f"{hb}|{mon}|{repo}|{channel}|{mirror}")
PY
    ); then
        warn "解析服务器设置失败，将继续使用默认值"
        return
    fi

    IFS='|' read -r hb mon repo channel mirror <<<"$parsed"
    if [[ -n "$hb" ]]; then
        HEARTBEAT_INTERVAL="$hb"
    fi
    if [[ -n "$mon" ]]; then
        MONITOR_INTERVAL="$mon"
    fi
    if [[ -n "$repo" ]]; then
        GITHUB_REPO="$repo"
    fi
    if [[ -n "$channel" ]]; then
        RELEASE_CHANNEL="$channel"
    fi
    if [[ "$mirror" != "" ]]; then
        DOWNLOAD_MIRROR="$mirror"
    fi
    info "心跳/监控间隔: ${HEARTBEAT_INTERVAL} / ${MONITOR_INTERVAL}"
}

select_release_asset() {
    local os="$1"
    local arch="$2"
    # 注意：不要把完整 Release JSON 放进环境变量里（可能触发 "Argument list too long"）。
    # 直接从 stdin 读取更稳妥。
    BM_RELEASE_CHANNEL="${RELEASE_CHANNEL}" BM_OS="$os" BM_ARCH="$arch" \
        python3 -c '
import json, os, sys

channel=os.environ.get("BM_RELEASE_CHANNEL","stable").lower()
os_name=os.environ["BM_OS"]
arch=os.environ["BM_ARCH"]

try:
    data=json.load(sys.stdin)
except Exception as e:
    raise SystemExit(f"解析 Release JSON 失败: {e}")

if isinstance(data, dict):
    data=[data]

def match_release(rel):
    if rel.get("draft"):
        return False
    if channel == "stable":
        return not rel.get("prerelease")
    if channel == "prerelease":
        return bool(rel.get("prerelease"))
    if channel == "nightly":
        name=(rel.get("tag_name") or "") + " " + (rel.get("name") or "")
        return "nightly" in name.lower()
    return False

release=next((r for r in data if match_release(r)), None)
if release is None and channel in {"prerelease","nightly"}:
    release=next((r for r in data if not r.get("draft")), None)
if release is None:
    release=data[0] if data else None
if release is None:
    raise SystemExit("没有找到可用的 Release")

assets=release.get("assets") or []
tag=release.get("tag_name") or ""
version=tag.lstrip("vV")
expected_suffix=f"{os_name}-{arch}"
preferred_patterns=[]
extensions=["", ".exe", ".tar.gz", ".tgz", ".zip"]

if version:
    preferred_patterns.append(f"better-monitor-agent-{version}-{expected_suffix}")
    preferred_patterns.append(f"better-monitor-agent-{version}-{os_name}-{arch}")
    preferred_patterns.append(f"better-monitor-agent-{version}-{os_name}")

preferred_patterns.append(f"better-monitor-agent-{expected_suffix}")
preferred_patterns.append(f"better-monitor-agent-{os_name}-{arch}")
preferred_patterns.append(f"better-monitor-agent-{os_name}")

def find_by_pattern():
    for pattern in preferred_patterns:
        for ext in extensions:
            target=pattern+ext
            for asset in assets:
                name=asset.get("name") or ""
                if name == target:
                    return asset
    return None

selected=find_by_pattern()
if selected is None:
    for asset in assets:
        name=asset.get("name") or ""
        if "better-monitor-agent" in name and expected_suffix in name:
            selected=asset
            break
if selected is None:
    raise SystemExit("未找到匹配的 Release 资产")

print("|".join([
    tag or "",
    selected.get("name") or "",
    selected.get("browser_download_url") or ""
]))
'
}

download_agent() {
    local os arch asset_info tag asset_name asset_url download_url
    os="$(detect_os)"
    arch="$(detect_arch)"
    info "检测到系统: ${os}/${arch}"

    local releases_json
    releases_json=$(curl -fsSL \
        -H "Accept: application/vnd.github+json" \
        -H "User-Agent: better-monitor-agent-installer" \
        "https://api.github.com/repos/${GITHUB_REPO}/releases?per_page=20") \
        || error "无法从 GitHub 获取 Release 信息，请检查仓库 ${GITHUB_REPO}"

    asset_info="$(select_release_asset "$os" "$arch" <<<"$releases_json")" \
        || error "无法解析 Release 信息，请确认仓库 ${GITHUB_REPO} 是否存在对应平台的二进制"

    IFS='|' read -r tag asset_name asset_url <<<"$asset_info"
    if [[ -z "$asset_url" ]]; then
        error "未找到可下载的 Release 资产"
    fi

    info "将安装版本: ${tag} (${asset_name})"

    download_url="$asset_url"
    if [[ -n "$DOWNLOAD_MIRROR" && "$asset_url" == https://github.com/* ]]; then
        download_url="${DOWNLOAD_MIRROR%/}${asset_url#https://github.com}"
        info "使用镜像下载: ${DOWNLOAD_MIRROR}"
    fi

    create_tmp_dir

    local tmp_bin="${TMP_DIR}/${BINARY_NAME}"
    info "开始下载 Agent..."
    curl -fL --retry 3 --retry-delay 2 -o "$tmp_bin" "$download_url" \
        || error "下载失败，请稍后再试"

    chmod +x "$tmp_bin"

    # 若是升级场景，先停止服务再写入目标路径，避免出现 "text file busy"。
    stop_existing_service

    info "安装 Agent 到 ${BIN_DIR}"
    $SUDO_CMD mkdir -p "$BIN_DIR"
    $SUDO_CMD install -m 0755 "$tmp_bin" "${BIN_DIR}/${BINARY_NAME}"
}

create_config() {
    info "写入配置文件 ${CONFIG_FILE}"
    $SUDO_CMD mkdir -p "$CONFIG_DIR"
    $SUDO_CMD mkdir -p "$LOG_DIR"

    $SUDO_CMD tee "$CONFIG_FILE" >/dev/null <<EOF
server_url: "${SERVER_API_URL}"
server_id: ${SERVER_ID}
secret_key: "${SECRET_KEY}"
register_token: "${REGISTER_TOKEN}"
heartbeat_interval: "${HEARTBEAT_INTERVAL}"
monitor_interval: "${MONITOR_INTERVAL}"
log_level: "info"
log_file: "${LOG_DIR}/agent.log"
enable_cpu_monitor: true
enable_mem_monitor: true
enable_disk_monitor: true
enable_network_monitor: true
update_repo: "${GITHUB_REPO}"
update_channel: "${RELEASE_CHANNEL}"
update_mirror: "${DOWNLOAD_MIRROR}"
EOF

    $SUDO_CMD chmod 600 "$CONFIG_FILE"
}

service_exists() {
    command -v systemctl >/dev/null 2>&1 && $SUDO_CMD systemctl list-unit-files | grep -Fq "${SERVICE_NAME}.service"
}

create_systemd_service() {
    info "创建 systemd 服务..."
    if service_exists; then
        $SUDO_CMD systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
    fi

    $SUDO_CMD tee "/etc/systemd/system/${SERVICE_NAME}.service" >/dev/null <<EOF
[Unit]
Description=Better Monitor Agent
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_ROOT}
ExecStart=${BIN_DIR}/${BINARY_NAME} --config ${CONFIG_FILE}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    $SUDO_CMD systemctl daemon-reload
    $SUDO_CMD systemctl enable "${SERVICE_NAME}" >/dev/null
}

create_openrc_service() {
    local openrc_run="/sbin/openrc-run"
    if [[ -x /usr/bin/openrc-run ]]; then
        openrc_run="/usr/bin/openrc-run"
    fi

    info "创建 OpenRC 服务..."
    $SUDO_CMD tee "/etc/init.d/${SERVICE_NAME}" >/dev/null <<EOF
#!${openrc_run}
description="Better Monitor Agent"

command="${BIN_DIR}/${BINARY_NAME}"
command_args="--config ${CONFIG_FILE}"
command_background=true
pidfile="/run/${SERVICE_NAME}.pid"

depend() {
  need net
}
EOF

    $SUDO_CMD chmod 0755 "/etc/init.d/${SERVICE_NAME}"

    # rc-update 在某些精简环境/容器里可能不可用或无默认 runlevel，失败不应中断安装。
    $SUDO_CMD rc-update add "${SERVICE_NAME}" default >/dev/null 2>&1 || true
}

create_launchd_service() {
    if ! command -v launchctl >/dev/null 2>&1; then
        warn "未检测到 launchctl，跳过服务安装，请手动管理 ${BIN_DIR}/${BINARY_NAME}"
        return
    fi

    info "创建 launchd 服务..."

    # 如果服务已加载，先卸载，避免 bootstrap 报重复。
    if [[ -f "$LAUNCHD_PLIST" ]]; then
        $SUDO_CMD launchctl bootout system "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
        $SUDO_CMD launchctl unload -w "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
    fi

    $SUDO_CMD mkdir -p "$(dirname "$LAUNCHD_PLIST")"
    $SUDO_CMD tee "$LAUNCHD_PLIST" >/dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LAUNCHD_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${BIN_DIR}/${BINARY_NAME}</string>
    <string>--config</string>
    <string>${CONFIG_FILE}</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${INSTALL_ROOT}</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/agent.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/agent.log</string>
</dict>
</plist>
EOF

    # LaunchDaemon 要求 root 拥有且不可写。
    $SUDO_CMD chown root:wheel "$LAUNCHD_PLIST" >/dev/null 2>&1 || $SUDO_CMD chown root:admin "$LAUNCHD_PLIST" >/dev/null 2>&1 || true
    $SUDO_CMD chmod 0644 "$LAUNCHD_PLIST"

    # macOS 10.13+ 推荐 bootstrap；旧版本 fallback 到 load。
    if ! $SUDO_CMD launchctl bootstrap system "$LAUNCHD_PLIST" >/dev/null 2>&1; then
        $SUDO_CMD launchctl load -w "$LAUNCHD_PLIST" >/dev/null 2>&1 \
            || warn "launchd 服务加载失败，请手动执行: sudo launchctl load -w ${LAUNCHD_PLIST}"
    fi
    $SUDO_CMD launchctl enable "system/${LAUNCHD_LABEL}" >/dev/null 2>&1 || true
}

install_service() {
    init_service_manager
    case "$SERVICE_MANAGER" in
        systemd)
            create_systemd_service
            ;;
        openrc)
            create_openrc_service
            ;;
        launchd)
            create_launchd_service
            ;;
        none)
            warn "未检测到 systemd/OpenRC/launchd，跳过服务安装，请手动管理 ${BIN_DIR}/${BINARY_NAME}"
            ;;
        *)
            warn "未知服务管理器: ${SERVICE_MANAGER}，跳过服务安装"
            ;;
    esac
}

start_systemd_service() {
    info "启动服务..."
    $SUDO_CMD systemctl restart "${SERVICE_NAME}"
    sleep 2
    if $SUDO_CMD systemctl is-active --quiet "${SERVICE_NAME}"; then
        info "Agent 服务启动成功 (systemd)"
    else
        warn "Agent 服务未成功启动，请使用 'systemctl status ${SERVICE_NAME}' 查看详情"
    fi
}

start_openrc_service() {
    info "启动服务..."
    $SUDO_CMD rc-service "${SERVICE_NAME}" restart >/dev/null 2>&1 || $SUDO_CMD rc-service "${SERVICE_NAME}" start >/dev/null 2>&1 || true
    sleep 2
    if $SUDO_CMD rc-service "${SERVICE_NAME}" status >/dev/null 2>&1; then
        info "Agent 服务启动成功 (OpenRC)"
    else
        warn "Agent 服务未成功启动，请使用 'rc-service ${SERVICE_NAME} status' 查看详情"
    fi
}

start_launchd_service() {
    info "启动服务..."
    # RunAtLoad 通常会自动启动，这里 kickstart 一次确保立即生效。
    if $SUDO_CMD launchctl kickstart -k "system/${LAUNCHD_LABEL}" >/dev/null 2>&1; then
        info "Agent 服务启动成功 (launchd)"
        return
    fi
    warn "Agent 服务可能未成功启动，请使用 'sudo launchctl print system/${LAUNCHD_LABEL}' 查看详情"
}

start_service() {
    case "$SERVICE_MANAGER" in
        systemd)
            start_systemd_service
            ;;
        openrc)
            start_openrc_service
            ;;
        launchd)
            start_launchd_service
            ;;
        *)
            return
            ;;
    esac
}

prepare_env() {
    if [[ "$EUID" -ne 0 ]]; then
        if ! command -v sudo >/dev/null 2>&1; then
            error "脚本需要 root 权限或 sudo，请以 root 运行或安装 sudo"
        fi
        SUDO_CMD="sudo"
    fi

    require_cmd curl
    require_cmd python3
}

main() {
    info "Better Monitor Agent 安装程序"
    parse_args "$@"
    prepare_env
    normalize_server_url
    fetch_public_release_config
    fetch_agent_settings
    download_agent
    create_config
    install_service
    start_service

    info "===================================="
    info "安装完成！"
    info "配置文件: ${CONFIG_FILE}"
    info "日志文件: ${LOG_DIR}/agent.log"
    if [[ "${SERVICE_MANAGER}" == "systemd" ]]; then
        info "查看状态: sudo systemctl status ${SERVICE_NAME}"
        info "查看日志: sudo journalctl -u ${SERVICE_NAME} -f"
    elif [[ "${SERVICE_MANAGER}" == "openrc" ]]; then
        info "查看状态: sudo rc-service ${SERVICE_NAME} status"
        info "启动服务: sudo rc-service ${SERVICE_NAME} start"
        info "停止服务: sudo rc-service ${SERVICE_NAME} stop"
    elif [[ "${SERVICE_MANAGER}" == "launchd" ]]; then
        info "查看状态: sudo launchctl print system/${LAUNCHD_LABEL}"
        info "重启服务: sudo launchctl kickstart -k system/${LAUNCHD_LABEL}"
        info "卸载服务: sudo launchctl bootout system ${LAUNCHD_PLIST}"
    fi
}

main "$@"
