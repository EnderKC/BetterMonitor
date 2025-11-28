# Better Monitor API Documentation

## Overview

Better Monitor 提供了一套完整的 RESTful API 和 WebSocket 接口，支持服务器监控、管理和实时通信功能。API 采用 JWT 认证，支持多种角色权限控制。

## 认证方式

### 1. JWT 认证
- **Header**: `Authorization: Bearer <token>`
- **适用**: 前端用户请求
- **获取**: 通过 `/api/auth/login` 登录获取
- **角色**: `user`, `admin`

### 2. Agent 认证
- **Header**: `X-Secret-Key: <secret_key>`
- **适用**: Agent 设备连接
- **获取**: 服务器注册时自动生成

## 基础信息

- **Base URL**: `http://your-domain:3333`
- **Content-Type**: `application/json`
- **编码**: UTF-8

## API 分类

### 1. 认证接口

#### 用户登录
```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

**响应**:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin",
    "last_login_at": "2024-01-01T00:00:00Z"
  }
}
```

#### 用户注册
```http
POST /api/auth/register
Content-Type: application/json

{
  "username": "newuser",
  "password": "password123",
  "role": "user"
}
```

### 2. 服务器管理接口

#### 获取服务器列表
```http
GET /api/servers
Authorization: Bearer <token>
```

**响应**:
```json
{
  "servers": [
    {
      "id": 1,
      "name": "Web Server",
      "ip": "192.168.1.100",
      "os": "Linux",
      "arch": "x86_64",
      "cpu_cores": 4,
      "cpu_model": "Intel Core i7",
      "memory_total": 8388608,
      "disk_total": 107374182400,
      "last_heartbeat": "2024-01-01T00:00:00Z",
      "online": true,
      "status": "online",
      "user_id": 1,
      "tags": "web,production",
      "description": "生产环境Web服务器",
      "agent_version": "1.0.0"
    }
  ]
}
```

#### 获取单个服务器信息
```http
GET /api/servers/{id}
Authorization: Bearer <token>
```

#### 创建服务器
```http
POST /api/servers
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Database Server",
  "ip": "192.168.1.101",
  "description": "MySQL数据库服务器",
  "tags": "database,mysql"
}
```

#### 更新服务器信息
```http
PUT /api/servers/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Updated Server Name",
  "description": "Updated description",
  "tags": "updated,tags"
}
```

#### 删除服务器
```http
DELETE /api/servers/{id}
Authorization: Bearer <token>
```

#### Agent 服务器注册
```http
POST /api/servers/register
X-Secret-Key: <secret_key>
Content-Type: application/json

{
  "name": "Agent Server",
  "ip": "192.168.1.102",
  "os": "Linux",
  "arch": "x86_64",
  "cpu_cores": 8,
  "cpu_model": "Intel Xeon E5",
  "memory_total": 16777216,
  "disk_total": 214748364800,
  "system_info": "{\"uptime\":3600,\"kernel\":\"5.4.0-42-generic\"}",
  "agent_version": "1.0.0"
}
```

#### 提交监控数据
```http
POST /api/servers/{id}/monitor
X-Secret-Key: <secret_key>
Content-Type: application/json

{
  "cpu_usage": 45.5,
  "memory_used": 4194304,
  "memory_total": 8388608,
  "disk_used": 53687091200,
  "disk_total": 107374182400,
  "network_in": 1024.5,
  "network_out": 2048.7,
  "load_avg_1": 0.5,
  "load_avg_5": 0.8,
  "load_avg_15": 1.2
}
```

#### 获取监控数据
```http
GET /api/servers/{id}/monitor?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z&limit=100
Authorization: Bearer <token>
```

### 3. 文件管理接口

#### 上传文件
```http
POST /api/servers/{id}/files/upload
Authorization: Bearer <token>
Content-Type: multipart/form-data

file: <file_content>
path: /path/to/destination
```

#### 下载文件
```http
GET /api/servers/{id}/files/download?path=/path/to/file
Authorization: Bearer <token>
```

#### 获取文件内容
```http
GET /api/servers/{id}/files/content?path=/path/to/file
Authorization: Bearer <token>
```

### 4. 进程管理接口

#### 获取进程列表
```http
GET /api/servers/{id}/processes
Authorization: Bearer <token>
```

#### 终止进程
```http
POST /api/servers/{id}/processes/{pid}/kill
Authorization: Bearer <token>
```

### 5. Docker 管理接口

#### 获取容器列表
```http
GET /api/servers/{id}/docker/containers
Authorization: Bearer <token>
```

