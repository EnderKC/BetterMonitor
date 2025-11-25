# Better Monitor 开发文档

## 概述

Better Monitor 是一个基于 Vue 3 + Go 的现代化服务器监控系统。本文档详细介绍项目的开发环境搭建、代码结构、开发规范和贡献指南。

## 技术栈

### 前端
- **框架**: Vue 3 + TypeScript
- **构建工具**: Vite
- **UI 库**: Element Plus
- **状态管理**: Pinia
- **路由**: Vue Router
- **WebSocket**: 原生 WebSocket API
- **图表库**: ECharts
- **HTTP 客户端**: Axios

### 后端
- **语言**: Go 1.21+
- **Web 框架**: Gin
- **ORM**: GORM
- **数据库**: SQLite/MySQL/PostgreSQL
- **认证**: JWT
- **WebSocket**: Gorilla WebSocket
- **配置**: Viper
- **日志**: Logrus

### Agent
- **语言**: Go 1.21+
- **WebSocket**: Gorilla WebSocket
- **系统监控**: gopsutil
- **配置**: Viper
- **日志**: Logrus

### 部署
- **容器化**: Docker + Docker Compose
- **反向代理**: Nginx
- **进程管理**: Supervisor
- **CI/CD**: GitHub Actions

## 开发环境搭建

### 1. 环境要求

- **Go**: 1.21+
- **Node.js**: 18+
- **Git**: 2.30+
- **Docker**: 20.10+ (可选)
- **MySQL**: 8.0+ (可选，默认使用 SQLite)

### 2. 克隆项目

```bash
git clone https://github.com/your-repo/better-monitor.git
cd better-monitor
```

### 3. 前端开发环境

```bash
# 进入前端目录
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build

# 类型检查
npm run type-check

# 代码格式化
npm run format
```

#### 前端环境变量

```bash
# frontend/.env.development
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_BASE_URL=ws://localhost:8080
VITE_APP_TITLE=Better Monitor
```

### 4. 后端开发环境

```bash
# 进入后端目录
cd backend

# 下载依赖
go mod tidy

# 启动开发服务器
go run main.go

# 构建二进制文件
go build -o better-monitor-backend main.go

# 运行测试
go test ./...

# 代码格式化
go fmt ./...
```

#### 后端环境变量

```bash
# backend/.env
JWT_SECRET=your_development_jwt_secret
DB_TYPE=sqlite
DB_DSN=data/better-monitor.db
PORT=8080
GIN_MODE=debug
```

### 5. Agent 开发环境

```bash
# 进入 Agent 目录
cd agent

# 下载依赖
go mod tidy

# 启动开发
go run cmd/agent/main.go -config config/agent.yaml

# 构建二进制文件
go build -o better-monitor-agent cmd/agent/main.go

# 运行测试
go test ./...
```

#### Agent 配置文件

```yaml
# agent/config/agent.yaml
server:
  url: "http://localhost:8080"
  server_id: 0
  secret_key: ""

monitor:
  interval: "30s"
  
heartbeat:
  interval: "10s"

log:
  level: "debug"
  file: "logs/agent.log"
```

## 项目结构详解

### 前端结构

```
frontend/
├── src/
│   ├── components/          # 可复用组件
│   │   ├── ServerStatusBadge.vue
│   │   ├── VersionInfo.vue
│   │   └── ...
│   ├── layout/             # 布局组件
│   │   └── AdminLayout.vue
│   ├── router/             # 路由配置
│   │   └── index.ts
│   ├── stores/             # Pinia 状态管理
│   │   ├── userStore.ts
│   │   ├── serverStore.ts
│   │   └── ...
│   ├── utils/              # 工具函数
│   │   ├── auth.ts
│   │   ├── request.ts
│   │   └── ...
│   └── views/              # 页面组件
│       ├── auth/
│       ├── dashboard/
│       └── server/
├── public/                 # 静态资源
├── package.json
├── vite.config.ts
└── tsconfig.json
```

### 后端结构

