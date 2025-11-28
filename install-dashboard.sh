#!/bin/bash

#==============================================================================
# Better Monitor Dashboard 一键安装/管理脚本
# 版本: 1.0.0
# 描述: 用于 Docker 环境下的 Better Monitor 面板安装、升级、卸载和数据迁移
#==============================================================================

set -euo pipefail
shopt -s nullglob

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置变量（支持通过环境变量覆盖）
INSTALL_DIR="${INSTALL_DIR:-/opt/better-monitor}"
DOCKER_IMAGE="${DOCKER_IMAGE:-enderhkc/better-monitor:latest}"
CONTAINER_NAME="${CONTAINER_NAME:-better-monitor}"
PORT="${PORT:-3333}"
DATA_DIR="${DATA_DIR:-${INSTALL_DIR}/data}"
LOGS_DIR="${LOGS_DIR:-${INSTALL_DIR}/logs}"
BACKUP_DIR="${BACKUP_DIR:-${INSTALL_DIR}/backups}"
ENV_FILE="${ENV_FILE:-${INSTALL_DIR}/.env}"
COMPOSE_FILE="${COMPOSE_FILE:-${INSTALL_DIR}/docker-compose.yml}"
TZ="${TZ:-Asia/Shanghai}"
COMPOSE_BIN=()
JWT_SECRET="${JWT_SECRET:-}"
ENV_LOADED_PATH=""

#==============================================================================
# 工具函数
#==============================================================================

# 打印信息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# 打印成功
print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# 打印警告
print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 打印错误
print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否为 root 用户
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "此脚本需要 root 权限运行"
        echo "请使用: sudo $0"
        exit 1
    fi
}

# 检查 Docker 是否安装
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker 未安装"
        echo ""
        echo "请先安装 Docker:"
        echo "  curl -fsSL https://get.docker.com | sh"
        echo "  或访问: https://docs.docker.com/engine/install/"
        exit 1
    fi

    if ! docker ps &> /dev/null; then
        print_error "Docker 服务未运行或当前用户无权限访问"
        echo "请启动 Docker 服务: systemctl start docker"
        exit 1
    fi

    print_success "Docker 环境检查通过"
}

# 检测 Docker Compose
detect_compose_cmd() {
    if docker compose version &>/dev/null; then
        COMPOSE_BIN=(docker compose)
        return 0
    fi

    if command -v docker-compose &>/dev/null; then
        COMPOSE_BIN=(docker-compose)
        return 0
    fi

    COMPOSE_BIN=()
    return 1
}

ensure_compose() {
    if detect_compose_cmd; then
        return 0
    fi

    print_warning "Docker Compose 未安装，将使用 docker run 方式部署"
    return 1
}

