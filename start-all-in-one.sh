#!/usr/bin/env bash
set -euo pipefail

# 颜色定义
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
RED="\033[0;31m"
NC="\033[0m" # 恢复默认颜色

echo -e "${YELLOW}[INFO]${NC} 检测到 Docker Compose: $(docker compose version | head -n 1)"

echo -e "${YELLOW}[INFO]${NC} 创建必要的目录..."
mkdir -p data logs

export BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/scripts/prepare-all-in-one-build.sh"

echo -e "${YELLOW}[INFO]${NC} 停止旧服务（如果有）..."
docker compose -f docker-compose.all-in-one.yml down 2>/dev/null

echo -e "${YELLOW}[INFO]${NC} 启动 Better Monitor 服务（单容器版）..."
echo -e "${YELLOW}[INFO]${NC} 执行: docker compose -f docker-compose.all-in-one.yml up -d"
if docker compose -f docker-compose.all-in-one.yml up -d --build; then
    echo -e "${GREEN}[SUCCESS]${NC} 服务启动成功！"
    echo -e "${YELLOW}[INFO]${NC} Better Monitor访问地址: http://localhost:3333"
    echo -e "${YELLOW}[INFO]${NC} 请配置反向代理将外部流量代理到3333端口"
else
    echo -e "${RED}[ERROR]${NC} 服务启动失败，请检查错误信息"
    exit 1
fi