```
backend/
├── controllers/            # 控制器层
│   ├── auth_controller.go
│   ├── server_controller.go
│   └── ...
├── models/                 # 数据模型
│   ├── user.go
│   ├── server.go
│   └── ...
├── services/               # 业务逻辑层
│   ├── alert_service.go
│   └── ...
├── middleware/             # 中间件
│   └── auth.go
├── routes/                 # 路由配置
│   └── routes.go
├── utils/                  # 工具函数
│   ├── jwt.go
│   ├── logger.go
│   └── ...
├── config/                 # 配置管理
│   └── config.go
├── go.mod
├── go.sum
└── main.go
```

### Agent 结构

```
agent/
├── cmd/
│   └── agent/
│       └── main.go         # 入口文件
├── internal/
│   ├── handler/            # 消息处理器
│   │   ├── websocket_handler.go
│   │   └── ...
│   ├── monitor/            # 监控实现
│   │   ├── monitor.go
│   │   ├── docker.go
│   │   └── ...
│   └── server/             # 服务器通信
│       ├── client.go
│       └── ...
├── pkg/                    # 共享包
│   ├── logger/
│   └── version/
├── config/
│   ├── config.go
│   └── agent.yaml
├── go.mod
├── go.sum
└── README.md
```

## 核心功能实现

### 1. 认证系统

#### JWT 认证

```go
// backend/utils/jwt.go
func GenerateToken(userID uint, username, role string) (string, error) {
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "user_id":  userID,
        "username": username,
        "role":     role,
        "exp":      time.Now().Add(time.Hour * 24).Unix(),
    })
    
    return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

func ValidateToken(tokenString string) (*jwt.Token, error) {
    return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return []byte(os.Getenv("JWT_SECRET")), nil
    })
}
```

#### 前端认证

```typescript
// frontend/src/utils/auth.ts
export class AuthService {
  private static TOKEN_KEY = 'auth_token'
  
  static setToken(token: string) {
    localStorage.setItem(this.TOKEN_KEY, token)
  }
  
  static getToken(): string | null {
    return localStorage.getItem(this.TOKEN_KEY)
  }
  
  static removeToken() {
    localStorage.removeItem(this.TOKEN_KEY)
  }
  
  static isAuthenticated(): boolean {
    const token = this.getToken()
    if (!token) return false
    
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload.exp > Date.now() / 1000
    } catch {
      return false
    }
  }
}
```

### 2. WebSocket 通信

#### 后端 WebSocket 处理

```go
// backend/controllers/websocket_controller.go
func (w *WebSocketController) HandleWebSocket(c *gin.Context) {
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        log.Printf("WebSocket upgrade error: %v", err)
        return
    }
    defer conn.Close()
    
    client := &WebSocketClient{
        conn:     conn,
        send:     make(chan []byte, 256),
        serverID: serverID,
    }
    
    // 注册客户端
    w.hub.register <- client
    
    // 启动读写协程
    go client.writePump()
    go client.readPump(w.hub)
}
```

#### Agent WebSocket 客户端

```go
// agent/internal/server/client.go
func (c *Client) Connect() error {
    dialer := websocket.Dialer{
        HandshakeTimeout: 10 * time.Second,
    }
    
    conn, _, err := dialer.Dial(c.serverURL, nil)
    if err != nil {
        return fmt.Errorf("websocket dial error: %v", err)
    }
    
    c.conn = conn
    c.connected = true
    
    // 启动消息处理
    go c.readMessages()
    go c.writeMessages()
    
    return nil
}
```

#### 前端 WebSocket 客户端

```typescript
// frontend/src/utils/websocket.ts
export class WebSocketClient {
  private ws: WebSocket | null = null
  private reconnectAttempts = 0
  private maxReconnectAttempts = 5
  
  connect(url: string) {
    this.ws = new WebSocket(url)
    
    this.ws.onopen = () => {
      console.log('WebSocket connected')
      this.reconnectAttempts = 0
    }
    
    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data)
        this.handleMessage(message)
      } catch (error) {
        console.error('WebSocket message parse error:', error)
      }
    }
    
    this.ws.onclose = () => {
      console.log('WebSocket disconnected')
      this.reconnect()
    }
    
    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error)
    }
  }
  
  private reconnect() {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++
      setTimeout(() => this.connect(this.url), 1000 * this.reconnectAttempts)
    }
  }
}
```

