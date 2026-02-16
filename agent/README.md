# Better Monitor Agent

<p align="center">
  <b>轻量级跨平台服务器监控与运维 Agent</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/language-Go-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows%20%7C%20Android-brightgreen?style=flat-square" alt="Platform">
  <img src="https://img.shields.io/badge/arch-x86__64%20%7C%20ARM64-blue?style=flat-square" alt="Arch">
</p>

<p align="center">
  <a href="https://github.com/EnderKC/BetterMonitor/stargazers"><img src="https://img.shields.io/github/stars/EnderKC/BetterMonitor?style=social" alt="Stars"></a>
  <a href="https://github.com/EnderKC/BetterMonitor/network/members"><img src="https://img.shields.io/github/forks/EnderKC/BetterMonitor?style=social" alt="Forks"></a>
  <a href="https://github.com/EnderKC/BetterMonitor/releases"><img src="https://img.shields.io/github/v/release/EnderKC/BetterMonitor?style=flat-square&color=orange" alt="Release"></a>
  <a href="https://github.com/EnderKC/BetterMonitor/releases"><img src="https://img.shields.io/github/downloads/EnderKC/BetterMonitor/total?style=flat-square&color=blueviolet" alt="Downloads"></a>
  <a href="https://github.com/EnderKC/BetterMonitor/blob/main/LICENSE"><img src="https://img.shields.io/github/license/EnderKC/BetterMonitor?style=flat-square" alt="License"></a>
</p>

---

## 功能特点

| 功能 | 说明 |
|------|------|
| 系统信息采集 | 自动收集主机名、OS、内核、IP 等基础信息 |
| 资源监控 | CPU、内存、磁盘、网络实时监控与上报 |
| 心跳保活 | 自动心跳，面板实时感知 Agent 在线状态 |
| 远程命令 | 通过面板下发 Shell 命令并回传结果 |
| 文件管理 | 远程浏览、上传、下载服务器文件 |
| Docker 管理 | 容器列表、启停、日志查看 |
| Nginx 管理 | 配置查看与管理 |

## 快速开始

### 一键安装（推荐）

在 Dashboard 面板中添加服务器后，使用面板生成的安装命令即可完成部署：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/main/install-agent.sh \
  | bash -s -- --server-id <ID> --secret-key <KEY> --server <DASHBOARD_URL>
```

### 手动安装

从 [Releases](https://github.com/EnderKC/BetterMonitor/releases) 页面下载对应平台的二进制文件，解压后直接运行。

### 源码编译

```bash
git clone https://github.com/EnderKC/BetterMonitor.git
cd BetterMonitor/agent
go build -o better-monitor-agent ./cmd/agent
```

## 配置

Agent 首次运行时会自动生成 `agent.yaml` 配置文件，也可以通过面板端进行配置。

```yaml
# 服务器连接
server_url: "http://localhost:8080"
server_id: 0          # 0 = 未注册
secret_key: ""        # 空 = 未注册

# 间隔设置
heartbeat_interval: "10s"
monitor_interval: "30s"

# 日志
log_level: "info"     # debug | info | warn | error | fatal
log_file: "./logs/agent.log"

# 监控开关
enable_cpu_monitor: true
enable_mem_monitor: true
enable_disk_monitor: true
enable_network_monitor: true
```

## 使用

```bash
./better-monitor-agent                    # 默认配置启动
./better-monitor-agent -c /path/to.yaml   # 指定配置文件
./better-monitor-agent version            # 查看版本
```

## 日志查看

通过一键安装脚本部署的 Agent，日志路径如下：

### Linux (systemd)

```bash
# 应用日志
sudo tail -f /var/log/better-monitor/agent.log

# 或通过 journalctl
sudo journalctl -u better-monitor-agent -f
```

### macOS (launchd)

```bash
# 应用日志
sudo tail -f /var/log/better-monitor/agent.log

# stdout / stderr（launchd 托管输出）
sudo tail -f /var/log/better-monitor/agent.stdout.log
sudo tail -f /var/log/better-monitor/agent.stderr.log
```

### Android Termux

```bash
tail -f "${PREFIX}/opt/better-monitor/logs/agent.log"
```

### Android Magisk

```bash
su -c 'tail -f /data/adb/better-monitor/logs/agent.log'
```

## 开发

### 环境要求

- Go 1.16+

### 编译

```bash
# 当前平台
go build -o better-monitor-agent ./cmd/agent

# 交叉编译
GOOS=linux   GOARCH=amd64 go build -o better-monitor-agent-linux-amd64       ./cmd/agent
GOOS=linux   GOARCH=arm64 go build -o better-monitor-agent-linux-arm64       ./cmd/agent
GOOS=darwin  GOARCH=amd64 go build -o better-monitor-agent-darwin-amd64      ./cmd/agent
GOOS=darwin  GOARCH=arm64 go build -o better-monitor-agent-darwin-arm64      ./cmd/agent
GOOS=windows GOARCH=amd64 go build -o better-monitor-agent-windows-amd64.exe ./cmd/agent
```

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

<p align="center">
  如果觉得项目不错，欢迎 <a href="https://github.com/EnderKC/BetterMonitor">Star</a> 支持一下 :)
</p>
