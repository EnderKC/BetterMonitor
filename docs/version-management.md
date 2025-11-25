# Better Monitor 版本管理指南

## 概述

Better Monitor 采用统一的版本管理机制，通过根目录的 `.env` 文件统一管理 Dashboard 和 Agent 的版本号。这确保了在开发、构建和部署过程中版本信息的一致性。

## 版本管理机制

### 1. 版本配置

版本信息通过根目录的 `.env` 文件配置：

```bash
# .env
VERSION=1.0.5
```

### 2. 版本读取优先级

系统按以下优先级读取版本信息：
1. 环境变量 `VERSION`
2. `.env` 文件中的 `VERSION` 设置
3. 默认值 `"unknown"`

### 3. 版本信息结构

Dashboard 和 Agent 使用统一的版本信息结构：

```go
type Info struct {
    Version   string `json:"version"`
    BuildDate string `json:"buildTime"`
    GoVersion string `json:"goVersion"`
    Platform  string `json:"platform"`
    Arch      string `json:"arch"`
}
```

## 组件版本处理

### Dashboard (Backend)
- 位置: `backend/pkg/version/version.go`
- 从环境变量读取版本信息
- 提供 API 端点获取版本信息
- 支持获取所有 Agent 版本信息

### Agent
- 位置: `agent/pkg/version/version.go`
- 从环境变量读取版本信息
- 在心跳中向 Dashboard 报告版本信息
- 支持 Release 发布渠道的自动升级

### Frontend
- 位置: `frontend/src/utils/version.ts`
- 通过 API 获取 Dashboard 和 Agent 版本信息
- 支持版本信息展示和管理

## 构建和部署

### 本地构建

使用统一的构建脚本：

```bash
# 构建所有组件
./build.sh

# 构建特定组件
./build.sh agent
./build.sh backend
./build.sh frontend
```

构建脚本会：
1. 加载 `.env` 文件
2. 设置版本信息到 LDFLAGS
3. 注入构建时间
4. 编译二进制文件

### Docker 部署

Docker Compose 配置自动使用 `.env` 文件：

```yaml
environment:
  - VERSION=${VERSION:-1.0.5}
```

### 批量构建

使用 Python 构建脚本进行跨平台构建：

```bash
cd Releases
python build.py --version $(grep VERSION ../.env | cut -d'=' -f2)
```

## 版本管理最佳实践

### 1. 版本号规范

使用语义化版本号：
- 主版本号：不兼容的 API 修改
- 次版本号：功能性新增
- 修订号：问题修正

### 2. 版本更新流程

1. 修改 `.env` 文件中的版本号
2. 运行测试验证版本管理机制
3. 构建和部署

### 3. 版本验证

使用提供的测试脚本验证版本管理机制：

```bash
./test-version.sh
```

## API 端点

### 获取 Dashboard 版本
```
GET /api/version
```

### 获取系统信息
```
GET /api/system/info
```

### 获取所有服务器版本
```
GET /api/servers/versions
```

## 生产环境注意事项

1. **版本一致性**: 确保 Dashboard 和 Agent 版本兼容
2. **批量升级**: 使用 Dashboard 的升级任务统一下发版本
3. **版本监控**: 监控各服务器的 Agent 版本状态
4. **回滚策略**: 准备版本回滚方案

## 故障排查

### 版本信息显示为 "unknown"
- 检查 `.env` 文件是否存在
- 确认 `VERSION` 变量是否正确设置
- 验证环境变量是否正确传递

### Agent 版本不更新
- 检查 Agent 心跳机制
- 确认 Dashboard 接收版本信息
- 验证数据库版本字段更新

### 构建时版本信息错误
- 检查构建脚本是否正确加载 `.env`
- 确认 LDFLAGS 设置正确
- 验证版本注入是否成功

## 测试和验证

项目提供了完整的测试机制来验证版本管理：

```bash
# 运行版本管理测试
./test-version.sh

# 检查特定组件版本
./agent/better-monitor-agent -version
./backend/better-monitor-backend -version
```

## 总结

Better Monitor 的版本管理机制提供了：
- 统一的版本配置
- 一致的版本信息结构
- 自动化的构建和部署
- 完整的版本监控和更新机制

这确保了在开发、测试和生产环境中版本信息的准确性和一致性。