### 3. 监控数据收集

#### 系统监控

```go
// agent/internal/monitor/monitor.go
func (m *Monitor) CollectSystemInfo() *SystemInfo {
    // CPU 信息
    cpuInfo, _ := cpu.Info()
    cpuUsage, _ := cpu.Percent(time.Second, false)
    
    // 内存信息
    memInfo, _ := mem.VirtualMemory()
    
    // 磁盘信息
    diskInfo, _ := disk.Usage("/")
    
    // 网络信息
    netInfo, _ := net.IOCounters(false)
    
    // 负载信息
    loadInfo, _ := load.Avg()
    
    return &SystemInfo{
        CPU: CPUInfo{
            Usage:    cpuUsage[0],
            Cores:    len(cpuInfo),
            Model:    cpuInfo[0].ModelName,
        },
        Memory: MemoryInfo{
            Total:       memInfo.Total,
            Used:        memInfo.Used,
            Available:   memInfo.Available,
            UsedPercent: memInfo.UsedPercent,
        },
        Disk: DiskInfo{
            Total:       diskInfo.Total,
            Used:        diskInfo.Used,
            Free:        diskInfo.Free,
            UsedPercent: diskInfo.UsedPercent,
        },
        Network: NetworkInfo{
            BytesRecv: netInfo[0].BytesRecv,
            BytesSent: netInfo[0].BytesSent,
        },
        Load: LoadInfo{
            Load1:  loadInfo.Load1,
            Load5:  loadInfo.Load5,
            Load15: loadInfo.Load15,
        },
    }
}
```

#### Docker 监控

```go
// agent/internal/monitor/docker.go
func (d *DockerMonitor) GetContainers() ([]Container, error) {
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return nil, err
    }
    
    containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
        All: true,
    })
    if err != nil {
        return nil, err
    }
    
    var result []Container
    for _, container := range containers {
        result = append(result, Container{
            ID:      container.ID,
            Name:    container.Names[0],
            Image:   container.Image,
            Status:  container.Status,
            State:   container.State,
            Created: container.Created,
        })
    }
    
    return result, nil
}
```

### 4. 告警系统

#### 告警规则检查

```go
// backend/services/alert_service.go
func (s *AlertService) CheckAlerts(serverID uint, monitorData *MonitorData) {
    alerts, err := s.getAlertSettings(serverID)
    if err != nil {
        return
    }
    
    for _, alert := range alerts {
        if !alert.Enabled {
            continue
        }
        
        triggered := false
        
        switch alert.Type {
        case "cpu":
            triggered = monitorData.CPUUsage > alert.Threshold
        case "memory":
            triggered = monitorData.MemoryUsedPercent > alert.Threshold
        case "disk":
            triggered = monitorData.DiskUsedPercent > alert.Threshold
        }
        
        if triggered {
            s.handleAlert(alert, monitorData)
        }
    }
}
```

#### 通知发送

```go
// backend/services/alert_service.go
func (s *AlertService) sendNotification(alert *Alert, message string) {
    channels, err := s.getNotificationChannels()
    if err != nil {
        return
    }
    
    for _, channel := range channels {
        if !channel.Enabled {
            continue
        }
        
        switch channel.Type {
        case "email":
            s.sendEmail(channel, alert, message)
        case "webhook":
            s.sendWebhook(channel, alert, message)
        case "dingtalk":
            s.sendDingTalk(channel, alert, message)
        }
    }
}
```

## 开发规范

### 1. 代码规范

#### Go 代码规范

