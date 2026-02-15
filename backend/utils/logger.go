package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	// 默认日志
	DefaultLogger *Logger
	// 终端日志
	TerminalLogger *Logger
	// WebSocket日志
	WebSocketLogger *Logger
	// 系统日志
	SystemLogger *Logger
)

// 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// 日志结构
type Logger struct {
	Level  LogLevel
	logger *log.Logger
	writer *lumberjack.Logger
}

// 初始化日志系统
func InitLoggers() error {
	// 创建日志目录
	logDir := "./logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 创建默认日志
	var err error
	DefaultLogger, err = NewLogger(filepath.Join(logDir, "app.log"), DEBUG)
	if err != nil {
		return err
	}

	// 创建终端日志
	TerminalLogger, err = NewLogger(filepath.Join(logDir, "terminal.log"), DEBUG)
	if err != nil {
		return err
	}

	// 创建WebSocket日志
	WebSocketLogger, err = NewLogger(filepath.Join(logDir, "websocket.log"), DEBUG)
	if err != nil {
		return err
	}

	// 创建系统日志
	SystemLogger, err = NewLogger(filepath.Join(logDir, "system.log"), INFO)
	if err != nil {
		return err
	}

	// 记录启动日志
	SystemLogger.Info("日志系统初始化完成")
	return nil
}

// 创建新的日志实例
// 使用 lumberjack 实现日志自动轮转：单文件最大50MB，保留3个备份，最多保留7天，旧日志自动压缩
func NewLogger(filePath string, level LogLevel) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 使用 lumberjack 实现日志轮转
	writer := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    50,   // 单文件最大 50MB
		MaxBackups: 3,    // 最多保留 3 个旧日志文件
		MaxAge:     7,    // 旧日志最多保留 7 天
		Compress:   true, // 压缩旧日志
		LocalTime:  true, // 使用本地时间命名轮转文件
	}

	// 创建多输出 (同时输出到控制台和文件)
	multiWriter := io.MultiWriter(os.Stdout, writer)
	logger := log.New(multiWriter, "", log.Ldate|log.Ltime|log.Lmicroseconds)

	return &Logger{
		Level:  level,
		logger: logger,
		writer: writer,
	}, nil
}

// Debug级别日志
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.Level <= DEBUG {
		l.logger.Printf("[DEBUG] "+format, v...)
	}
}

// Info级别日志
func (l *Logger) Info(format string, v ...interface{}) {
	if l.Level <= INFO {
		l.logger.Printf("[INFO] "+format, v...)
	}
}

// Warn级别日志
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.Level <= WARN {
		l.logger.Printf("[WARN] "+format, v...)
	}
}

// Error级别日志
func (l *Logger) Error(format string, v ...interface{}) {
	if l.Level <= ERROR {
		l.logger.Printf("[ERROR] "+format, v...)
	}
}

// Fatal级别日志
func (l *Logger) Fatal(format string, v ...interface{}) {
	if l.Level <= FATAL {
		l.logger.Printf("[FATAL] "+format, v...)
		os.Exit(1)
	}
}

// 关闭日志
func (l *Logger) Close() {
	if l.writer != nil {
		l.writer.Close()
	}
}

// 关闭所有日志
func CloseLoggers() {
	if DefaultLogger != nil {
		DefaultLogger.Close()
	}
	if TerminalLogger != nil {
		TerminalLogger.Close()
	}
	if WebSocketLogger != nil {
		WebSocketLogger.Close()
	}
	if SystemLogger != nil {
		SystemLogger.Close()
	}
}

// WebSocket特定日志
func LogWebSocketConnect(path string, serverID uint, sessionID string) {
	if WebSocketLogger != nil {
		WebSocketLogger.Info("WebSocket连接: 路径=%s, 服务器ID=%d, 会话ID=%s", path, serverID, sessionID)
	}
}

func LogWebSocketMessage(serverID uint, messageType string, data string) {
	if WebSocketLogger != nil {
		// 截断过长的数据，以免日志文件过大
		if len(data) > 100 {
			data = data[:100] + "..." // 只记录前100个字符
		}
		WebSocketLogger.Debug("WebSocket消息: 服务器ID=%d, 类型=%s, 数据=%s", serverID, messageType, data)
	}
}

func LogWebSocketError(serverID uint, err error) {
	if WebSocketLogger != nil {
		WebSocketLogger.Error("WebSocket错误: 服务器ID=%d, 错误=%v", serverID, err)
	}
}

// 终端特定日志
func LogTerminalCreate(serverID uint, sessionID string) {
	if TerminalLogger != nil {
		TerminalLogger.Info("创建终端会话: 服务器ID=%d, 会话ID=%s", serverID, sessionID)
	}
}

func LogTerminalCommand(serverID uint, sessionID string, commandType string) {
	if TerminalLogger != nil {
		TerminalLogger.Debug("终端命令: 服务器ID=%d, 会话ID=%s, 类型=%s",
			serverID, sessionID, commandType)
	}
}

func LogTerminalClose(serverID uint, sessionID string) {
	if TerminalLogger != nil {
		TerminalLogger.Info("关闭终端会话: 服务器ID=%d, 会话ID=%s", serverID, sessionID)
	}
}

// 工具函数: 获取当前时间戳字符串
func GetTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}
