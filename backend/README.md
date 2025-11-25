# 服务器运维系统 - 后端

## 项目简介

本项目是基于Golang的服务器应急响应与运维系统后端部分，使用Gin框架和SQLite数据库开发。

## 功能特点

- 用户认证与授权系统
- 服务器管理与监控
- WebSocket实时通信
- 在线Shell和文件管理
- Docker容器管理
- Nginx配置管理

## 环境要求

- Go 1.16+
- SQLite 3

## 安装与运行

1. 克隆仓库并进入项目目录
```bash
git clone https://github.com/yourusername/server-ops.git
cd server-ops/backend
```

2. 下载依赖
```bash
go mod tidy
```

3. 配置环境变量（可选）
```bash 
# 复制示例配置文件
cp .env.example .env

# 编辑配置文件
nano .env
```

4. 运行项目
```bash
go run main.go
```

服务器默认运行在 http://localhost:8080

## API文档

### 认证相关

- `POST /api/login` - 用户登录
- `GET /api/profile` - 获取用户信息
- `POST /api/change-password` - 修改密码

### 服务器管理

- `GET /api/servers` - 获取所有服务器
- `GET /api/servers/:id` - 获取单个服务器详情
- `POST /api/servers` - 创建新服务器
- `PUT /api/servers/:id` - 更新服务器信息
- `DELETE /api/servers/:id` - 删除服务器

### 监控数据

- `GET /api/servers/:id/monitor` - 获取服务器监控数据（面板使用）

### WebSocket

- `GET /api/servers/public/ws` - 探针页面获取全部服务器列表
- `GET /api/servers/public/:id/ws` - 探针页面订阅单台服务器
- `GET /api/servers/:id/ws` - Agent/控制台共用的 WebSocket 连接

## 配置说明

项目配置通过环境变量或.env文件进行设置：

- `PORT` - 服务器端口，默认为8080
- `DB_PATH` - SQLite数据库路径，默认为./data/data.db
- `JWT_SECRET` - JWT签名密钥，请在生产环境中修改 