#### 获取镜像列表
```http
GET /api/servers/{id}/docker/images
Authorization: Bearer <token>
```

#### 启动容器
```http
POST /api/servers/{id}/docker/containers/{containerId}/start
Authorization: Bearer <token>
```

#### 停止容器
```http
POST /api/servers/{id}/docker/containers/{containerId}/stop
Authorization: Bearer <token>
```

#### 重启容器
```http
POST /api/servers/{id}/docker/containers/{containerId}/restart
Authorization: Bearer <token>
```

#### 删除容器
```http
DELETE /api/servers/{id}/docker/containers/{containerId}
Authorization: Bearer <token>
```

#### 获取容器日志
```http
GET /api/servers/{id}/docker/containers/{containerId}/logs
Authorization: Bearer <token>
```

#### 获取 Compose 项目
```http
GET /api/servers/{id}/docker/composes
Authorization: Bearer <token>
```

#### 启动 Compose 项目
```http
POST /api/servers/{id}/docker/composes/{project}/up
Authorization: Bearer <token>
```

#### 停止 Compose 项目
```http
POST /api/servers/{id}/docker/composes/{project}/down
Authorization: Bearer <token>
```

### 6. Nginx 管理接口

#### 获取 Nginx 状态
```http
#### 启动 Nginx
```http
POST /api/servers/{id}/nginx/start
Authorization: Bearer <token>
```

#### 停止 Nginx
```http
POST /api/servers/{id}/nginx/stop
Authorization: Bearer <token>
```

#### 重启 Nginx
```http
POST /api/servers/{id}/nginx/restart
Authorization: Bearer <token>
```

#### 重新加载配置
```http
POST /api/servers/{id}/nginx/reload
Authorization: Bearer <token>
```

#### 获取 Nginx 配置
```http
GET /api/servers/{id}/nginx/config
Authorization: Bearer <token>
```

#### 更新 Nginx 配置
```http
POST /api/servers/{id}/nginx/config
Authorization: Bearer <token>
Content-Type: application/json

{
  "config": "server {\n    listen 80;\n    server_name example.com;\n    root /var/www/html;\n}"
}
```

#### 获取站点列表
```http
GET /api/servers/{id}/nginx/sites
Authorization: Bearer <token>
```

#### 创建站点
```http
POST /api/servers/{id}/nginx/sites
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "example.com",
  "config": "server {\n    listen 80;\n    server_name example.com;\n    root /var/www/example;\n}"
}
```

#### 获取站点配置
```http
GET /api/servers/{id}/nginx/sites/{siteName}
Authorization: Bearer <token>
```

#### 更新站点配置
```http
PUT /api/servers/{id}/nginx/sites/{siteName}
Authorization: Bearer <token>
Content-Type: application/json

{
  "config": "updated_config_content"
}
```

#### 删除站点
```http
DELETE /api/servers/{id}/nginx/sites/{siteName}
Authorization: Bearer <token>
```

#### 启用站点
```http
POST /api/servers/{id}/nginx/sites/{siteName}/enable
Authorization: Bearer <token>
```

#### 禁用站点
```http
POST /api/servers/{id}/nginx/sites/{siteName}/disable
Authorization: Bearer <token>
```

#### 获取 SSL 证书信息
```http
GET /api/servers/{id}/nginx/ssl/{domain}
Authorization: Bearer <token>
```

#### 生成 SSL 证书
```http
POST /api/servers/{id}/nginx/ssl/{domain}
Authorization: Bearer <token>
```

### 7. 告警管理接口

#### 获取告警设置
```http
GET /api/alerts/settings
Authorization: Bearer <token>
```

#### 创建告警设置
```http
POST /api/alerts/settings
Authorization: Bearer <token>
Content-Type: application/json

{
  "type": "cpu",
  "threshold": 80.0,
  "duration": 300,
  "enabled": true,
  "server_id": 1
}
```

#### 更新告警设置
```http
PUT /api/alerts/settings/{id}
Authorization: Bearer <token>
Content-Type: application/json

{
  "threshold": 85.0,
  "duration": 600,
  "enabled": false
}
```

#### 删除告警设置
```http
DELETE /api/alerts/settings/{id}
Authorization: Bearer <token>
```

#### 获取通知渠道
```http
GET /api/alerts/channels
Authorization: Bearer <token>
```

#### 创建通知渠道
```http
POST /api/alerts/channels
Authorization: Bearer <token>
Content-Type: application/json

