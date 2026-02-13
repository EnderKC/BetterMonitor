# Better Monitor

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3-green.svg)](https://vuejs.org/)
[![Docker](https://img.shields.io/badge/Docker-Supported-blue.svg)](https://docker.com/)

轻量级服务器监控与运维管理平台。Dashboard 1.2.0 / Agent 1.2.0

**技术栈**: Vue 3 + TypeScript + Vite | Go 1.24 + Gin + GORM | SQLite/MySQL

## 目录

- [功能特性](#功能特性)
- [快速入门](#快速入门)
- [部署方式](#部署方式)
  - [Docker Compose](#docker-compose)
  - [Docker Run](#docker-run)
  - [手动部署](#手动部署)
- [Agent 安装](#agent-安装)
  - [Linux / macOS](#linux--macos)
  - [Windows](#windows)
  - [Android](#android)
- [环境变量](#环境变量)
- [许可证](#许可证)

## 功能特性

| 分类 | 功能 |
|------|------|
| 实时监控 | CPU / 内存 / 磁盘 / 网络流量实时采集，历史趋势分析，可配置数据保留策略 |
| Web 终端 | 浏览器内 SSH 终端，支持多会话管理 |
| 文件管理 | 在线浏览、编辑、上传、下载，支持拖拽操作 |
| 进程管理 | 实时进程列表、资源占用监控 |
| Docker 管理 | 容器 / 镜像 / Compose 编排，容器日志查看与文件管理 |
| Nginx 管理 | 配置在线编辑与验证、虚拟主机管理、网站创建 |
| SSL 证书 | Let's Encrypt 自动申请与续期 |
| Agent 类型 | 支持 full（完整功能）和 monitor（只读监控）两种模式 |
| 自动升级 | Dashboard 下发指令，Agent 自动从 GitHub Releases 拉取新版本 |
| 健康数据 | LifeProbe 集成：心率、步数、睡眠、专注状态监控 |

## 快速入门

> 前置条件：Linux 系统，已安装 Docker

一键安装 Dashboard：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-dashboard.sh | sudo bash
```

安装完成后访问 `http://your-server-ip:3333`，默认账号 `admin` / `admin123`。

> 首次登录后请立即修改默认密码。

脚本还支持以下操作：

```bash
sudo ./install-dashboard.sh install    # 安装
sudo ./install-dashboard.sh upgrade    # 升级
sudo ./install-dashboard.sh backup     # 备份
sudo ./install-dashboard.sh restore    # 恢复
sudo ./install-dashboard.sh status     # 查看状态
sudo ./install-dashboard.sh migrate    # 数据迁移
```

## 部署方式

### Docker Compose

```bash
mkdir -p /opt/better-monitor && cd /opt/better-monitor

cat > docker-compose.yml << 'EOF'
version: '3.8'
services:
  better-monitor:
    image: enderhkc/better-monitor:latest
    container_name: better-monitor
    restart: unless-stopped
    ports:
      - "3333:3333"
    volumes:
      - ./data:/app/data:rw
      - ./logs:/app/logs:rw
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - TZ=Asia/Shanghai
      - JWT_SECRET=${JWT_SECRET:-$(openssl rand -base64 32)}
    security_opt:
      - no-new-privileges:true
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3333/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
EOF

docker-compose up -d
```

从源码构建：

```bash
git clone https://github.com/EnderKC/BetterMonitor.git
cd BetterMonitor
docker-compose -f docker-compose.all-in-one.yml up -d --build
```

### Docker Run

```bash
mkdir -p /opt/better-monitor/{data,logs}

docker run -d \
  --name better-monitor \
  --restart unless-stopped \
  -p 3333:3333 \
  -v /opt/better-monitor/data:/app/data:rw \
  -v /opt/better-monitor/logs:/app/logs:rw \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e TZ=Asia/Shanghai \
  -e JWT_SECRET="$(openssl rand -base64 32)" \
  --security-opt no-new-privileges:true \
  enderhkc/better-monitor:latest
```

### 手动部署

适合需要自定义部署或不使用 Docker 的场景。

**1. 构建后端**

```bash
cd backend
go mod tidy
go build -o better-monitor-backend main.go
```

**2. 构建前端**

```bash
cd frontend
npm install
npm run build
```

**3. 构建 Agent**

```bash
cd agent
go mod tidy
go build -o better-monitor-agent cmd/agent/main.go
```

**4. 运行**

将前端构建产物部署到 Nginx 等静态文件服务器，配置反向代理指向后端。后端启动前配置 `.env` 文件（参考[环境变量](#环境变量)）：

```bash
# 启动后端
./better-monitor-backend
```

也可从 [Releases](https://github.com/EnderKC/BetterMonitor/releases) 直接下载预编译二进制文件。

## Agent 安装

登录 Dashboard，在"服务器管理"中添加服务器并获取 `server_id` 和 `secret_key`。

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --server-id <ID> --secret-key "<KEY>" --server "https://your-dashboard-url"
```

安装后路径：

| 项目 | 路径 |
|------|------|
| 二进制 | `/opt/better-monitor/bin/better-monitor-agent` |
| 配置 | `/etc/better-monitor/agent.yaml` |
| 日志 | `/var/log/better-monitor/agent.log` |

脚本会自动注册系统服务（systemd / OpenRC / launchd）。

```bash
# 查看状态
sudo systemctl status better-monitor-agent

# 查看日志
sudo journalctl -u better-monitor-agent -f
```

卸载：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/uninstall-agent.sh | bash
```

### Windows

PowerShell（管理员）：

```powershell
irm https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.ps1 | iex `
  -ServerUrl "https://your-dashboard-url:3333" -ServerId <ID> -SecretKey "<KEY>"
```

或从 Releases 下载 `better-monitor-agent-windows-amd64.zip`，解压后运行：

```powershell
.\better-monitor-agent.exe --server https://your-dashboard-url:3333 --server-id <ID> --secret-key "<KEY>"
```

### Android

**Termux（无需 root）：**

```bash
pkg update && pkg install -y curl python

curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --android-mode termux --server-id <ID> --secret-key "<KEY>" --server "https://your-dashboard-url"
```

可选安装 `termux-services` 实现后台常驻，配合 Termux:Boot 实现开机自启。

**Root（Magisk）：**

```bash
pkg install -y curl python tsu
tsu
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --android-mode root --server-id <ID> --secret-key "<KEY>" --server "https://your-dashboard-url"
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `JWT_SECRET` | JWT 签名密钥，生产环境务必修改 | - |
| `DB_PATH` | SQLite 数据库路径 | `./data/data.db` |
| `PORT` | 后端监听端口 | `8085` |
| `TZ` | 时区 | `Asia/Shanghai` |

## 许可证

[MIT License](LICENSE)
