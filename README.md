<div align="center">

# Better Monitor

**轻量级跨平台服务器监控与运维管理平台**

Dashboard 1.2.2 / Agent 1.2.2

[![License](https://img.shields.io/github/license/EnderKC/BetterMonitor?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vue.js&logoColor=white)](https://vuejs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![Docker](https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white)](https://docker.com/)

[![GitHub Stars](https://img.shields.io/github/stars/EnderKC/BetterMonitor?style=social)](https://github.com/EnderKC/BetterMonitor/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/EnderKC/BetterMonitor?style=social)](https://github.com/EnderKC/BetterMonitor/network/members)
[![GitHub Release](https://img.shields.io/github/v/release/EnderKC/BetterMonitor?style=flat-square&color=orange)](https://github.com/EnderKC/BetterMonitor/releases)
[![GitHub Downloads](https://img.shields.io/github/downloads/EnderKC/BetterMonitor/total?style=flat-square&color=blueviolet)](https://github.com/EnderKC/BetterMonitor/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/enderhkc/better-monitor?style=flat-square&color=2496ED)](https://hub.docker.com/r/enderhkc/better-monitor)

**技术栈** : Vue 3 + TypeScript + Vite | Go 1.24 + Gin + GORM | SQLite / MySQL

[快速入门](#-快速入门) · [部署方式](#-部署方式) · [Agent 安装](#-agent-安装) · [环境变量](#-环境变量)

</div>

---

## 功能特性

<table>
<tr>
<td width="50%">

### 监控与运维

- **实时监控** — CPU / 内存 / 磁盘 / 网络流量实时采集，历史趋势分析，可配置数据保留策略
- **Web 终端** — 浏览器内 SSH 终端，支持多会话管理
- **文件管理** — 在线浏览、编辑、上传、下载，支持拖拽操作
- **进程管理** — 实时进程列表、资源占用监控

</td>
<td width="50%">

### 服务与管理

- **Docker 管理** — 容器 / 镜像 / Compose 编排，容器日志查看与文件管理
- **Nginx 管理** — 配置在线编辑与验证、虚拟主机管理、网站创建
- **SSL 证书** — Let's Encrypt 自动申请与续期
- **自动升级** — Dashboard 下发指令，Agent 自动从 GitHub Releases 拉取新版本

</td>
</tr>
<tr>
<td>

### Agent 模式

- **full** — 完整功能版：监控 + 远程管理
- **monitor** — 只读监控版：仅采集数据

</td>
<td>

### 扩展能力

- **LifeProbe 集成** — 心率、步数、睡眠、专注状态监控
- **多平台支持** — Linux / macOS / Windows / Android

</td>
</tr>
</table>

---

## 🚀 快速入门

> 前置条件：Linux 系统，已安装 Docker

**一键安装 Dashboard：**

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-dashboard.sh | sudo bash
```

安装完成后访问 `http://your-server-ip:3333`，默认账号 `admin` / `admin123`。

> ⚠️ 首次登录后请立即修改默认密码。

<details>
<summary><b>📦 安装脚本支持的操作</b></summary>

```bash
sudo ./install-dashboard.sh install    # 安装
sudo ./install-dashboard.sh upgrade    # 升级
sudo ./install-dashboard.sh backup     # 备份
sudo ./install-dashboard.sh restore    # 恢复
sudo ./install-dashboard.sh status     # 查看状态
sudo ./install-dashboard.sh migrate    # 数据迁移
```

</details>

---

## 📦 部署方式

### Docker Compose（推荐）

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

<details>
<summary><b>从源码构建</b></summary>

```bash
git clone https://github.com/EnderKC/BetterMonitor.git
cd BetterMonitor
docker-compose -f docker-compose.all-in-one.yml up -d --build
```

</details>

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

<details>
<summary><b>🔧 手动部署（不使用 Docker）</b></summary>

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

将前端构建产物部署到 Nginx 等静态文件服务器，配置反向代理指向后端。后端启动前配置 `.env` 文件（参考[环境变量](#-环境变量)）：

```bash
./better-monitor-backend
```

也可从 [Releases](https://github.com/EnderKC/BetterMonitor/releases) 直接下载预编译二进制文件。

</details>

---

## 🖥️ Agent 安装

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

<details>
<summary><b>Termux（无需 root）</b></summary>

```bash
pkg update && pkg install -y curl python

curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --android-mode termux --server-id <ID> --secret-key "<KEY>" --server "https://your-dashboard-url"
```

可选安装 `termux-services` 实现后台常驻，配合 Termux:Boot 实现开机自启。

</details>

<details>
<summary><b>Root（Magisk）</b></summary>

```bash
pkg install -y curl python tsu
tsu
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --android-mode root --server-id <ID> --secret-key "<KEY>" --server "https://your-dashboard-url"
```

</details>

---

## ⚙️ 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `JWT_SECRET` | JWT 签名密钥，生产环境务必修改 | — |
| `DB_PATH` | SQLite 数据库路径 | `./data/data.db` |
| `PORT` | 后端监听端口 | `8085` |
| `TZ` | 时区 | `Asia/Shanghai` |
| `GITHUB_TOKEN` | GitHub Personal Access Token，用于提升 API 请求限额（详见下方说明） | — |
| `AGENT_RELEASE_GITHUB_TOKEN` | 同上，优先级高于 `GITHUB_TOKEN`，适用于需要区分用途的场景 | — |

### GitHub Token 配置说明

Dashboard 会通过 GitHub API 检查 Agent 的最新版本，以支持自动升级功能。GitHub API 对**未认证请求**有严格的频率限制：

| 认证方式 | 限额 | 限制维度 |
|----------|------|----------|
| 未认证 | **60 次 / 小时** | 按 IP 地址 |
| 携带 Token | **5,000 次 / 小时** | 按 Token 所属用户 |

未认证时，同一 IP 下的所有请求共享 60 次额度。如果服务器处于共享 IP 环境（如部分云服务商的 NAT 出口），其他用户的请求也会占用你的额度。当额度耗尽时，API 会返回 `403` 错误，导致版本检查失败。

**建议配置 GitHub Token 以避免此问题。**

#### 生成 Token 步骤

1. 登录你自己的 GitHub 账号（无需仓库所有者的 Token，任意可正常调用 GitHub API 的个人账号均可）
2. 进入 [Settings → Developer settings → Personal access tokens](https://github.com/settings/tokens)
3. 选择 **Tokens (classic)** 或 **Fine-grained tokens** 均可
4. 点击 **Generate new token**，填写名称（如 `better-monitor`），**不需要勾选任何权限 scope**（发行版信息属于公开数据）
5. 设置过期时间（建议设置合理的过期时间并定期轮换）
6. 点击 **Generate token**，复制生成的 Token

#### 配置方式

**脚本安装（install-dashboard.sh）** — 编辑安装目录下的 `docker-compose.yml`（默认路径 `/opt/better-monitor/docker-compose.yml`），在 `environment` 段中添加一行：

```yaml
    environment:
      - TZ=Asia/Shanghai
      - JWT_SECRET=xxx
      - VERSION=latest
      - GITHUB_TOKEN=your_github_token_here   # ← 添加此行
```

保存后重启容器：

```bash
cd /opt/better-monitor && docker-compose down && docker-compose up -d
```

**Docker Compose（手动编排）** — 同上，在 `docker-compose.yml` 的 `environment` 中添加：

```yaml
environment:
  - GITHUB_TOKEN=your_github_token_here
```

**Docker Run** — 添加 `-e` 参数：

```bash
docker run -d \
  -e GITHUB_TOKEN=your_github_token_here \
  ...
```

**手动部署（非 Docker）** — 在 `.env` 文件中添加：

```env
GITHUB_TOKEN=your_github_token_here
```

> **注意：**
> - 如果同时设置了 `AGENT_RELEASE_GITHUB_TOKEN` 和 `GITHUB_TOKEN`，系统会优先使用 `AGENT_RELEASE_GITHUB_TOKEN`。两个变量功能完全相同，提供两个变量名是为了避免与其他使用 `GITHUB_TOKEN` 的工具产生冲突。
> - 配置 Token 后需要**重启服务或容器**才会生效。
> - 请勿将真实 Token 提交到公开仓库，建议通过环境变量注入或 CI/CD Secrets 管理。

---

## Star History

<a href="https://star-history.com/#EnderKC/BetterMonitor&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=EnderKC/BetterMonitor&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=EnderKC/BetterMonitor&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=EnderKC/BetterMonitor&type=Date" width="600" />
  </picture>
</a>

## Contributors

<a href="https://github.com/EnderKC/BetterMonitor/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=EnderKC/BetterMonitor" alt="Contributors" />
</a>

---

<div align="center">

**[MIT License](LICENSE)**

如果觉得项目不错，欢迎 Star 支持一下 :)

</div>
