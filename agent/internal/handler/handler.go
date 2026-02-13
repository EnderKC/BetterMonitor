package handler

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/internal/server"
	"github.com/user/server-ops-agent/pkg/logger"
)

// App 应用程序结构
type App struct {
	config          *config.Config
	log             *logger.Logger
	client          *server.Client
	monitor         *monitor.Monitor
	ctx             context.Context
	cancel          context.CancelFunc
	stopChan        chan struct{}
	lastMonitorData *monitor.MonitorData
	lastSendTime    time.Time

	// 操作类功能字段（通过 build tag 控制）
	appOpsFields
}

// New 创建一个新的应用程序
func New(configPath string) (*App, error) {
	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// 创建日志器
	log, err := logger.New(cfg.LogFile, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("创建日志器失败: %w", err)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 创建应用程序
	app := &App{
		config:   cfg,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
		stopChan: make(chan struct{}),
	}

	// 创建监控器
	app.monitor = monitor.New(log)

	// 创建服务器客户端
	app.client = server.New(cfg, log)

	return app, nil
}

// Start 启动应用程序
func (a *App) Start() error {
	a.log.Info("启动Agent...")

	// 检查配置
	if a.config.ServerID == 0 || a.config.SecretKey == "" {
		a.log.Warn("未配置服务器ID或密钥，将使用初始化模式")
		return a.startInitMode()
	}

	// 尝试从服务器获取最新配置
	a.log.Info("尝试从服务器获取最新配置...")
	if err := a.client.FetchSettings(); err != nil {
		a.log.Warn("获取配置失败，将使用本地配置: %v", err)
	} else {
		a.log.Info("已获取最新配置")
	}

	// 收集系统信息
	sysInfo, err := a.monitor.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("获取系统信息失败: %w", err)
	}

	// 发送系统信息
	a.log.Info("发送系统信息到服务器...")
	if err := a.client.SendSystemInfo(sysInfo); err != nil {
		a.log.Warn("发送系统信息失败: %v", err)
	} else {
		a.log.Info("系统信息发送成功")

		// 系统信息发送成功后，再次获取配置确保同步
		if err := a.client.FetchSettings(); err != nil {
			a.log.Warn("二次获取配置失败: %v", err)
		}
	}

	// 初始化终端处理
	a.InitTerminalHandling()

	// 尝试连接WebSocket并启动自动重连
	a.log.Info("尝试连接WebSocket并启动自动重连...")
	if err := a.client.ConnectWebSocket(); err != nil {
		a.log.Warn("WebSocket初始连接失败: %v", err)
		a.log.Info("将尝试在后台自动重连")
	}

	// 处理信号
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动监控协程（监控数据上报同时承担心跳功能）
	go a.monitorLoop()

	a.log.Info("Agent已启动")

	// 等待信号
	select {
	case <-signalChan:
		a.log.Info("收到退出信号")
	case <-a.stopChan:
		a.log.Info("收到停止信号")
	}

	// 取消上下文
	a.cancel()

	// 关闭WebSocket连接
	a.client.CloseWebSocket()

	a.log.Info("Agent已停止")
	return nil
}

// Stop 停止应用程序
func (a *App) Stop() {
	close(a.stopChan)
}

// 启动初始化模式
func (a *App) startInitMode() error {
	a.log.Info("进入初始化模式")

	// 获取系统信息
	sysInfo, err := a.monitor.GetSystemInfo()
	if err != nil {
		return fmt.Errorf("获取系统信息失败: %w", err)
	}

	// 打印系统信息
	a.log.Info("系统信息:")
	a.log.Info("  主机名: %s", sysInfo.Hostname)
	a.log.Info("  操作系统: %s %s", sysInfo.Platform, sysInfo.PlatformVersion)
	a.log.Info("  内核版本: %s", sysInfo.KernelVersion)
	a.log.Info("  CPU: %s (%d核)", sysInfo.CPUModel, sysInfo.CPUCores)
	a.log.Info("  内存: %d MB", sysInfo.MemoryTotal/1024/1024)

	// 检查是否有注册令牌
	if a.config.RegisterToken == "" {
		a.log.Info("未配置注册令牌，请在配置文件中设置 register_token 后重启")
		a.log.Info("或者使用以下命令行启动: ./server-ops-agent --register-token=YOUR_TOKEN")
	} else {
		// 尝试使用令牌注册
		a.log.Info("正在尝试使用令牌注册到服务器...")

		serverID, secretKey, err := a.client.RegisterAgent(a.config.RegisterToken)
		if err != nil {
			a.log.Error("注册失败: %v", err)
		} else {
			a.log.Info("注册成功！获取到服务器ID: %d", serverID)

			// 更新配置
			a.config.ServerID = serverID
			a.config.SecretKey = secretKey

			// 保存配置
			if err := config.SaveConfig(a.config, ""); err != nil {
				a.log.Error("保存配置失败: %v", err)
			} else {
				a.log.Info("配置已保存，重新启动应用...")

				// 重新启动应用（不会立即退出，因为还有信号监听）
				return a.Start()
			}
		}
	}

	// 等待退出信号
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	a.log.Info("Agent已停止")
	return nil
}

// 监控循环
func (a *App) monitorLoop() {
	ticker := time.NewTicker(a.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 获取监控数据
			data, err := a.monitor.GetMonitorData()
			if err != nil {
				a.log.Error("获取监控数据失败: %v", err)
				continue
			}

			// 智能上报：检查数据是否显著变化
			shouldSend := false
			now := time.Now()

			// 如果超过60秒未发送，强制发送
			if a.lastMonitorData == nil || now.Sub(a.lastSendTime) > 60*time.Second {
				shouldSend = true
			} else {
				// 检查各项指标变化
				// CPU变化超过1%
				if abs(data.CPUUsage-a.lastMonitorData.CPUUsage) > 1.0 {
					shouldSend = true
				}
				// 内存变化超过10MB
				if abs(float64(data.MemoryUsed)-float64(a.lastMonitorData.MemoryUsed)) > 10*1024*1024 {
					shouldSend = true
				}
				// 磁盘变化超过100MB
				if abs(float64(data.DiskUsed)-float64(a.lastMonitorData.DiskUsed)) > 100*1024*1024 {
					shouldSend = true
				}
				// 网络流量变化超过5KB/s
				if abs(data.NetworkIn-a.lastMonitorData.NetworkIn) > 5*1024 ||
					abs(data.NetworkOut-a.lastMonitorData.NetworkOut) > 5*1024 {
					shouldSend = true
				}
				// 负载变化超过0.1
				if abs(data.LoadAvg1-a.lastMonitorData.LoadAvg1) > 0.1 {
					shouldSend = true
				}
			}

			if shouldSend {
				// 发送监控数据
				if err := a.client.SendMonitorData(data); err != nil {
					a.log.Error("发送监控数据失败: %v", err)
				} else {
					a.lastMonitorData = data
					a.lastSendTime = now
				}
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// SetRegisterToken 设置注册令牌
func (a *App) SetRegisterToken(token string) {
	if token != "" {
		a.config.RegisterToken = token
		a.log.Info("已设置注册令牌")
	}
}

