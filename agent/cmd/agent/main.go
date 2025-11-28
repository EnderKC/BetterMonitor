package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/internal/server"
	"github.com/user/server-ops-agent/pkg/logger"
	"github.com/user/server-ops-agent/pkg/version"
)

func main() {
	// 定义命令行参数
	var (
		showVersion   bool
		showHelp      bool
		configFile    string
		serverURL     string
		registerToken string
		serverID      uint
		secretKey     string
		logFile       string
		logLevel      string
	)

	// 解析命令行参数
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.BoolVar(&showVersion, "v", false, "显示版本信息(简写)")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息(简写)")
	flag.StringVar(&configFile, "config", "", "指定配置文件路径")
	flag.StringVar(&configFile, "c", "", "指定配置文件路径(简写)")
	flag.StringVar(&serverURL, "server", "", "服务器URL(例如: 127.0.0.1:8080)")
	flag.StringVar(&registerToken, "token", "", "注册令牌")
	flag.UintVar(&serverID, "server-id", 0, "服务器ID")
	flag.StringVar(&secretKey, "secret-key", "", "服务器密钥")
	flag.StringVar(&logFile, "log", "", "日志文件路径")
	flag.StringVar(&logLevel, "level", "", "日志级别(debug, info, warn, error)")

	// 解析命令行参数
	flag.Parse()

	// 处理版本参数
	if showVersion {
		fmt.Printf("Better-Monitor Agent v%s\n", version.Version)
		fmt.Printf("构建日期: %s\n", version.BuildDate)
		fmt.Printf("Go版本: %s\n", version.GetVersion().GoVersion)
		fmt.Printf("平台: %s/%s\n", version.GetVersion().Platform, version.GetVersion().Arch)
		return
	}

	// 处理帮助参数
	if showHelp {
		fmt.Printf("Better-Monitor Agent v%s - 服务器监控代理\n\n", version.Version)
		fmt.Println("使用方法:")
		fmt.Println("  better-monitor-agent                启动监控代理")
		fmt.Println("  better-monitor-agent -version       显示版本信息")
		fmt.Println("  better-monitor-agent -help          显示帮助信息")
		fmt.Println("\n参数:")
		flag.PrintDefaults()
		fmt.Println("\n配置文件:")
		fmt.Println("  /etc/better-monitor/agent.yaml      系统配置文件")
		fmt.Println("  ./agent.yaml                        当前目录配置文件")
		fmt.Println("\n环境变量:")
		fmt.Println("  BM_CONFIG_FILE                      指定配置文件路径")
		fmt.Println("\n更多信息:")
		fmt.Println("  项目地址: https://github.com/user/better-monitor")
		return
	}

	// 处理非flag参数(兼容旧版本命令行)
	if args := flag.Args(); len(args) > 0 {
		switch args[0] {
		case "version":
			fmt.Printf("Better-Monitor Agent v%s\n", version.Version)
			fmt.Printf("构建日期: %s\n", version.BuildDate)
			fmt.Printf("Go版本: %s\n", version.GetVersion().GoVersion)
			fmt.Printf("平台: %s/%s\n", version.GetVersion().Platform, version.GetVersion().Arch)
			return
		case "help":
			fmt.Printf("Better-Monitor Agent v%s - 服务器监控代理\n\n", version.Version)
			fmt.Println("使用方法:")
			fmt.Println("  better-monitor-agent                启动监控代理")
			fmt.Println("  better-monitor-agent version        显示版本信息")
			fmt.Println("  better-monitor-agent help           显示帮助信息")
			fmt.Println("\n参数:")
			flag.PrintDefaults()
			fmt.Println("\n配置文件:")
			fmt.Println("  /etc/better-monitor/agent.yaml      系统配置文件")
			fmt.Println("  ./agent.yaml                        当前目录配置文件")
			fmt.Println("\n环境变量:")
			fmt.Println("  BM_CONFIG_FILE                      指定配置文件路径")
			fmt.Println("\n更多信息:")
			fmt.Println("  项目地址: https://github.com/user/better-monitor")
			return
		default:
			fmt.Printf("未知参数: %s\n", args[0])
			fmt.Println("使用 'better-monitor-agent -help' 查看帮助")
			os.Exit(1)
		}
	}

	// 加载配置
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		panic("加载配置失败: " + err.Error())
	}

	// 应用命令行参数覆盖配置文件
	if serverURL != "" {
		cfg.ServerURL = serverURL
	}
	if registerToken != "" {
		cfg.RegisterToken = registerToken
	}
	if serverID > 0 {
		cfg.ServerID = serverID
	}
	if secretKey != "" {
		cfg.SecretKey = secretKey
	}
	if logFile != "" {
		cfg.LogFile = logFile
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}

	// 初始化日志
	log, err := logger.New(cfg.LogFile, cfg.LogLevel)
	if err != nil {
		panic("初始化日志失败: " + err.Error())
	}
	log.Info("服务器监控Agent启动")

	// 创建服务器客户端
	client := server.New(cfg, log)

	// 创建监控器
	mon := monitor.New(log)

	// 设置服务器URL用于延迟检测
	if cfg.ServerURL != "" {
		// 构建完整的HTTP URL用于ping检测
		serverURL := cfg.ServerURL
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "http://" + serverURL
		}
		mon.SetServerURL(serverURL)
		log.Info("已配置延迟检测目标: %s", serverURL)
	}

	// 创建等待组和停止通道
	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// 创建连接状态通道，用于通知连接状态的变化
	connStatusCh := make(chan bool, 1)
	// 创建重连请求通道，避免与状态通知混用
	reconnectCh := make(chan struct{}, 1)

	notifyConnStatus := func(status bool) bool {
		select {
		case connStatusCh <- status:
			return true
		default:
			return false
		}
	}

	var reconnectPending int32

	notifyReconnect := func() bool {
		if !atomic.CompareAndSwapInt32(&reconnectPending, 0, 1) {
			return false
		}
		select {
		case reconnectCh <- struct{}{}:
			return true
		default:
			atomic.StoreInt32(&reconnectPending, 0)
			return false
		}
	}

	client.SetReconnectHandler(func() {
		if notifyReconnect() {
			log.Debug("Client内部触发重连")
		}
	})

	// 添加连接监控任务
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 初始连接状态
		var connected bool

		// 重试参数
		maxRetries := 10
		baseRetryDelay := 5 * time.Second

		// 首次尝试连接
		tryConnect := func() bool {
			log.Info("尝试建立WebSocket连接...")
			// 如果已经有服务器ID和密钥，尝试连接
			if cfg.ServerID > 0 && cfg.SecretKey != "" {
				if err := client.ConnectWebSocket(); err != nil {
					log.Error("连接WebSocket服务器失败: %s", err)
					return false
				}
				log.Info("WebSocket连接成功")
				return true
			} else if cfg.RegisterToken != "" {
				// 尝试使用注册令牌注册
				serverID, secretKey, err := client.RegisterAgent(cfg.RegisterToken)
				if err != nil {
					log.Error("注册服务器失败: %s", err)
					return false
				}

				log.Info("服务器注册成功，ID: %d", serverID)

				// 更新配置
				cfg.ServerID = serverID
				cfg.SecretKey = secretKey

				if err := config.SaveConfig(cfg, configFile); err != nil {
					log.Error("保存配置失败: %s", err)
				}

				// 连接WebSocket
				if err := client.ConnectWebSocket(); err != nil {
					log.Error("连接WebSocket服务器失败: %s", err)
					return false
				}
				log.Info("WebSocket连接成功")
				return true
			}
			log.Warn("未配置服务器ID和密钥，也未提供注册令牌，无法连接到管理平台")
			return false
		}

		// 首次连接尝试
		connected = tryConnect()

		// 立即将连接状态通知其他goroutine
		if notifyConnStatus(connected) {
			log.Debug("初始连接状态已通知: %v", connected)
		}

		// 创建连接检查定时器
		checkTicker := time.NewTicker(30 * time.Second)
		defer checkTicker.Stop()

		for {
			select {
			case <-checkTicker.C:
				// 定期检查连接状态
				isConn := client.IsConnected()
				if !isConn && connected {
					log.Warn("定时检查: 检测到WebSocket连接已断开，标记为离线")
					connected = false

					// 通知连接状态变化
					if notifyConnStatus(false) {
						log.Debug("已通知其他组件连接状态变为离线")
					}
				} else if isConn && !connected {
					log.Info("定时检查: 检测到WebSocket已连接，标记为在线")
					connected = true

					// 通知连接状态变化
					if notifyConnStatus(true) {
						log.Debug("已通知其他组件连接状态变为在线")
					}
				}

			case <-reconnectCh:
				// 收到重连请求信号
				atomic.StoreInt32(&reconnectPending, 0)
				log.Info("收到重连请求信号")

				if client.IsConnected() {
					log.Debug("当前已处于连接状态，忽略这次重连请求")
					continue
				}

				if connected {
					log.Info("根据请求标记连接为离线并通知其他组件")
					connected = false
					notifyConnStatus(false)
				}

				if !connected {
					log.Info("当前连接状态为离线，开始重连流程")
					// 使用指数退避策略
					for retryAttempt := 0; retryAttempt < maxRetries; retryAttempt++ {
						// 计算退避时间
						delay := baseRetryDelay
						if retryAttempt > 0 {
							delay = baseRetryDelay * time.Duration(1<<uint(retryAttempt))
							if delay > 5*time.Minute {
								delay = 5 * time.Minute // 最大退避时间为5分钟
							}
						}

						log.Info("将在 %v 后尝试第 %d/%d 次重连", delay, retryAttempt+1, maxRetries)

						// 等待退避时间
						select {
						case <-time.After(delay):
							// 继续重试
						case <-stopCh:
							// 收到停止信号
							return
						}

						// 尝试连接
						log.Info("开始第 %d/%d 次重连尝试", retryAttempt+1, maxRetries)
						if tryConnect() {
							// 连接成功
							connected = true
							log.Info("WebSocket重连成功！")
							if notifyConnStatus(true) {
								log.Debug("重连成功后已广播在线状态")
							}

							// 通知其他goroutine连接已恢复
							go func() {
								// 连接恢复后立即发送系统信息
								sysInfo, err := mon.GetSystemInfo()
								if err == nil && cfg.ServerID > 0 && cfg.SecretKey != "" {
									if err := client.SendSystemInfo(sysInfo); err != nil {
										log.Error("发送系统信息失败: %s", err)
									} else {
										log.Info("重连后系统信息已更新")
									}
								}
							}()
							break
						}

						// 连接失败，继续下一次尝试
						log.Warn("第 %d/%d 次重连尝试失败", retryAttempt+1, maxRetries)
					}

					// 达到最大重试次数但仍未连接成功
					if !connected {
						log.Error("达到最大重试次数，暂时放弃重连")
						// 一分钟后再次尝试
						go func() {
							select {
							case <-time.After(1 * time.Minute):
								log.Info("超过最大重试次数后再次尝试重连")
								notifyReconnect() // 触发新一轮重连
							case <-stopCh:
								return
							}
						}()
					}
				}

			case <-stopCh:
				return
			}
		}
	}()

	// 获取系统信息
	sysInfo, err := mon.GetSystemInfo()
	if err != nil {
		log.Error("获取系统信息失败: %s", err)
	} else if cfg.ServerID > 0 && cfg.SecretKey != "" {
		if err := client.SendSystemInfo(sysInfo); err != nil {
			log.Error("发送系统信息失败: %s", err)
		}
	}

	// 创建一个配置更新通道
	configUpdateCh := make(chan struct{}, 1)

	// 启动心跳任务
	wg.Add(1)
	go func() {
		defer wg.Done()
		heartbeatTicker := time.NewTicker(cfg.HeartbeatInterval)
		defer heartbeatTicker.Stop()

		// 心跳失败计数
		var failedHeartbeats int
		maxFailedHeartbeats := 3 // 连续失败3次触发重连

		for {
			select {
			case <-heartbeatTicker.C:
				// 发送心跳
				if cfg.ServerID > 0 && cfg.SecretKey != "" {
					if err := client.SendHeartbeat(); err != nil {
						log.Error("发送心跳失败: %s", err)
						failedHeartbeats++

						// 连续心跳失败达到阈值，触发重连
						if failedHeartbeats >= maxFailedHeartbeats {
							log.Warn("连续 %d 次心跳失败，触发WebSocket重连", failedHeartbeats)
							failedHeartbeats = 0 // 重置失败计数

							// 通知连接状态变化
							if notifyReconnect() {
								log.Debug("已通知连接监控处理重连")
							}
						}
					} else {
						// 心跳成功，重置失败计数
						if failedHeartbeats > 0 {
							log.Debug("心跳恢复正常")
							failedHeartbeats = 0
						}
					}
				}
			case <-configUpdateCh:
				// 重置心跳间隔
				heartbeatTicker.Reset(cfg.HeartbeatInterval)
				log.Info("已更新心跳间隔为: %s", cfg.HeartbeatInterval)
			case <-stopCh:
				return
			}
		}
	}()

	// 启动监控任务
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitorTicker := time.NewTicker(cfg.MonitorInterval)
		defer monitorTicker.Stop()

		for {
			select {
			case <-monitorTicker.C:
				// 收集监控数据
				if cfg.EnableCPUMonitor || cfg.EnableMemMonitor || cfg.EnableDiskMonitor || cfg.EnableNetworkMonitor {
					// 每次上报时重新获取最新监控数据
					data, err := mon.GetMonitorData()
					if err != nil {
						log.Error("收集监控数据失败: %s", err)
						continue
					}

					// 发送监控数据
					if cfg.ServerID > 0 && cfg.SecretKey != "" {
						log.Info("发送最新监控数据（间隔：%s）...", cfg.MonitorInterval)
						if err := client.SendMonitorData(data); err != nil {
							log.Error("发送监控数据失败: %s", err)

							// 如果是连接错误，通知连接状态变化
							if client.IsConnectionError(err) {
								log.Warn("监控数据发送失败可能是由于连接问题，尝试重连")
								if notifyReconnect() {
									log.Debug("已通知连接监控处理重连")
								}
							}
						}
					}
				}
			case <-configUpdateCh:
				// 重置监控间隔
				monitorTicker.Reset(cfg.MonitorInterval)
				log.Info("已更新监控间隔为: %s", cfg.MonitorInterval)

				// 配置更新后立即获取并发送一次最新数据
				if cfg.EnableCPUMonitor || cfg.EnableMemMonitor || cfg.EnableDiskMonitor || cfg.EnableNetworkMonitor {
					if cfg.ServerID > 0 && cfg.SecretKey != "" {
						data, err := mon.GetMonitorData()
						if err != nil {
							log.Error("收集监控数据失败: %s", err)
						} else {
							log.Info("配置更新后立即发送最新监控数据...")
							if err := client.SendMonitorData(data); err != nil {
								log.Error("发送监控数据失败: %s", err)
							}
						}
					}
				}
			case <-stopCh:
				return
			}
		}
	}()

	// 启动配置获取任务
	wg.Add(1)
	go func() {
		defer wg.Done()
		// 修改配置获取频率，从10分钟改为1分钟
		configTicker := time.NewTicker(1 * time.Minute)
		defer configTicker.Stop()

		// 初始状态下立即获取一次
		if cfg.ServerID > 0 && cfg.SecretKey != "" {
			if err := client.FetchSettings(); err != nil {
				log.Error("获取配置失败: %s", err)
			} else {
				// 通知更新配置
				select {
				case configUpdateCh <- struct{}{}:
				default:
					// 通道已满，跳过
				}
			}
		}

		for {
			select {
			case <-configTicker.C:
				if cfg.ServerID > 0 && cfg.SecretKey != "" {
					if err := client.FetchSettings(); err != nil {
						log.Error("获取配置失败: %s", err)
					} else {
						// 通知更新配置
						select {
						case configUpdateCh <- struct{}{}:
						default:
							// 通道已满，跳过
						}
					}
				}
			case <-stopCh:
				return
			}
		}
	}()

	// 处理信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号或错误
	sig := <-sigCh
	log.Info("收到信号: %s，正在关闭...", sig)

	// 关闭通道，通知所有goroutine停止
	close(stopCh)

	// 关闭WebSocket连接
	client.CloseWebSocket()

	// 等待所有goroutine退出
	wg.Wait()
	log.Info("服务器监控Agent已关闭")
}