```go
// 良好的结构体定义
type Server struct {
    ID            uint      `gorm:"primarykey" json:"id"`
    Name          string    `gorm:"size:100;not null" json:"name"`
    IP            string    `gorm:"size:45;not null" json:"ip"`
    OS            string    `gorm:"size:50" json:"os"`
    Arch          string    `gorm:"size:20" json:"arch"`
    CPUCores      int       `json:"cpu_cores"`
    CPUModel      string    `gorm:"size:200" json:"cpu_model"`
    MemoryTotal   int64     `json:"memory_total"`
    DiskTotal     int64     `json:"disk_total"`
    LastHeartbeat time.Time `json:"last_heartbeat"`
    Online        bool      `gorm:"default:false" json:"online"`
    Status        string    `gorm:"default:'offline'" json:"status"`
    UserID        uint      `gorm:"not null" json:"user_id"`
    Tags          string    `gorm:"size:500" json:"tags"`
    Description   string    `gorm:"size:1000" json:"description"`
    AgentVersion  string    `gorm:"size:20" json:"agent_version"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

#### TypeScript 代码规范

```typescript
// 接口定义
interface ServerInfo {
  id: number
  name: string
  ip: string
  os: string
  arch: string
  cpuCores: number
  cpuModel: string
  memoryTotal: number
  diskTotal: number
  lastHeartbeat: string
  online: boolean
  status: 'online' | 'offline' | 'error'
  userId: number
  tags: string
  description: string
  agentVersion: string
  createdAt: string
  updatedAt: string
}

// 组件定义
export default defineComponent({
  name: 'ServerList',
  props: {
    servers: {
      type: Array as PropType<ServerInfo[]>,
      required: true
    }
  },
  emits: ['refresh', 'delete'],
  setup(props, { emit }) {
    const refreshServers = () => {
      emit('refresh')
    }
    
    const deleteServer = (id: number) => {
      emit('delete', id)
    }
    
    return {
      refreshServers,
      deleteServer
    }
  }
})
```

### 2. Git 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```bash
# 功能开发
git commit -m "feat: 添加服务器监控功能"

# Bug 修复
git commit -m "fix: 修复WebSocket连接断开问题"

# 文档更新
git commit -m "docs: 更新API文档"

# 代码重构
git commit -m "refactor: 重构监控数据收集逻辑"

# 测试相关
git commit -m "test: 添加用户认证测试用例"

# 构建相关
git commit -m "build: 更新Docker构建配置"
```

### 3. 测试规范

#### 单元测试

```go
// backend/controllers/server_controller_test.go
func TestServerController_GetServers(t *testing.T) {
    // 准备测试数据
    db := setupTestDB()
    defer db.Migrator().DropTable(&models.Server{})
    
    controller := &ServerController{DB: db}
    
    // 创建测试服务器
    server := &models.Server{
        Name: "Test Server",
        IP:   "192.168.1.1",
        OS:   "Linux",
    }
    db.Create(server)
    
    // 创建测试请求
    req := httptest.NewRequest("GET", "/api/servers", nil)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    
    // 执行测试
    controller.GetServers(c)
    
    // 验证结果
    assert.Equal(t, 200, w.Code)
    
    var response map[string]interface{}
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(t, err)
    assert.Contains(t, response, "servers")
}
```

#### 集成测试

```go
// backend/integration_test.go
func TestServerRegistration(t *testing.T) {
    // 启动测试服务器
    router := setupRouter()
    ts := httptest.NewServer(router)
    defer ts.Close()
    
    // 测试服务器注册
    payload := map[string]interface{}{
        "name":        "Test Agent",
        "ip":          "192.168.1.100",
        "os":          "Linux",
        "arch":        "amd64",
        "cpu_cores":   4,
        "cpu_model":   "Intel Core i7",
        "memory_total": 8388608,
        "disk_total":  107374182400,
    }
    
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", ts.URL+"/api/servers/register", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Secret-Key", "test-secret-key")
    
    client := &http.Client{}
    resp, err := client.Do(req)
    
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
}
```

#### 前端测试

```typescript
// frontend/src/components/__tests__/ServerList.test.ts
import { mount } from '@vue/test-utils'
import ServerList from '../ServerList.vue'

