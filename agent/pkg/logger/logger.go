package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Level 日志级别
type Level int

const (
	// DebugLevel 调试级别
	DebugLevel Level = iota
	// InfoLevel 信息级别
	InfoLevel
	// WarnLevel 警告级别
	WarnLevel
	// ErrorLevel 错误级别
	ErrorLevel
	// FatalLevel 致命错误级别
	FatalLevel
)

// 日志级别字符串
var levelNames = map[Level]string{
	DebugLevel: "DEBUG",
	InfoLevel:  "INFO",
	WarnLevel:  "WARN",
	ErrorLevel: "ERROR",
	FatalLevel: "FATAL",
}

// ParseLevel 从字符串解析日志级别
func ParseLevel(level string) Level {
	switch strings.ToLower(level) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// Logger 日志器结构
type Logger struct {
	level  Level
	debug  *log.Logger
	info   *log.Logger
	warn   *log.Logger
	error  *log.Logger
	fatal  *log.Logger
	writer *lumberjack.Logger // 用于关闭时释放资源，仅在启用文件日志时非 nil
}

// New 创建一个新的日志器
// 使用 lumberjack 实现日志自动轮转：单文件最大50MB，保留3个备份，最多保留14天，旧日志自动压缩
func New(logFile string, level string) (*Logger, error) {
	var output io.Writer = os.Stdout
	var lj *lumberjack.Logger

	if logFile != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败: %w", err)
		}

		// 使用 lumberjack 实现日志轮转
		lj = &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    50,   // 单文件最大 50MB
			MaxBackups: 3,    // 最多保留 3 个旧日志文件
			MaxAge:     14,   // 旧日志最多保留 14 天
			Compress:   true, // 压缩旧日志
			LocalTime:  true, // 使用本地时间命名轮转文件
		}

		// 同时写入 stdout 和文件（保持与 systemd/journal 等兼容）
		output = io.MultiWriter(os.Stdout, lj)
	}

	// 创建日志器
	logger := &Logger{
		level:  ParseLevel(level),
		debug:  log.New(output, "DEBUG: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		info:   log.New(output, "INFO: ", log.Ldate|log.Ltime|log.Lmicroseconds),
		warn:   log.New(output, "WARN: ", log.Ldate|log.Ltime|log.Lmicroseconds),
		error:  log.New(output, "ERROR: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		fatal:  log.New(output, "FATAL: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		writer: lj,
	}

	return logger, nil
}

// Close 关闭日志文件
func (l *Logger) Close() {
	if l.writer != nil {
		l.writer.Close()
	}
}

// Debug 输出调试级别日志
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= DebugLevel {
		l.debug.Printf(format, v...)
	}
}

// Info 输出信息级别日志
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= InfoLevel {
		l.info.Printf(format, v...)
	}
}

// Warn 输出警告级别日志
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= WarnLevel {
		l.warn.Printf(format, v...)
	}
}

// Error 输出错误级别日志
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= ErrorLevel {
		l.error.Printf(format, v...)
	}
}

// Fatal 输出致命错误级别日志
func (l *Logger) Fatal(format string, v ...interface{}) {
	if l.level <= FatalLevel {
		l.fatal.Printf(format, v...)
		os.Exit(1)
	}
}