{
  "type": "email",
  "name": "邮件通知",
  "config": "{\"smtp_host\":\"smtp.gmail.com\",\"smtp_port\":587,\"username\":\"user@gmail.com\",\"password\":\"password\",\"from\":\"alerts@example.com\",\"to\":\"admin@example.com\"}",
  "enabled": true
}
```

#### 获取告警记录
```http
GET /api/alerts/records?server_id=1&alert_type=cpu&only_unresolved=true&page=1&limit=10
Authorization: Bearer <token>
```

#### 解决告警
```http
POST /api/alerts/records/{id}/resolve
Authorization: Bearer <token>
```

### 8. 系统设置接口

#### 获取系统设置
```http
GET /api/settings
Authorization: Bearer <token>
```

#### 更新系统设置
```http
POST /api/settings
Authorization: Bearer <token>
Content-Type: application/json

{
  "site_title": "Better Monitor",
  "site_description": "企业级服务器监控系统",
  "max_servers": 100,
  "data_retention_days": 30
}
```

#### 获取公共设置
```http
GET /api/settings/public
```

#### 获取 Agent 设置
```http
GET /api/settings/agent
X-Secret-Key: <secret_key>
```

### 9. 终端管理接口

#### 获取终端会话列表
```http
GET /api/servers/{id}/terminal/sessions
Authorization: Bearer <token>
```

#### 创建终端会话
```http
POST /api/servers/{id}/terminal/sessions
Authorization: Bearer <token>
Content-Type: application/json

{
  "working_dir": "/home/user",
  "shell": "/bin/bash"
}
```

#### 获取终端会话信息
```http
GET /api/servers/{id}/terminal/sessions/{sessionId}
Authorization: Bearer <token>
```

#### 删除终端会话
```http
DELETE /api/servers/{id}/terminal/sessions/{sessionId}
Authorization: Bearer <token>
```

#### 获取会话工作目录
```http
GET /api/servers/{id}/terminal/sessions/{sessionId}/working-directory
Authorization: Bearer <token>
```

### 10. 版本和健康检查接口

#### 获取版本信息
```http
GET /api/version
```

**响应**:
```json
{
  "version": "1.0.0",
  "build_time": "2024-01-01T00:00:00Z",
  "git_commit": "abc123",
  "go_version": "1.21.0"
}
```

#### 健康检查
```http
GET /api/health
```

**响应**:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T00:00:00Z",
  "database": "connected",
  "active_agents": 5
}
```

#### 获取最新Agent版本
```http
GET /api/agents/releases/latest
Authorization: Bearer <jwt_token>
```

**响应**:
```json
{
  "success": true,
  "version": "1.2.3",
  "releasedAt": "2024-01-01T00:00:00Z",
  "release_repo": "your-org/better-monitor-agent",
  "assets": [
    {
      "name": "better-monitor-agent-linux-amd64.tar.gz",
      "download_url": "https://github.com/your-org/better-monitor-agent/releases/download/v1.2.3/...",
      "os": "linux",
      "arch": "amd64"
    }
  ]
}
```

#### 批量升级 Agent
```http
POST /api/servers/upgrade
Authorization: Bearer <jwt_token>
Content-Type: application/json

{
  "serverIds": [1, 2, 3],
  "targetVersion": "1.2.3",
  "channel": "stable"
}
```

**响应**:
```json
{
  "success": true,
  "message": "升级指令已触发，目标版本: 1.2.3",
  "result": {
    "success": [1, 2],
    "failure": [],
    "offline": [3],
    "missing": []
  }
}
```

## WebSocket 接口

### 1. 服务器监控 WebSocket
```
ws://your-domain:3333/ws/servers/{id}?token={jwt_token}
```

### 2. 终端 WebSocket
```
ws://your-domain:3333/ws/servers/{id}/terminal?session={session_id}
```

### 3. 公共监控 WebSocket
```
ws://your-domain:3333/public/ws/servers/{id}
```

### WebSocket 消息格式

#### 客户端发送
```json
{
  "type": "shell_command",
  "data": {
    "command": "ls -la",
    "session": "session_id"
  }
}
```

#### 服务端响应
```json
{
  "type": "shell_response",
  "data": {
    "output": "total 8\ndrwxr-xr-x 2 root root 4096 Jan  1 00:00 .\ndrwxr-xr-x 3 root root 4096 Jan  1 00:00 ..",
    "session": "session_id"
  }
}
```

### 支持的消息类型