describe('ServerList', () => {
  const mockServers = [
    {
      id: 1,
      name: 'Test Server',
      ip: '192.168.1.1',
      status: 'online',
      online: true
    }
  ]

  it('renders server list correctly', () => {
    const wrapper = mount(ServerList, {
      props: {
        servers: mockServers
      }
    })

    expect(wrapper.text()).toContain('Test Server')
    expect(wrapper.text()).toContain('192.168.1.1')
  })

  it('emits refresh event when refresh button clicked', async () => {
    const wrapper = mount(ServerList, {
      props: {
        servers: mockServers
      }
    })

    await wrapper.find('.refresh-btn').trigger('click')
    expect(wrapper.emitted('refresh')).toBeTruthy()
  })
})
```

## 调试指南

### 1. 后端调试

#### 使用 VS Code

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Backend",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/backend/main.go",
      "env": {
        "GIN_MODE": "debug",
        "JWT_SECRET": "development_secret"
      },
      "cwd": "${workspaceFolder}/backend"
    }
  ]
}
```

#### 使用 Delve

```bash
# 安装 Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 启动调试
cd backend
dlv debug main.go

# 设置断点
(dlv) break main.main
(dlv) break controllers.(*ServerController).GetServers

# 运行程序
(dlv) continue
```

### 2. 前端调试

#### 浏览器调试

```typescript
// 使用 console.log 调试
console.log('Server data:', servers)

// 使用 debugger 断点
debugger

// 使用 Vue DevTools
// 安装 Vue DevTools 浏览器扩展
```

#### VS Code 调试

```json
// .vscode/launch.json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Chrome",
      "type": "chrome",
      "request": "launch",
      "url": "http://localhost:3000",
      "webRoot": "${workspaceFolder}/frontend/src"
    }
  ]
}
```

### 3. Agent 调试

```bash
# 启用调试日志
./better-monitor-agent -config config/agent.yaml -log-level debug

# 使用 strace 跟踪系统调用
strace -p $(pgrep better-monitor-agent)

# 使用 tcpdump 跟踪网络包
sudo tcpdump -i any -w agent.pcap host your-server-ip
```

## 性能优化

### 1. 前端性能优化

#### 代码分割

```typescript
// router/index.ts
const routes = [
  {
    path: '/dashboard',
    component: () => import('../views/dashboard/index.vue')
  },
  {
    path: '/servers',
    component: () => import('../views/server/ServerList.vue')
  }
]
```

#### 组件缓存

```vue
<!-- 使用 keep-alive 缓存组件 -->
<keep-alive>
  <component :is="currentComponent" />
</keep-alive>
```

#### 虚拟滚动

```vue
<!-- 对于大量数据列表 -->
<virtual-list
  :data="servers"
  :height="400"
  :item-height="60"
  v-slot="{ item }"
>
  <server-item :server="item" />
</virtual-list>
```

### 2. 后端性能优化

#### 数据库优化

```go
// 使用索引
type Server struct {
    ID     uint   `gorm:"primarykey"`
    UserID uint   `gorm:"index"`
    IP     string `gorm:"index"`
    Online bool   `gorm:"index"`
}

// 预加载关联数据
var servers []Server
db.Preload("User").Find(&servers)

// 使用分页
var servers []Server
db.Offset((page - 1) * pageSize).Limit(pageSize).Find(&servers)
```

#### 缓存优化

```go
// 使用 Redis 缓存
func (s *ServerService) GetServers(userID uint) ([]Server, error) {
    cacheKey := fmt.Sprintf("servers:user:%d", userID)
    
    // 尝试从缓存获取
    if cached, err := s.redis.Get(cacheKey).Result(); err == nil {
        var servers []Server
        json.Unmarshal([]byte(cached), &servers)
        return servers, nil
    }
    
    // 从数据库获取
    var servers []Server
    err := s.db.Where("user_id = ?", userID).Find(&servers).Error
    if err != nil {
        return nil, err
    }
    
    // 缓存结果
    data, _ := json.Marshal(servers)
    s.redis.Set(cacheKey, data, time.Hour)
    
    return servers, nil
}
```

### 3. 系统性能优化

#### 连接池优化

