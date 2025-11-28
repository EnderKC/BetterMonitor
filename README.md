# Better Monitor - 企业级服务器监控运维系统

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.23+-blue.svg)](https://golang.org/)
[![Vue Version](https://img.shields.io/badge/Vue-3.0+-green.svg)](https://vuejs.org/)
[![Docker](https://img.shields.io/badge/Docker-Supported-blue.svg)](https://docker.com/)

Better Monitor 是一个现代化的企业级服务器监控和运维管理系统，提供实时监控、远程管理、自动化运维等功能，支持多服务器统一管理和自动化部署。

## 🌟 核心特性

### 📊 实时监控
- **系统指标监控**: CPU、内存、磁盘、网络实时监控
- **服务状态监控**: 系统服务、端口、进程状态监控
- **历史数据分析**: 长期性能趋势分析
- **预警通知**: 多渠道告警通知（邮件、短信、WebHook）

### 🖥️ 远程管理
- **Web终端**: 浏览器中的完整SSH终端体验
- **文件管理**: 在线文件浏览、编辑、上传、下载
- **进程管理**: 实时进程监控、启动、停止、重启
- **服务管理**: 系统服务状态管理

### 🐳 容器化支持
- **Docker管理**: 容器、镜像、网络、卷管理
- **Docker Compose**: 多容器应用编排管理
- **容器监控**: 容器资源使用情况监控
- **镜像管理**: 镜像构建、推送、拉取管理

### 🌐 Web服务管理
- **Nginx管理**: 配置文件管理、虚拟主机配置
- **SSL证书管理**: 证书申请、续期、部署
- **负载均衡**: 多服务器负载均衡配置
- **访问日志分析**: 实时日志监控和分析

### 🔄 自动化运维
- **Agent升级**: Dashboard 下发升级指令，Agent 自动从发布仓库获取版本
- **批量操作**: 多服务器批量命令执行
- **自动化脚本**: 定时任务和自动化脚本执行
- **配置同步**: 配置文件自动同步和备份

## 🏗️ 架构设计

### 系统架构
```
┌─────────────────────────────────────────────────────────────┐
│                     Better Monitor                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐                                        │
│  │   Web Dashboard │══════ WebSocket ══════┐                │
│  │  (Vue3 + Gin)   │                      │                │
│  └─────────────────┘                ┌─────▼─────┐          │
│                                     │  Agents   │ ...      │
│                                     └───────────┘          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 技术栈
- **前端**: Vue 3 + TypeScript + Vite + Element Plus
- **后端**: Go + Gin + GORM + SQLite/MySQL
- **Agent**: Go + WebSocket + 系统API
- **部署**: Docker + Docker Compose
- **监控**: Prometheus + Grafana（可选）

## 📁 项目结构

```
better_monitor/
├── 📁 frontend/                # Vue3 前端项目
│   ├── 📁 src/
│   │   ├── 📁 components/      # 可复用组件
│   │   ├── 📁 layout/          # 布局组件
│   │   ├── 📁 router/          # 路由配置
│   │   ├── 📁 stores/          # Pinia状态管理
│   │   ├── 📁 utils/           # 工具函数
│   │   └── 📁 views/           # 页面组件
│   ├── 📄 package.json         # 前端依赖
│   └── 📄 vite.config.ts       # Vite配置
├── 📁 backend/                 # Go后端项目
│   ├── 📁 controllers/         # 控制器层
│   ├── 📁 models/              # 数据模型
│   ├── 📁 services/            # 业务逻辑层
│   ├── 📁 middleware/          # 中间件
│   ├── 📁 routes/              # 路由配置
│   ├── 📁 utils/               # 工具函数
│   ├── 📄 go.mod               # Go依赖
│   └── 📄 main.go              # 入口文件
├── 📁 agent/                   # Agent监控程序
│   ├── 📁 cmd/                 # 命令行入口
│   ├── 📁 config/              # 配置管理
│   ├── 📁 internal/            # 内部包
│   │   ├── 📁 handler/         # 消息处理器
│   │   ├── 📁 monitor/         # 监控实现
│   │   └── 📁 server/          # 服务器通信
│   └── 📁 pkg/                 # 共享包
├── 📁 docs/                    # 项目文档
├── 📁 data/                    # 数据存储
├── 📁 logs/                    # 日志文件
├── 📄 docker-compose.all-in-one.yml # Docker部署配置
├── 📄 Dockerfile.all-in-one        # Docker镜像配置
└── 📄 README.md                # 项目说明
```

## 🚀 快速开始

### 系统要求
- **操作系统**: Linux/Windows/macOS
- **Docker**: 20.10+
- **Docker Compose**: 2.0+ (可选)
- **端口**: 3333 (Dashboard)
- **磁盘空间**: 建议至少 2GB 可用空间

### 方式一：一键安装（推荐）

使用官方安装脚本，支持安装、升级、卸载和数据迁移：

```bash
curl -fsSL https://raw.githubusercontent.com/EnderKC/BetterMonitor/refs/heads/main/install-dashboard.sh | sudo bash
```

或者下载后执行：

```bash
wget https://raw.githubusercontent.com/EnderKC/BetterMonitor/refs/heads/main/install-dashboard.sh
chmod +x install-dashboard.sh
sudo ./install-dashboard.sh
```

**脚本功能：**
- 🚀 一键安装面板
- 🔄 一键升级到最新版本
- 🗑️ 一键卸载（可选保留数据）
- 📦 数据备份与恢复
- 🔀 服务器间数据迁移
- 📊 查看运行状态

**命令行模式：**
```bash
# 直接安装
sudo ./install-dashboard.sh install

# 升级面板
sudo ./install-dashboard.sh upgrade

# 备份数据
sudo ./install-dashboard.sh backup

# 查看状态
sudo ./install-dashboard.sh status
```

### 方式二：Docker Compose 部署

#### 1. 使用预构建镜像（推荐）
```bash
# 创建目录
mkdir -p /opt/better-monitor && cd /opt/better-monitor

# 创建 docker-compose.yml
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

# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f better-monitor
```

#### 2. 源码构建部署
```bash
# 克隆项目
git clone https://github.com/EnderKC/BetterMonitor.git
cd BetterMonitor

# 构建并启动
docker-compose -f docker-compose.all-in-one.yml up -d --build

# 查看状态
docker-compose -f docker-compose.all-in-one.yml ps
```

### 方式三：Docker Run 部署

适合不想使用 Docker Compose 的场景：

```bash
# 创建数据目录
mkdir -p /opt/better-monitor/{data,logs}

# 生成 JWT Secret
JWT_SECRET=$(openssl rand -base64 32)

# 运行容器
docker run -d \
  --name better-monitor \
  --restart unless-stopped \
  -p 3333:3333 \
  -v /opt/better-monitor/data:/app/data:rw \
  -v /opt/better-monitor/logs:/app/logs:rw \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e TZ=Asia/Shanghai \
  -e JWT_SECRET="${JWT_SECRET}" \
  -e VERSION=latest \
  --security-opt no-new-privileges:true \
  enderhkc/better-monitor:latest

# 查看日志
docker logs -f better-monitor
```

### 方式四：手动部署

适合需要自定义部署的高级用户：

1. 从 [Releases](https://github.com/EnderKC/BetterMonitor/releases) 页面下载最新版本
2. 解压并配置环境变量
3. 启动后端服务和前端静态文件服务器

详细步骤请参考：[手动部署文档](docs/manual-deployment.md)

### 访问系统

安装完成后，通过浏览器访问：

- **访问地址**: http://your-server-ip:3333
- **默认账号**: admin
- **默认密码**: admin123

> ⚠️ **安全提醒**:
> 1. 首次登录后请立即修改默认密码
> 2. 建议配置 HTTPS 证书
> 3. 妥善保管 JWT_SECRET
> 4. 定期备份数据

### 常用管理命令

```bash
# 查看容器状态
docker ps -a | grep better-monitor

# 查看实时日志
docker logs -f better-monitor

# 重启服务
docker restart better-monitor

# 停止服务
docker stop better-monitor

# 启动服务
docker start better-monitor

# 进入容器
docker exec -it better-monitor bash

# 查看资源使用
docker stats better-monitor

# 更新到最新版本
docker pull enderhkc/better-monitor:latest
docker stop better-monitor
docker rm better-monitor
# 然后重新运行 docker run 命令
```

### 数据备份与迁移

使用一键脚本进行数据管理：

```bash
# 备份数据
sudo ./install-dashboard.sh backup

# 恢复数据
sudo ./install-dashboard.sh restore

# 创建迁移包（用于迁移到新服务器）
sudo ./install-dashboard.sh migrate
# 选择 "1. 创建迁移包"

# 在新服务器上导入迁移包
sudo ./install-dashboard.sh migrate
# 选择 "2. 导入迁移包"
```

**手动备份：**
```bash
# 备份数据目录
tar -czf better-monitor-backup-$(date +%Y%m%d).tar.gz \
  -C /opt/better-monitor data logs .env docker-compose.yml

# 恢复数据
tar -xzf better-monitor-backup-20240101.tar.gz -C /opt/better-monitor
docker restart better-monitor
```

## 🔧 Agent安装与配置

### 获取 Agent 二进制

Better Monitor 不再依赖独立的 OTA 服务器。所有 Agent 安装包都通过 GitHub Releases（或你在系统设置中配置的镜像仓库）分发。登录 Dashboard → “服务器管理” → “令牌” 可以看到当前的下载链接和需要填入的 `server_id`/`secret_key`。

#### Linux / macOS
```bash
# 以 Linux amd64 为例
curl -L https://github.com/your-username/better-monitor/releases/latest/download/better-monitor-agent-linux-amd64 -o better-monitor-agent
chmod +x better-monitor-agent

# 使用命令行参数启动
sudo ./better-monitor-agent \
  --server http://your-dashboard-url:3333 \
  --token your-registration-token
```

其他架构（arm64/armv7 等）只需要替换下载文件名即可。

#### Windows
1. 从 Releases 页面下载 `better-monitor-agent-windows-amd64.zip`
2. 解压后以管理员身份运行 PowerShell：
   ```powershell
   .\better-monitor-agent.exe --server https://your-dashboard-url:3333 --token your-registration-token
   ```

也可以在 `agent/config/agent.yaml` 中写入 `server_id` 与 `secret_key`，随后以服务方式运行。

### 手动安装

#### 1. 下载Agent
```bash
# 从 GitHub Releases 下载最新版本（示例：Linux amd64）
curl -L https://github.com/your-username/better-monitor/releases/latest/download/better-monitor-agent-linux-amd64 -o /usr/local/bin/better-monitor-agent
chmod +x /usr/local/bin/better-monitor-agent
```

#### 2. 创建配置文件
```bash
# 创建配置目录
sudo mkdir -p /etc/better-monitor

# 创建配置文件
sudo tee /etc/better-monitor/agent.yaml << EOF
# 服务器配置
server:
  url: "http://your-dashboard-url:3333"
  server_id: 0  # 注册后会自动设置
  secret_key: ""  # 注册后会自动设置

# 监控配置
monitor:
  interval: "30s"  # 监控数据收集间隔
  
# 心跳配置
heartbeat:
  interval: "10s"  # 心跳间隔

# 日志配置
log:
  level: "info"
  file: "/var/log/better-monitor/agent.log"

## 🔄 Agent升级机制

Better Monitor 通过 Dashboard 下发升级指令，Agent 会自动从配置的发布仓库（默认为 GitHub Releases）下载匹配平台的最新版本并完成替换。

### 发布流程
1. 通过 `Releases/build.py` 构建多平台二进制文件。
2. 将生成的压缩包上传到 GitHub Releases 或企业内部制品仓库。
3. 在 Dashboard 的 **版本信息** 页面勾选需要升级的服务器，点击“升级”即可完成批量滚动升级。

### 关键配置
在系统设置中可以调整以下字段：

```json
{
  "agent_release_repo": "your-org/better-monitor-agent",
  "agent_release_channel": "stable",
  "agent_release_mirror": "https://download.fastgit.org"
}
```

- `agent_release_repo`：用于获取发布信息的 GitHub 仓库。
- `agent_release_channel`：默认使用 `stable`，也可以切换到 `prerelease/nightly` 获取预发布版本。
- `agent_release_mirror`：可选，替换下载域名，适合国内或离线环境。

Agent 接收到升级任务后会校验包体、备份当前二进制并应用新版本。如升级失败会自动回滚到上一版本，确保系统高可用。

## 📊 功能详解

### 1. 实时监控
- **系统指标**: CPU使用率、内存使用情况、磁盘I/O、网络流量
- **进程监控**: 实时进程列表、资源占用、进程树
- **服务状态**: 系统服务状态、端口监听状态
- **历史数据**: 长期趋势分析、性能基线

### 2. 告警通知
- **告警规则**: 灵活的告警规则配置
- **通知渠道**: 邮件、短信、WebHook、钉钉、企业微信
- **告警等级**: 信息、警告、错误、严重
- **告警抑制**: 避免告警风暴

### 3. 文件管理
- **在线编辑**: 支持语法高亮的代码编辑器
- **文件上传**: 拖拽上传、批量上传
- **权限管理**: 文件权限查看和修改
- **备份恢复**: 自动备份和一键恢复

### 4. 终端管理
- **Web SSH**: 浏览器中的完整SSH体验
- **多会话**: 支持多个终端会话
- **会话管理**: 会话保持、断线重连
- **终端录制**: 操作录制和回放

### 5. 容器管理
- **容器操作**: 启动、停止、重启、删除
- **镜像管理**: 镜像构建、推送、拉取
- **容器监控**: 资源使用监控
- **Compose管理**: 多容器应用管理

### 6. Nginx管理
- **配置管理**: 配置文件在线编辑
- **虚拟主机**: 快速创建虚拟主机
- **SSL证书**: 证书申请和自动续期
- **日志分析**: 访问日志实时分析

## 🛠️ 开发指南

### 开发环境搭建

#### 前端开发
```bash
cd frontend
npm install
npm run dev
```

#### 后端开发
```bash
cd backend
go mod tidy
go run main.go
```

#### Agent开发
```bash
cd agent
go mod tidy
go run cmd/agent/main.go
```

### 构建发布

#### 前端构建
```bash
cd frontend
npm run build
```

#### 后端构建
```bash
cd backend
go build -o better-monitor-backend main.go
```

#### Agent构建
```bash
cd agent
go build -o better-monitor-agent cmd/agent/main.go
```

#### 批量构建
```bash
cd Releases
python build.py --all
```

### 测试

#### 单元测试
```bash
# 后端测试
cd backend
go test ./...

# Agent测试
cd agent
go test ./...
```

#### 集成测试
```bash
# 使用Docker运行集成测试
docker-compose -f docker-compose.test.yml up
```

## 🔒 安全配置

### 1. 认证与授权
- **JWT认证**: 使用JWT进行用户认证
- **RBAC权限**: 基于角色的访问控制
- **API密钥**: Agent通信使用密钥认证
- **会话管理**: 会话超时和并发控制

### 2. 网络安全
- **HTTPS**: 强制使用HTTPS通信
- **防火墙**: 合理配置防火墙规则
- **VPN**: 建议使用VPN访问
- **IP白名单**: 限制访问IP范围

### 3. 数据安全
- **数据加密**: 敏感数据加密存储
- **备份策略**: 定期数据备份
- **访问日志**: 完整的访问日志记录
- **安全审计**: 定期安全审计

### 4. 系统安全
```bash
# 1. 修改默认密码
# 登录后立即修改admin密码

# 2. 配置SSL证书
# 使用Let's Encrypt免费证书
certbot --nginx -d your-domain.com

# 3. 配置防火墙
ufw allow 22/tcp
ufw allow 3333/tcp
ufw allow 8086/tcp
ufw enable

# 4. 启用fail2ban
apt install fail2ban
systemctl enable fail2ban
systemctl start fail2ban
```

## 📈 性能优化

### 1. 数据库优化
- **索引优化**: 合理创建数据库索引
- **查询优化**: 优化慢查询
- **连接池**: 配置合适的连接池大小
- **数据清理**: 定期清理历史数据

### 2. 缓存策略
- **Redis缓存**: 使用Redis缓存热点数据
- **浏览器缓存**: 配置合适的缓存策略
- **CDN**: 使用CDN加速静态资源
- **数据压缩**: 启用Gzip压缩

### 3. 系统优化
```bash
# 1. 系统参数优化
echo "net.core.somaxconn = 65535" >> /etc/sysctl.conf
echo "net.ipv4.tcp_max_syn_backlog = 65535" >> /etc/sysctl.conf
sysctl -p

# 2. 文件描述符限制
echo "* soft nofile 65535" >> /etc/security/limits.conf
echo "* hard nofile 65535" >> /etc/security/limits.conf

# 3. Docker优化
docker system prune -a
```

## 🚨 故障排查

### 常见问题

#### 1. Agent连接失败
```bash
# 检查网络连通性
curl -I http://dashboard-url:3333/health

# 检查Agent配置
cat /etc/better-monitor/agent.yaml

# 查看Agent日志
tail -f /var/log/better-monitor/agent.log

# 重启Agent服务
systemctl restart better-monitor-agent
```

#### 2. Dashboard无法访问
```bash
# 检查Docker容器状态
docker-compose ps

# 查看容器日志
docker-compose logs -f better-monitor

# 检查端口占用
netstat -tlnp | grep 3333

# 重启服务
docker-compose restart
```

#### 3. Agent升级失败
```bash
# 检查Dashboard发布API
curl http://dashboard-url:3333/api/agents/releases/latest

# 查看Agent升级日志
grep "upgrade" /var/log/better-monitor/agent.log

# 手动触发升级（示例）
curl -X POST http://dashboard-url:3333/api/servers/upgrade \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"serverIds":[1],"targetVersion":"1.2.3"}'
```

### 日志分析

#### 系统日志位置
- **Dashboard**: `/app/logs/backend.log`
- **Agent**: `/var/log/better-monitor/agent.log`
- **Nginx**: `/var/log/nginx/access.log`

#### 日志级别
- **DEBUG**: 调试信息
- **INFO**: 一般信息
- **WARN**: 警告信息
- **ERROR**: 错误信息

## 🔧 配置参考

### Dashboard配置
```yaml
# config/config.yaml
server:
  port: 3333
  host: "0.0.0.0"
  
database:
  type: "sqlite"
  dsn: "data/better-monitor.db"
  
jwt:
  secret: "your-jwt-secret"
  expire: "24h"

agent_release:
  repo: "EnderKC/BetterMonitor"
  channel: "stable"
  mirror: ""
```

### Agent配置
```yaml
# agent.yaml
server:
  url: "http://dashboard-url:3333"
  server_id: 1
  secret_key: "agent-secret-key"
  
monitor:
  interval: "30s"
  cpu_threshold: 80
  memory_threshold: 85
  disk_threshold: 90
  
heartbeat:
  interval: "10s"
  timeout: "30s"
  
log:
  level: "info"
  file: "/var/log/better-monitor/agent.log"
  max_size: 100  # MB
  max_backups: 5

update_repo: "EnderKC/BetterMonitor"
update_channel: "stable"
update_mirror: ""
```

## 📚 API文档

### 认证接口
| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/auth/login` | 用户登录 |
| POST | `/api/auth/logout` | 用户退出 |
| GET | `/api/auth/profile` | 获取用户信息 |
| PUT | `/api/auth/profile` | 更新用户信息 |

### 服务器管理
| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/servers` | 获取服务器列表 |
| POST | `/api/servers` | 添加服务器 |
| GET | `/api/servers/:id` | 获取服务器详情 |
| PUT | `/api/servers/:id` | 更新服务器信息 |
| DELETE | `/api/servers/:id` | 删除服务器 |

### 监控数据
| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/servers/:id/monitor` | 获取监控数据 |
| GET | `/api/servers/:id/processes` | 获取进程列表 |
| GET | `/api/servers/:id/docker` | 获取Docker信息 |
| GET | `/api/servers/:id/nginx` | 获取Nginx配置 |

详细API文档请参考：[API Documentation](docs/api.md)

## 🤝 贡献指南

### 贡献类型
- 🐛 **Bug修复**: 修复现有功能的问题
- ✨ **新功能**: 添加新的功能特性
- 📝 **文档改进**: 改进项目文档
- 🎨 **代码优化**: 改进代码结构和性能
- 🧪 **测试**: 添加或改进测试用例

### 贡献流程
1. **Fork项目**: 点击Fork按钮创建副本
2. **创建分支**: `git checkout -b feature/new-feature`
3. **编写代码**: 遵循项目代码规范
4. **提交代码**: `git commit -m "feat: add new feature"`
5. **推送分支**: `git push origin feature/new-feature`
6. **创建PR**: 创建Pull Request

### 代码规范
- **Go代码**: 遵循Go官方代码规范
- **Vue代码**: 遵循Vue官方风格指南
- **提交信息**: 使用Conventional Commits规范
- **文档**: 更新相关文档

### 开发环境
```bash
# 1. 克隆项目
git clone https://github.com/your-username/better-monitor.git
cd better-monitor

# 2. 安装依赖
# 前端
cd frontend && npm install

# 后端
cd backend && go mod tidy

# Agent
cd agent && go mod tidy

# 3. 启动开发环境
# 按照开发指南启动各个服务
```

## 📄 许可证

本项目采用 [MIT License](LICENSE) 开源许可证。

## 🙏 致谢

感谢所有为Better Monitor项目做出贡献的开发者和用户！

### 技术栈致谢
- [Vue.js](https://vuejs.org/) - 渐进式JavaScript框架
- [Go](https://golang.org/) - 高性能编程语言
- [Gin](https://gin-gonic.com/) - 高性能Go Web框架
- [Element Plus](https://element-plus.org/) - Vue 3 UI组件库
- [Docker](https://www.docker.com/) - 容器化平台

### 社区贡献
- 🌟 **Star**: 如果这个项目对您有帮助，请给我们一个Star
- 🐛 **Issue**: 发现问题请提交Issue
- 🚀 **PR**: 欢迎提交Pull Request
- 📖 **文档**: 帮助改进项目文档

## 📞 联系我们

- **GitHub**: [项目地址](https://github.com/your-username/better-monitor)
- **Issues**: [问题反馈](https://github.com/your-username/better-monitor/issues)
- **Discussions**: [讨论区](https://github.com/your-username/better-monitor/discussions)
- **Email**: support@better-monitor.com

---

**Better Monitor** - 让服务器监控更简单、更智能！