- `shell_command` - 执行Shell命令
- `file_list` - 获取文件列表
- `file_content` - 获取文件内容
- `file_upload` - 上传文件
- `process_list` - 获取进程列表
- `process_kill` - 终止进程
- `docker_command` - Docker操作
- `nginx_command` - Nginx操作
- `heartbeat` - 心跳消息

## 错误处理

### 标准错误响应
```json
{
  "error": "错误描述",
  "code": "ERROR_CODE",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### 常见错误代码

- `UNAUTHORIZED` - 未授权访问
- `FORBIDDEN` - 权限不足
- `NOT_FOUND` - 资源不存在
- `INVALID_REQUEST` - 请求参数错误
- `SERVER_ERROR` - 服务器内部错误
- `RATE_LIMITED` - 请求频率超限

## 速率限制

- 登录接口: 每分钟 5 次
- 注册接口: 每分钟 3 次
- 其他接口: 每秒 10 次

## 数据格式

### 时间格式
所有时间字段使用 ISO 8601 格式：`2024-01-01T00:00:00Z`

### 数据单位
- 内存: 字节 (bytes)
- 磁盘: 字节 (bytes)
- 网络: 字节/秒 (bytes/s)
- CPU: 百分比 (0-100)

## 示例代码

### JavaScript (前端)
```javascript
// 登录获取token
const login = async (username, password) => {
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ username, password })
  });
  
  const data = await response.json();
  if (data.token) {
    localStorage.setItem('token', data.token);
  }
  return data;
};

// 获取服务器列表
const getServers = async () => {
  const token = localStorage.getItem('token');
  const response = await fetch('/api/servers', {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });
  
  return await response.json();
};

// WebSocket连接
const connectWebSocket = (serverId) => {
  const token = localStorage.getItem('token');
  const ws = new WebSocket(`ws://localhost:3333/ws/servers/${serverId}?token=${token}`);
  
  ws.onopen = () => {
    console.log('WebSocket连接已建立');
  };
  
  ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    console.log('收到消息:', message);
  };
  
  return ws;
};
```

### Go (Agent)
```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type MonitorData struct {
    CPUUsage    float64 `json:"cpu_usage"`
    MemoryUsed  int64   `json:"memory_used"`
    MemoryTotal int64   `json:"memory_total"`
    DiskUsed    int64   `json:"disk_used"`
    DiskTotal   int64   `json:"disk_total"`
    NetworkIn   float64 `json:"network_in"`
    NetworkOut  float64 `json:"network_out"`
    LoadAvg1    float64 `json:"load_avg_1"`
    LoadAvg5    float64 `json:"load_avg_5"`
    LoadAvg15   float64 `json:"load_avg_15"`
}

func sendMonitorData(serverID int, secretKey string, data MonitorData) error {
    jsonData, err := json.Marshal(data)
    if err != nil {
        return err
    }
    
    req, err := http.NewRequest("POST", 
        fmt.Sprintf("http://localhost:3333/api/servers/%d/monitor", serverID),
        bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Secret-Key", secretKey)
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    return nil
}
```

### Python (自动化脚本)
```python
import requests
import json

class BetterMonitorAPI:
    def __init__(self, base_url, token=None):
        self.base_url = base_url
        self.token = token
        self.session = requests.Session()
        
        if token:
            self.session.headers.update({
                'Authorization': f'Bearer {token}'
            })
    
    def login(self, username, password):
        response = self.session.post(f'{self.base_url}/api/auth/login', 
                                   json={'username': username, 'password': password})
        data = response.json()
        if 'token' in data:
            self.token = data['token']
            self.session.headers.update({
                'Authorization': f'Bearer {self.token}'
            })
        return data
    
    def get_servers(self):
        response = self.session.get(f'{self.base_url}/api/servers')
        return response.json()
    
    def get_server_monitor(self, server_id, start_time=None, end_time=None):
        params = {}
        if start_time:
            params['start_time'] = start_time
        if end_time:
            params['end_time'] = end_time
            
        response = self.session.get(f'{self.base_url}/api/servers/{server_id}/monitor', 
                                  params=params)
        return response.json()

# 使用示例
api = BetterMonitorAPI('http://localhost:3333')
api.login('admin', 'admin123')
servers = api.get_servers()
print(f"找到 {len(servers['servers'])} 台服务器")
```

## 更多信息

- [部署文档](deployment.md)
- [开发指南](development.md)
- [项目主页](../README.md)
- [故障排查](../README.md#故障排查)

---

如有问题，请查看 [故障排查指南](../README.md#故障排查) 或提交 [Issue](https://github.com/your-repo/better-monitor/issues)。