```go
// 数据库连接池
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)

// HTTP 客户端连接池
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

#### 监控数据压缩

```go
// 使用 gzip 压缩监控数据
func compressData(data []byte) []byte {
    var buf bytes.Buffer
    writer := gzip.NewWriter(&buf)
    writer.Write(data)
    writer.Close()
    return buf.Bytes()
}
```

## 部署与发布

### 1. 开发环境部署

```bash
# 使用 Docker Compose
docker-compose -f docker-compose.dev.yml up -d

# 或者分别启动各服务
cd backend && go run main.go &
cd frontend && npm run dev &
cd agent && go run cmd/agent/main.go &
```

### 2. 生产环境部署

```bash
# 构建生产镜像
docker-compose -f docker-compose.prod.yml build

# 启动生产服务
docker-compose -f docker-compose.prod.yml up -d

# 监控服务状态
docker-compose -f docker-compose.prod.yml ps
```

### 3. CI/CD 流程

```yaml
# .github/workflows/ci.yml
name: CI/CD Pipeline

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    
    - name: Set up Go
      uses: actions/setup-go@v2
      with:
        go-version: 1.21
    
    - name: Run backend tests
      run: |
        cd backend
        go test ./...
    
    - name: Set up Node.js
      uses: actions/setup-node@v2
      with:
        node-version: '18'
    
    - name: Run frontend tests
      run: |
        cd frontend
        npm ci
        npm run test
  
  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2
    
    - name: Build Docker image
      run: |
        docker build -t better-monitor:${{ github.sha }} .
        docker tag better-monitor:${{ github.sha }} better-monitor:latest
    
    - name: Push to registry
      run: |
        echo ${{ secrets.DOCKER_PASSWORD }} | docker login -u ${{ secrets.DOCKER_USERNAME }} --password-stdin
        docker push better-monitor:${{ github.sha }}
        docker push better-monitor:latest
```

## 常见问题解决

### 1. 编译问题

#### Go 依赖问题

```bash
# 清理模块缓存
go clean -modcache

# 重新下载依赖
go mod download

# 更新依赖
go mod tidy
```

#### 前端依赖问题

```bash
# 删除 node_modules
rm -rf node_modules

# 清理 npm 缓存
npm cache clean --force

# 重新安装
npm install
```

### 2. 运行时问题

#### 端口冲突

```bash
# 查看端口占用
lsof -i :3333
netstat -tulpn | grep 3333

# 杀死占用进程
kill -9 <PID>
```

#### 权限问题

```bash
# 添加用户到 docker 组
sudo usermod -aG docker $USER

# 重新登录使组权限生效
newgrp docker
```

### 3. 调试技巧

#### 日志分析

```bash
# 查看应用日志
tail -f logs/backend.log

# 查看 Docker 日志
docker logs -f better-monitor

# 查看系统日志
journalctl -u better-monitor -f
```

#### 性能分析

```bash
# Go 性能分析
go tool pprof http://localhost:8080/debug/pprof/profile

# 查看内存使用
go tool pprof http://localhost:8080/debug/pprof/heap

# 查看 goroutine
go tool pprof http://localhost:8080/debug/pprof/goroutine
```

## 贡献指南

### 1. 开发流程

1. Fork 项目到个人仓库
2. 创建功能分支：`git checkout -b feature/new-feature`
3. 开发并测试功能
4. 提交代码：`git commit -m "feat: add new feature"`
5. 推送分支：`git push origin feature/new-feature`
6. 创建 Pull Request

### 2. 代码审查

- 确保代码符合项目规范
- 添加必要的测试用例
- 更新相关文档
- 通过 CI/CD 检查

### 3. 版本发布

- 使用语义化版本号
- 更新 CHANGELOG.md
- 创建 Git 标签
- 发布 Docker 镜像

## 参考资料

- [Vue 3 官方文档](https://vuejs.org/)
- [Go 官方文档](https://golang.org/doc/)
- [Gin 框架文档](https://gin-gonic.com/docs/)
- [Element Plus 文档](https://element-plus.org/)
- [Docker 官方文档](https://docs.docker.com/)

---

如有开发相关问题，请查看 [FAQ](../README.md#常见问题) 或提交 [Issue](https://github.com/your-repo/better-monitor/issues)。