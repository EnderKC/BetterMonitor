package monitor

import (
	"sync"
	"time"
)

// InstallLogger 安装日志管理器
type InstallLogger struct {
	sessionID   string
	logChan     chan string
	logs        []string
	subscribers []func(string)
	mu          sync.RWMutex
	done        chan struct{}
	completed   bool // 标记安装是否完成
}

var (
	installLoggers = make(map[string]*InstallLogger)
	loggersMu      sync.RWMutex
)

// NewInstallLogger 创建新的安装日志管理器
func NewInstallLogger(sessionID string) *InstallLogger {
	logger := &InstallLogger{
		sessionID:   sessionID,
		logChan:     make(chan string, 100),
		logs:        make([]string, 0),
		subscribers: make([]func(string), 0),
		done:        make(chan struct{}),
	}

	loggersMu.Lock()
	installLoggers[sessionID] = logger
	loggersMu.Unlock()

	go logger.run()
	return logger
}

// Subscribe 订阅日志
func (l *InstallLogger) Subscribe(callback func(string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subscribers = append(l.subscribers, callback)
}

// Log 记录日志
func (l *InstallLogger) Log(message string) {
	select {
	case l.logChan <- message:
	case <-l.done:
		return
	default:
		// 日志通道满，跳过
	}
}

// GetLogs 获取所有日志
func (l *InstallLogger) GetLogs() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// 返回日志副本
	logsCopy := make([]string, len(l.logs))
	copy(logsCopy, l.logs)
	return logsCopy
}

// IsCompleted 检查安装是否完成
func (l *InstallLogger) IsCompleted() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.completed
}

// Close 关闭日志管理器
func (l *InstallLogger) Close() {
	l.mu.Lock()
	l.completed = true
	l.mu.Unlock()

	close(l.done)

	// 不立即删除，让前端可以获取最后的日志
	// 5分钟后自动清理
	go func() {
		time.Sleep(5 * time.Minute)
		loggersMu.Lock()
		delete(installLoggers, l.sessionID)
		loggersMu.Unlock()
	}()
}

func (l *InstallLogger) run() {
	for {
		select {
		case msg := <-l.logChan:
			l.mu.Lock()
			l.logs = append(l.logs, msg)
			l.mu.Unlock()

			l.mu.RLock()
			for _, subscriber := range l.subscribers {
				subscriber(msg)
			}
			l.mu.RUnlock()
		case <-l.done:
			// 处理剩余的日志消息
			for {
				select {
				case msg := <-l.logChan:
					l.mu.Lock()
					l.logs = append(l.logs, msg)
					l.mu.Unlock()

					l.mu.RLock()
					for _, subscriber := range l.subscribers {
						subscriber(msg)
					}
					l.mu.RUnlock()
				default:
					// channel 已空，退出
					return
				}
			}
		}
	}
}

// GetInstallLogger 获取指定会话的日志管理器
func GetInstallLogger(sessionID string) *InstallLogger {
	loggersMu.RLock()
	defer loggersMu.RUnlock()
	return installLoggers[sessionID]
}
