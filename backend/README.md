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

## LifeLogger 数据接入

后端已经内置 `/api/life-logger/events` 接口用于接收 LifeLogger iOS App 的各类数据。要让链路跑通，请按以下步骤操作：

1. **创建生命探针**：登录 Dashboard → 生命探针 → 新增，把 App 设置页显示的 `Device ID` 填入 `device_id` 字段（也可以直接调用 `POST /api/life-probes`）。未登记的设备无法上传。
2. **配置 App**：在 LifeLogger 设置里把服务器地址填写成 `https://<你的域名>/api/life-logger/events`，点击「测试连接」，后台会返回 `pong` 并验证 `device_id`。
3. **开始采集**：打开 App 主页点击「开启记录」，心率、步数、睡眠、专注模式、锁屏事件都会按 Envelope 规范上传。

### 数据格式

- Envelope 字段：`event_id`, `timestamp`, `device_id`, `battery_level`, `data_type`, `payload`
- `data_type` 取值：`heart_rate`、`steps_detailed`、`sleep_detailed`、`focus_status`、`screen_event`
- 各 payload 的字段说明详见项目根目录的 `需求.md`

### 服务端去重

服务器会为不同的数据加上唯一索引并在写入时使用 `ON CONFLICT DO NOTHING`：

- 心率：`life_probe_id + measure_time`
- 步数：`life_probe_id + sample_type + start_time + end_time`
- 睡眠：`life_probe_id + stage + start_time + end_time`
- 专注模式：`life_probe_id + event_time + is_focused`
- 屏幕事件：`life_probe_id + event_time + action`

因此客户端即使因为网络抖动重复上报，同一条数据也只会入库一次；探针的 `last_sync_at`、心率和专注模式的最新状态也只会在收到更新的时间戳时才覆盖。

## 配置说明

项目配置通过环境变量或.env文件进行设置：

- `PORT` - 服务器端口，默认为8080
- `DB_PATH` - SQLite数据库路径，默认为./data/data.db
- `JWT_SECRET` - JWT签名密钥，请在生产环境中修改 
