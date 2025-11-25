# 服务器运维系统 - Agent1111

## 项目简介

本项目是基于Go开发的服务器应急响应与运维系统的Agent部分，负责收集服务器信息、执行管理命令等功能。

## 功能特点

- 服务器系统信息收集
- 资源使用监控（CPU、内存、磁盘、网络）
- 自动心跳与状态上报
- 远程命令执行
- 远程文件管理
- Docker容器管理
- Nginx配置管理

## 支持平台

- Windows
- Linux
- macOS

支持的CPU架构:
- x86_64
- ARM64

## 安装

### 二进制安装

从 [Releases](https://github.com/yourusername/server-ops/releases) 页面下载适合你系统的二进制文件，解压后直接运行。

### 使用源码编译

```bash
git clone https://github.com/yourusername/server-ops.git
cd server-ops/agent
go build -o server-ops-agent ./cmd/agent
```

## 配置

首次运行时，Agent会在当前目录下创建`config/agent.yaml`配置文件，你可以手动修改此文件进行配置，也可以通过面板端进行配置。

```yaml
# 服务器连接设置
server_url: "http://localhost:8080"
server_id: 0  # 0表示未注册
secret_key: "" # 空表示未注册

# 时间间隔设置
heartbeat_interval: "10s"
monitor_interval: "30s"

# 日志设置
log_level: "info" # debug, info, warn, error, fatal
log_file: "./logs/agent.log"

# 监控设置
enable_cpu_monitor: true
enable_mem_monitor: true
enable_disk_monitor: true
enable_network_monitor: true
```

## 使用

### 基本使用

```bash
# 使用默认配置启动
./server-ops-agent

# 指定配置文件启动
./server-ops-agent -c /path/to/config.yaml

# 显示版本信息
./server-ops-agent version
```

### 注册到面板

首次运行时，Agent会进入初始化模式，需要在面板端添加服务器，然后使用面板生成的注册命令进行注册。

## 开发

### 依赖

- Go 1.16+

### 编译

```bash
# 编译当前平台
go build -o server-ops-agent ./cmd/agent

# 交叉编译
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o server-ops-agent-linux-amd64 ./cmd/agent

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o server-ops-agent-windows-amd64.exe ./cmd/agent

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -o server-ops-agent-darwin-amd64 ./cmd/agent

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o server-ops-agent-linux-arm64 ./cmd/agent
``` 