run_compose_cmd() {
    if [[ ${#COMPOSE_BIN[@]} -eq 0 ]]; then
        return 1
    fi
    (cd "${INSTALL_DIR}" && "${COMPOSE_BIN[@]}" "$@")
}

# 检查端口是否被占用
check_port() {
    if netstat -tuln 2>/dev/null | grep -q ":${PORT} " || ss -tuln 2>/dev/null | grep -q ":${PORT} "; then
        print_warning "端口 ${PORT} 已被占用"
        read -p "是否继续安装？(y/n): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

# 创建必要的目录
create_directories() {
    print_info "创建必要的目录..."
    mkdir -p "${INSTALL_DIR}"
    mkdir -p "${DATA_DIR}"
    mkdir -p "${LOGS_DIR}"
    mkdir -p "${BACKUP_DIR}"
    print_success "目录创建完成"
}

# 生成随机 JWT Secret
generate_jwt_secret() {
    openssl rand -base64 32 2>/dev/null || cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1
}

# 创建 docker-compose.yml
create_docker_compose() {
    local jwt_secret="${1:-$(generate_jwt_secret)}"

    print_info "创建 docker-compose.yml 配置文件..."

    cat > "${COMPOSE_FILE}" <<EOF
version: '3.8'

services:
  better-monitor:
    image: ${DOCKER_IMAGE}
    container_name: ${CONTAINER_NAME}
    restart: unless-stopped
    ports:
      - "${PORT}:3333"
    volumes:
      - ${DATA_DIR}:/app/data:rw
      - ${LOGS_DIR}:/app/logs:rw
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - TZ=${TZ}
      - JWT_SECRET=${jwt_secret}
      - VERSION=latest
    security_opt:
      - no-new-privileges:true
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3333/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
EOF

    print_success "配置文件创建完成"
}

# 创建 .env 文件
create_env_file() {
    local jwt_secret="${1:-$(generate_jwt_secret)}"

    cat > "${ENV_FILE}" <<EOF
# Better Monitor 配置文件
# 生成时间: $(date '+%Y-%m-%d %H:%M:%S')

# JWT 密钥（请妥善保管）
JWT_SECRET=${jwt_secret}

# 端口配置
PORT=${PORT}

# 时区配置
TZ=${TZ}

# Docker 镜像
DOCKER_IMAGE=${DOCKER_IMAGE}
EOF

    chmod 600 "${ENV_FILE}"
}

load_env_file() {
    if [ -f "${ENV_FILE}" ]; then
        if [[ "${ENV_LOADED_PATH}" != "${ENV_FILE}" ]]; then
            print_info "加载环境配置: ${ENV_FILE}"
            ENV_LOADED_PATH="${ENV_FILE}"
        fi
        # shellcheck disable=SC1090
        source "${ENV_FILE}"
    fi
}

cleanup_old_backups() {
    local removable
    removable=$(ls -1t "${BACKUP_DIR}"/better-monitor-backup-*.tar.gz 2>/dev/null | tail -n +6 || true)
    if [[ -z "${removable}" ]]; then
        return
    fi

    print_info "清理旧备份..."
    while IFS= read -r old_file; do
        [[ -n "${old_file}" ]] && rm -f "${old_file}"
    done <<<"${removable}"
}

#==============================================================================
# 主要功能函数
#==============================================================================

# 安装面板
install_dashboard() {
    print_info "开始安装 Better Monitor Dashboard..."
    echo ""

    load_env_file

    # 检查环境
    check_root
    check_docker
    check_port

    # 检查是否已安装
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_warning "检测到已存在的 Better Monitor 容器"
        read -p "是否删除旧容器并重新安装？(y/n): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            print_info "停止并删除旧容器..."
            docker stop "${CONTAINER_NAME}" 2>/dev/null || true
            docker rm "${CONTAINER_NAME}" 2>/dev/null || true
        else
            print_info "安装已取消"
            exit 0
        fi
    fi

    # 创建目录
    create_directories

    # 生成 JWT Secret
    local jwt_secret="${JWT_SECRET:-$(generate_jwt_secret)}"
    JWT_SECRET="$jwt_secret"

    # 创建配置文件
    create_docker_compose "${jwt_secret}"
    create_env_file "${jwt_secret}"

    # 拉取镜像
    print_info "拉取 Docker 镜像..."
    docker pull "${DOCKER_IMAGE}"

    # 启动容器
    print_info "启动 Better Monitor 容器..."
    if ensure_compose; then
        run_compose_cmd up -d
    else
        # 使用 docker run 方式
        docker run -d \
            --name "${CONTAINER_NAME}" \
            --restart unless-stopped \
            -p "${PORT}:3333" \
            -v "${DATA_DIR}:/app/data:rw" \
            -v "${LOGS_DIR}:/app/logs:rw" \
            -v /var/run/docker.sock:/var/run/docker.sock:ro \
            -e TZ="${TZ}" \
            -e JWT_SECRET="${jwt_secret}" \
            -e VERSION=latest \
            --security-opt no-new-privileges:true \
            "${DOCKER_IMAGE}"
    fi

    # 等待服务启动
    print_info "等待服务启动..."
    sleep 5

    # 检查容器状态
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_success "Better Monitor 安装成功！"
        echo ""
        echo "=========================================="
        echo "  访问地址: http://$(hostname -I | awk '{print $1}'):${PORT}"
        echo "  默认账号: admin"
        echo "  默认密码: admin123"
        echo "=========================================="
        echo ""
        echo "重要提示:"
        echo "  1. 请立即登录并修改默认密码"
        echo "  2. JWT Secret 已保存在: ${ENV_FILE}"
        echo "  3. 数据目录: ${DATA_DIR}"
        echo "  4. 日志目录: ${LOGS_DIR}"
        echo ""
        echo "常用命令:"
        echo "  查看日志: docker logs -f ${CONTAINER_NAME}"
        echo "  重启服务: docker restart ${CONTAINER_NAME}"
        echo "  停止服务: docker stop ${CONTAINER_NAME}"
        echo ""
    else
        print_error "容器启动失败，请查看日志"
        docker logs ${CONTAINER_NAME}
        exit 1
    fi
}

# 升级面板
upgrade_dashboard() {
    print_info "开始升级 Better Monitor Dashboard..."
    echo ""

    load_env_file

    check_root
    check_docker

    # 检查容器是否存在
    if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_error "未找到 Better Monitor 容器，请先安装"
        exit 1
    fi

    # 备份数据
    print_info "备份当前数据..."
    backup_data "upgrade"

    # 拉取最新镜像
    print_info "拉取最新镜像..."
    docker pull "${DOCKER_IMAGE}"

    # 停止并删除旧容器
    print_info "停止旧容器..."
    docker stop "${CONTAINER_NAME}"
    docker rm "${CONTAINER_NAME}"

    # 启动新容器
    print_info "启动新容器..."
    local jwt_secret="${JWT_SECRET:-$(generate_jwt_secret)}"
    JWT_SECRET="$jwt_secret"

    if detect_compose_cmd && [ -f "${COMPOSE_FILE}" ]; then
        run_compose_cmd up -d
    else
        docker run -d \
            --name "${CONTAINER_NAME}" \
            --restart unless-stopped \
            -p "${PORT}:3333" \
            -v "${DATA_DIR}:/app/data:rw" \
            -v "${LOGS_DIR}:/app/logs:rw" \
            -v /var/run/docker.sock:/var/run/docker.sock:ro \
            -e TZ="${TZ}" \
            -e JWT_SECRET="${jwt_secret}" \
            -e VERSION=latest \
            --security-opt no-new-privileges:true \
            "${DOCKER_IMAGE}"
    fi

    # 等待服务启动
    print_info "等待服务启动..."
    sleep 5

    # 检查容器状态
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_success "Better Monitor 升级成功！"
        echo ""
        echo "访问地址: http://$(hostname -I | awk '{print $1}'):${PORT}"
    else
        print_error "容器启动失败，正在回滚..."
        # 这里可以添加回滚逻辑
        docker logs ${CONTAINER_NAME}
        exit 1
    fi
}

# 卸载面板
uninstall_dashboard() {
    print_warning "即将卸载 Better Monitor Dashboard"
    echo ""

    load_env_file
    echo "此操作将："
    echo "  1. 停止并删除 Docker 容器"
    echo "  2. 删除 Docker 镜像"
    echo "  3. 可选择是否删除数据文件"
    echo ""

    read -p "确认要卸载吗？(yes/no): " -r
    if [[ ! $REPLY == "yes" ]]; then
        print_info "已取消卸载"
        exit 0
    fi

    check_root

    # 停止并删除容器
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_info "停止并删除容器..."
        docker stop "${CONTAINER_NAME}" 2>/dev/null || true
        docker rm "${CONTAINER_NAME}" 2>/dev/null || true
        print_success "容器已删除"
    fi

    # 删除镜像
    read -p "是否删除 Docker 镜像？(y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        print_info "删除 Docker 镜像..."
        docker rmi "${DOCKER_IMAGE}" 2>/dev/null || true
        print_success "镜像已删除"
    fi

    # 询问是否删除数据
    echo ""
    read -p "是否删除所有数据文件（包括数据库、日志、配置）？(yes/no): " -r
    if [[ $REPLY == "yes" ]]; then
        # 先备份
        print_info "在删除前创建最后一次备份..."
        backup_data "uninstall"

        print_info "删除数据文件..."
        rm -rf "${INSTALL_DIR}"
        print_success "数据文件已删除"
        echo ""
        print_info "备份文件保存在: ${BACKUP_DIR}"
    else
        print_info "数据文件已保留在: ${INSTALL_DIR}"
        echo "如需完全删除，请手动执行: rm -rf ${INSTALL_DIR}"
    fi

    echo ""
    print_success "Better Monitor 卸载完成"
}

# 备份数据
backup_data() {
    local backup_type="${1:-manual}"
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${BACKUP_DIR}/better-monitor-backup-${backup_type}-${timestamp}.tar.gz"

    load_env_file
    print_info "创建数据备份..."

    # 确保备份目录存在
    mkdir -p "${BACKUP_DIR}"

    # 创建备份
    if [ -d "${DATA_DIR}" ] || [ -d "${LOGS_DIR}" ]; then
        tar -czf "${backup_file}" \
            -C "${INSTALL_DIR}" \
            $([ -d "${DATA_DIR}" ] && echo "data") \
            $([ -d "${LOGS_DIR}" ] && echo "logs") \
            $([ -f "${INSTALL_DIR}/.env" ] && echo ".env") \
            $([ -f "${INSTALL_DIR}/docker-compose.yml" ] && echo "docker-compose.yml") \
            2>/dev/null || true

        if [ -f "${backup_file}" ]; then
            print_success "备份创建成功: ${backup_file}"
            cleanup_old_backups
        else
            print_warning "备份创建失败"
        fi
    else
        print_warning "没有找到需要备份的数据"
    fi
}

# 恢复数据
restore_data() {
    load_env_file
    print_info "可用的备份文件："
    echo ""

    # 列出所有备份
    if ! compgen -G "${BACKUP_DIR}/better-monitor-backup-*.tar.gz" >/dev/null; then
        print_error "没有找到备份文件"
        exit 1
    fi

    mapfile -t backups < <(ls -1t "${BACKUP_DIR}"/better-monitor-backup-*.tar.gz)

    # 显示备份列表
    for i in "${!backups[@]}"; do
        local file="${backups[$i]}"
        local size
        size=$(du -h "${file}" | cut -f1)
        local date
        date=$(stat -c %y "${file}" | cut -d' ' -f1,2 | cut -d'.' -f1)
        echo "  [$((i+1))] $(basename "${file}") - ${size} - ${date}"
    done

    echo ""
    read -p "请选择要恢复的备份 (1-${#backups[@]}): " -r

    if [[ ! $REPLY =~ ^[0-9]+$ ]] || [ $REPLY -lt 1 ] || [ $REPLY -gt ${#backups[@]} ]; then
        print_error "无效的选择"
        exit 1
    fi

    local selected_backup="${backups[$((REPLY-1))]}"

    print_warning "恢复数据将覆盖当前数据"
    read -p "确认要恢复吗？(yes/no): " -r
    if [[ ! $REPLY == "yes" ]]; then
        print_info "已取消恢复"
        exit 0
    fi

    check_root

    # 停止容器
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_info "停止容器..."
        docker stop "${CONTAINER_NAME}"
    fi

    # 备份当前数据
    print_info "备份当前数据..."
    backup_data "before-restore"

    # 恢复数据
    print_info "恢复数据..."
    tar -xzf "${selected_backup}" -C "${INSTALL_DIR}"

    # 启动容器
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        print_info "启动容器..."
        docker start "${CONTAINER_NAME}"
    fi

    print_success "数据恢复完成"
}

# 迁移数据
migrate_data() {
    load_env_file
    print_info "Better Monitor 数据迁移工具"
    echo ""
    echo "此工具用于将数据迁移到新服务器"
    echo ""
    echo "迁移步骤："
    echo "  1. 在源服务器上创建数据备份"
    echo "  2. 将备份文件传输到目标服务器"
    echo "  3. 在目标服务器上恢复数据"
    echo ""

    read -p "请选择操作 [1.创建迁移包 2.导入迁移包]: " -n 1 -r
    echo

    case $REPLY in
        1)
            # 创建迁移包
            local timestamp=$(date +%Y%m%d_%H%M%S)
            local migration_file="${BACKUP_DIR}/better-monitor-migration-${timestamp}.tar.gz"

            print_info "创建迁移包..."

            # 停止容器以确保数据一致性
            local need_restart=false
            if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
                print_info "停止容器以确保数据一致性..."
                docker stop "${CONTAINER_NAME}"
                need_restart=true
            fi

            # 创建迁移包
            tar -czf "${migration_file}" \
                -C "${INSTALL_DIR}" \
                data logs .env docker-compose.yml 2>/dev/null || true

            # 重启容器
            if [ "${need_restart}" = true ]; then
                print_info "重启容器..."
                docker start "${CONTAINER_NAME}"
            fi

            if [ -f "${migration_file}" ]; then
                print_success "迁移包创建成功: ${migration_file}"
                echo ""
                echo "请将此文件传输到目标服务器，然后运行："
                echo "  sudo bash install-dashboard.sh"
                echo "  选择 '4. 迁移面板数据' -> '2. 导入迁移包'"
            else
                print_error "迁移包创建失败"
            fi
            ;;
        2)
            # 导入迁移包
            read -p "请输入迁移包的完整路径: " -r migration_file

            if [ ! -f "$migration_file" ]; then
                print_error "文件不存在: $migration_file"
                exit 1
            fi

            print_warning "导入迁移包将覆盖当前数据"
            read -p "确认要导入吗？(yes/no): " -r
            if [[ ! $REPLY == "yes" ]]; then
                print_info "已取消导入"
                exit 0
            fi

            check_root

            # 停止容器
            if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
                print_info "停止容器..."
                docker stop "${CONTAINER_NAME}"
            fi

            # 备份当前数据
            if [ -d "${DATA_DIR}" ]; then
                print_info "备份当前数据..."
                backup_data "before-migration"
            fi

            # 创建目录
            create_directories

            # 解压迁移包
            print_info "导入数据..."
            tar -xzf "$migration_file" -C "${INSTALL_DIR}"
            load_env_file

            # 如果容器不存在，则安装
            if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
                print_info "容器不存在，开始安装..."

                # 拉取镜像
                docker pull "${DOCKER_IMAGE}"
                local jwt_secret="${JWT_SECRET:-$(generate_jwt_secret)}"
                JWT_SECRET="$jwt_secret"

                # 启动容器
                if detect_compose_cmd && [ -f "${COMPOSE_FILE}" ]; then
                    run_compose_cmd up -d
                else
                    docker run -d \
                        --name "${CONTAINER_NAME}" \
                        --restart unless-stopped \
                        -p "${PORT}:3333" \
                        -v "${DATA_DIR}:/app/data:rw" \
                        -v "${LOGS_DIR}:/app/logs:rw" \
                        -v /var/run/docker.sock:/var/run/docker.sock:ro \
                        -e TZ="${TZ}" \
                        -e JWT_SECRET="${jwt_secret}" \
                        -e VERSION=latest \
                        --security-opt no-new-privileges:true \
                        "${DOCKER_IMAGE}"
                fi
            else
                # 启动现有容器
                print_info "启动容器..."
                docker start "${CONTAINER_NAME}"
            fi

            print_success "数据迁移完成"
            echo ""
            echo "访问地址: http://$(hostname -I | awk '{print $1}'):${PORT}"
            ;;
        *)
            print_error "无效的选择"
            exit 1
            ;;
    esac
}

# 查看状态
show_status() {
    load_env_file
    print_info "Better Monitor 状态信息"
    echo ""

    # 容器状态
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        echo "容器状态:"
        docker ps -a --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
        echo ""

        # 资源使用
        echo "资源使用:"
        docker stats "${CONTAINER_NAME}" --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}"
        echo ""

        # 磁盘使用
        if [ -d "${DATA_DIR}" ]; then
            echo "磁盘使用:"
            echo "  数据目录: $(du -sh ${DATA_DIR} 2>/dev/null | cut -f1)"
            echo "  日志目录: $(du -sh ${LOGS_DIR} 2>/dev/null | cut -f1)"
            echo "  备份目录: $(du -sh ${BACKUP_DIR} 2>/dev/null | cut -f1)"
            echo ""
        fi

        # 访问信息
        echo "访问信息:"
        echo "  地址: http://$(hostname -I | awk '{print $1}'):${PORT}"
        echo "  账号: admin"
        echo "  密码: admin123 (首次登录请修改)"
        echo ""

        # 日志
        echo "最近日志:"
        docker logs "${CONTAINER_NAME}" --tail 10
    else
        print_warning "Better Monitor 未安装或容器不存在"
    fi
}

# 显示菜单
show_menu() {
    clear
    echo "=========================================="
    echo "  Better Monitor Dashboard 管理脚本"
    echo "=========================================="
    echo ""
    echo "1. 安装面板"
    echo "2. 升级面板"
    echo "3. 卸载面板"
    echo "4. 迁移面板数据"
    echo "5. 备份数据"
    echo "6. 恢复数据"
    echo "7. 查看状态"
    echo "0. 退出"
    echo ""
    read -p "请选择操作 [0-7]: " -n 1 -r
    echo
}

#==============================================================================
# 主程序
#==============================================================================

main() {
    load_env_file

    # 如果有参数，直接执行对应功能
    if [ $# -gt 0 ]; then
        case $1 in
            install)
                install_dashboard
                ;;
            upgrade)
                upgrade_dashboard
                ;;
            uninstall)
                uninstall_dashboard
                ;;
            migrate)
                migrate_data
                ;;
            backup)
                backup_data "manual"
                ;;
            restore)
                restore_data
                ;;
            status)
                show_status
                ;;
            *)
                echo "用法: $0 {install|upgrade|uninstall|migrate|backup|restore|status}"
                exit 1
                ;;
        esac
        exit 0
    fi

    # 交互式菜单
    while true; do
        load_env_file
        show_menu
        case $REPLY in
            1)
                install_dashboard
                read -p "按回车键继续..."
                ;;
            2)
                upgrade_dashboard
                read -p "按回车键继续..."
                ;;
            3)
                uninstall_dashboard
                read -p "按回车键继续..."
                ;;
            4)
                migrate_data
                read -p "按回车键继续..."
                ;;
            5)
                backup_data "manual"
                read -p "按回车键继续..."
                ;;
            6)
                restore_data
                read -p "按回车键继续..."
                ;;
            7)
                show_status
                read -p "按回车键继续..."
                ;;
            0)
                print_info "感谢使用 Better Monitor!"
                exit 0
                ;;
            *)
                print_error "无效的选择"
                sleep 2
                ;;
        esac
    done
}

# 运行主程序
main "$@"
