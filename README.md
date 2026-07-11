<div align="center">

# Better Monitor

跨平台服务器监控与运维管理平台

Dashboard **1.2.6** · Agent **1.2.6**

[![CI](https://github.com/EnderKC/BetterMonitor/actions/workflows/ci.yml/badge.svg)](https://github.com/EnderKC/BetterMonitor/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/EnderKC/BetterMonitor?style=flat-square)](https://github.com/EnderKC/BetterMonitor/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/enderhkc/better-monitor?style=flat-square)](https://hub.docker.com/r/enderhkc/better-monitor)
[![License](https://img.shields.io/github/license/EnderKC/BetterMonitor?style=flat-square)](LICENSE)

[快速开始](#快速开始) · [Agent 安装](#agent-安装) · [升级与维护](#升级与维护) · [源码开发](#源码开发) · [配置说明](#配置说明)

</div>

## 项目简介

Better Monitor 由 Dashboard 和 Agent 两部分组成：

- **Dashboard**：提供监控展示、服务器管理、告警、Docker、Nginx、网站和证书管理。
- **Agent**：部署在被管理节点，负责采集数据并执行经过认证的远程操作。

项目采用单管理员模式，不包含多用户权限体系。Agent 提供两种构建：

| 类型 | 能力 | 建议场景 |
|---|---|---|
| `full` | 监控、终端、文件、进程、Docker、Nginx 等完整功能 | 自有服务器和运维节点 |
| `monitor` | 只读监控数据采集 | 只需要观测、不允许远程管理的节点 |

支持 Linux、Windows、macOS 和 Android。具体可用制品以 [GitHub Releases](https://github.com/EnderKC/BetterMonitor/releases) 为准。

## 主要功能

- CPU、内存、磁盘、网络、进程和历史趋势监控
- 浏览器终端与服务器文件管理
- Docker 容器、镜像、Compose、实时日志和容器文件管理
- Nginx/OpenResty 配置、网站和端口管理
- Let's Encrypt 证书签发、查看和续期
- Agent 在线升级、版本状态和升级任务追踪
- 告警记录、通知渠道和公开探针页面
- LifeProbe 数据接入

## 快速开始

### 前置条件

- 一台 Linux 服务器
- Docker Engine
- Docker Compose v2 或 `docker-compose`
- 对外开放 Dashboard 端口，默认 `3333`

### 安装 Dashboard

推荐先下载并检查脚本，再执行安装：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-dashboard.sh \
  -o /tmp/install-dashboard.sh
sudo bash /tmp/install-dashboard.sh install
```

默认安装目录为 `/opt/better-monitor`。安装脚本会：

- 生成随机 JWT 密钥
- 生成初始管理员密码文件
- 创建数据、日志和备份目录
- 创建并启动 Docker Compose 服务

安装后访问：

```text
http://<Dashboard IP>:3333
```

默认管理员用户名是 `admin`。初始密码保存在：

```bash
sudo cat /opt/better-monitor/data/admin-password
```

首次登录后应立即修改密码，并为 Dashboard 配置 HTTPS 反向代理。

### 查看状态

```bash
sudo bash /tmp/install-dashboard.sh status
docker ps --filter name=better-monitor
docker logs --tail 100 better-monitor
```

## Agent 安装

先在 Dashboard 的服务器管理页面创建服务器，取得 `server_id` 和 `secret_key`。

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | sudo bash -s -- \
      --server "https://monitor.example.com" \
      --server-id <SERVER_ID> \
      --secret-key "<SECRET_KEY>" \
      --agent-type full \
      --channel stable
```

常用选项：

| 参数 | 可选值 | 说明 |
|---|---|---|
| `--agent-type` | `full`, `monitor` | Agent 功能类型 |
| `--channel` | `stable`, `prerelease`, `nightly` | Release 渠道 |
| `--log-level` | `debug`, `info`, `warn`, `error` | 日志级别 |

Linux 默认路径：

| 内容 | 路径 |
|---|---|
| 二进制 | `/opt/better-monitor/bin/better-monitor-agent` |
| 配置 | `/etc/better-monitor/agent.yaml` |
| 日志 | `/var/log/better-monitor/agent.log` |

```bash
sudo systemctl status better-monitor-agent
sudo journalctl -u better-monitor-agent -f
```

卸载：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/uninstall-agent.sh \
  -o /tmp/uninstall-agent.sh
sudo bash /tmp/uninstall-agent.sh
```

### Windows

在管理员 PowerShell 中执行：

```powershell
Invoke-WebRequest `
  -Uri "https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.ps1" `
  -OutFile "$env:TEMP\install-agent.ps1"

& "$env:TEMP\install-agent.ps1" `
  -ServerUrl "https://monitor.example.com" `
  -ServerId <SERVER_ID> `
  -SecretKey "<SECRET_KEY>" `
  -AgentType "full" `
  -Channel "stable"
```

### Android

Termux 模式：

```bash
pkg update
pkg install -y curl

curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- \
      --android-mode termux \
      --server "https://monitor.example.com" \
      --server-id <SERVER_ID> \
      --secret-key "<SECRET_KEY>"
```

Root/Magisk 环境使用 `--android-mode root`，并以 root 用户执行脚本。

## 升级与维护

Dashboard 管理脚本支持：

```bash
sudo bash /tmp/install-dashboard.sh upgrade
sudo bash /tmp/install-dashboard.sh backup
sudo bash /tmp/install-dashboard.sh restore
sudo bash /tmp/install-dashboard.sh migrate
sudo bash /tmp/install-dashboard.sh status
sudo bash /tmp/install-dashboard.sh uninstall
```

建议升级前先备份：

```bash
sudo bash /tmp/install-dashboard.sh backup
sudo bash /tmp/install-dashboard.sh upgrade
```

Agent 可在 Dashboard 中执行升级或类型切换。升级包从 GitHub Releases 下载并校验 SHA256；`stable`、`prerelease` 和 `nightly` 渠道不会相互混用。

## 其他部署方式

### Docker Run

```bash
mkdir -p /opt/better-monitor/{data,logs}
JWT_SECRET="$(openssl rand -hex 32)"
ADMIN_PASSWORD="$(openssl rand -base64 18)"

docker run -d \
  --name better-monitor \
  --restart unless-stopped \
  -p 3333:3333 \
  -v /opt/better-monitor/data:/app/data:rw \
  -v /opt/better-monitor/logs:/app/logs:rw \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e TZ=Asia/Shanghai \
  -e JWT_SECRET="$JWT_SECRET" \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD="$ADMIN_PASSWORD" \
  --security-opt no-new-privileges:true \
  enderhkc/better-monitor:1.2.6

printf 'Initial admin password: %s\n' "$ADMIN_PASSWORD"
```

挂载 Docker socket 会让 Dashboard 具备管理宿主机 Docker 的能力。若不需要 Docker 管理，不要挂载 `/var/run/docker.sock`。

### 从源码构建 all-in-one 镜像

```bash
git clone https://github.com/EnderKC/BetterMonitor.git
cd BetterMonitor
./start-all-in-one.sh
```

脚本会先执行前端构建和当前架构 Backend 构建，再启动 Docker Compose。该流程需要本机已安装 Go、Node.js、npm 和 Docker。

## 配置说明

Dashboard 常用运行时变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8085` | Backend 内部监听端口 |
| `DB_PATH` | `./data/data.db` | SQLite 数据库路径 |
| `JWT_SECRET` | 自动生成或必须显式提供 | JWT 签名密钥 |
| `ADMIN_USERNAME` | `admin` | 单一管理员用户名 |
| `ADMIN_PASSWORD_FILE` | 无 | 初始管理员密码文件 |
| `TZ` | `Asia/Shanghai` | 时区 |
| `GITHUB_TOKEN` | 无 | 提升公开 GitHub API 请求限额 |
| `AGENT_RELEASE_GITHUB_TOKEN` | 无 | Agent Release 专用 Token，优先级更高 |

公开仓库的 Release 查询不要求额外权限。Token 只用于提升 API 限额，不应提交到 Git，建议通过运行时环境变量或 Secret 管理器注入。

配置样例：

- 根目录：[`.env.example`](.env.example)
- Backend：[backend/.env.example](backend/.env.example)
- 发布版本源：[versions.env](versions.env)

`versions.env` 仅保存 Dashboard 和 Agent 的发布版本，不应存放运行时密钥。

## 源码开发

### Backend

```bash
cd backend
go test ./...
go vet ./...
go run .
```

### Agent

```bash
cd agent
go test ./...
go vet ./...
go run ./cmd/agent
```

Monitor-only 构建：

```bash
cd agent
go build -tags monitor_only -o better-monitor-agent-monitor ./cmd/agent
```

### Frontend

```bash
cd frontend
npm ci
npm run check
npm run dev
```

### 项目质量门禁

CI 会执行：

- Backend/Agent 格式、测试、vet 和 race
- Frontend lint、typecheck、Vitest 和生产构建
- Linux/Windows 安装脚本契约
- Release 版本、仓库卫生和 CI/CD 契约

## 项目结构

```text
BetterMonitor/
├── agent/          # 节点 Agent
├── backend/        # Dashboard Backend
├── frontend/       # Vue Dashboard
├── scripts/        # 本地构建与仓库检查脚本
├── tests/          # Shell/发布/安装契约测试
├── testdata/       # 测试夹具
├── doc/            # 专题技术文档
├── reports/        # 项目检查与优化报告
└── versions.env    # Dashboard / Agent 发布版本源
```

## 安全建议

- Dashboard 应通过 HTTPS 暴露，不应直接公开 Backend 内部端口。
- 妥善保存服务器 `secret_key`、管理员密码和 GitHub Token。
- 不要把运行时 `.env`、数据库、证书、日志或备份提交到仓库。
- 只在需要远程 Docker 管理时挂载 Docker socket。
- 为公开部署设置防火墙、备份和日志保留策略。

## 文档与链接

- [GitHub Releases](https://github.com/EnderKC/BetterMonitor/releases)
- [Docker Hub](https://hub.docker.com/r/enderhkc/better-monitor)
- [LifeProbe 技术说明](doc/life-probe-technical-guide.md)
- [当日优化报告](reports/BetterMonitor-2026-07-11.md)

## License

[MIT License](LICENSE)

欢迎通过 Issue 或 Pull Request 反馈问题和改进建议。
