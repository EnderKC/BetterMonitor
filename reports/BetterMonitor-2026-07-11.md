# BetterMonitor 2026-07-11 优化与安全整改报告

## 概要

本次工作围绕 Agent 升级与版本契约、Docker 管理、Nginx/网站/证书管理、实时日志生命周期以及前端数据真实性和界面一致性展开。项目不再兼容旧 Agent 请求协议，并继续维持单管理员模式，不引入多用户机制。

## Agent 请求与版本管理

- 建立可取消、带超时和随机请求 ID 的 typed Agent request broker。
- 请求绑定服务器 ID 和允许的响应类型，拒绝错服务器、错类型、重复及晚到响应。
- Agent 断连会使对应 pending 请求立即失败并完成清理。
- 命令不再重复携带 `secret_key`，发送统一使用认证连接的 `SafeConn.WriteJSON`。
- 删除旧裸 WebSocket 连接池、全局写锁、重复响应 handler 和响应回写路径。
- 证书读取、续期以及 Docker/Nginx 操作迁移到统一 broker。
- Agent 升级与发布契约沿用本次工作前已完成的新版本流程，不保留旧版本兼容分支。

## Docker 管理

- Backend 使用严格 DTO、action/response/timeout 表和边界校验。
- 容器、镜像、Compose、tail、timeout、环境变量、端口、卷和 Compose 内容均增加限制。
- 镜像拉取改为同步完成后返回，失败不再被前端误报为成功。
- Agent 远程错误使用稳定错误码和安全消息，不回显 Docker CLI 私密输出。
- 删除浏览器侧旧 `docker_command` WebSocket RPC，管理操作统一使用 REST。
- 实时日志流绑定服务器、浏览器 owner 和 stream ID，拒绝重复 ID、跨服务器消息和非 owner stop。
- 浏览器或 Agent 断开时清理日志流；Agent scanner 可响应 stop，避免缓冲区满后 goroutine 阻塞。

## Nginx、网站、OpenResty 与证书

- 配置和日志访问只接受 32 位十六进制 `config_id` / `log_id`。
- 删除客户端 path、旧 id 和任意路径 fallback。
- 配置及日志列表不再向前端暴露主机绝对路径。
- 配置创建限制在探测到的 Nginx 配置目录，并拒绝目录穿越、符号链接和非普通文件。
- 配置保存采用临时文件、fsync、原子替换、配置测试和失败恢复。
- 域名、通配符、provider、邮箱、webroot、DNS 配置、session ID 和 JSON 内容增加统一边界。
- 网站、OpenResty、SSL 操作全部迁移到 typed broker。
- DNS 凭证、配置正文和命令输出不进入远程错误或完整日志。

## 前端数据与交互

- Docker 镜像拉取等待真实完成，成功后立即刷新镜像列表，删除固定 3 秒延迟。
- OpenResty 安装完成后立即刷新状态和网站列表，删除固定 1 秒延迟。
- Nginx 页面卸载时停止安装日志轮询。
- Docker 日志 WebSocket 使用事件驱动的连接等待，不再使用 100ms interval 轮询。
- 日志 stop 操作幂等，同一 stream ID 最多发送一次 stop。
- 删除前端全部 emoji，包括动态国家旗帜生成；国家信息改为国家代码或文字状态。

## UI 统一与信息密度

- 视觉方向统一为高密度运维控制台，沿用现有蓝色品牌与明暗主题。
- 间距收敛到 8px 网格附近，降低卡片、表格、表单、工具栏和页面区块留白。
- 大圆角收敛为 3–12px 分级，Docker 与 Nginx 页面取消 20px 装饰性圆角。
- 卡片标题、表格单元格、表单项、按钮、输入框、弹窗和抽屉使用统一紧凑尺寸。
- 增加键盘 `focus-visible` 边界，确保紧凑布局下仍有清晰焦点反馈。

## 验证结果

- Frontend：ESLint、Vue TypeScript、35 个 Vitest 测试和 Vite 生产构建通过。
- Backend：`go test ./...`、`go vet ./...`、`go test -race ./...` 通过。
- Agent：`go test ./...`、`go vet ./...`、`go test -race ./...` 通过。
- 前端 emoji Unicode 扫描无残留。
- 旧 Agent broker、Nginx path fallback 和 Docker 固定 3 秒刷新残留检查通过。
- `git diff --check` 通过。

## 已知非阻塞告警

- macOS Go 链接阶段仍会输出既有 `LC_DYSYMTAB` 警告，但测试与 race 均成功退出。
- Vite vendor chunk 约 3.6 MB，超过 1000 kB 告警阈值；本次未做大型依赖拆包，以避免扩大专项范围。
- 未执行真实 Docker、Nginx、OpenResty、ACME、部署或生产迁移操作；验证均使用单元测试、fake transport 和临时资源。

## 版本管理状态

- 工作分支：`codex/quality-agent-upgrade`。
- 工作位于隔离 worktree。
- 本报告生成时改动尚未 push；按任务约定仅创建一个最终本地提交。
