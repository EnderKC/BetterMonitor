package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	level Level
	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	error *log.Logger
	fatal *log.Logger
}

// New 创建一个新的日志器
func New(logFile string, level string) (*Logger, error) {
	// 如果指定了日志文件，打开或创建它
	var file *os.File
	var err error

	if logFile != "" {
		// 创建日志目录（如果不存在）
		logDir := filepath.Dir(logFile)
		if _, err := os.Stat(logDir); os.IsNotExist(err) {
			if err := os.MkdirAll(logDir, 0755); err != nil {
				return nil, fmt.Errorf("创建日志目录失败: %w", err)
			}
		}

		// 打开日志文件
		file, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件失败: %w", err)
		}
	}

	// 默认输出到标准输出和文件（如果指定了）
	var output *os.File
	if file == nil {
		output = os.Stdout
	} else {
		// 创建一个多输出器，同时写入标准输出和文件
		output = os.Stdout
		// TODO: 实现多输出
	}

	// 创建日志器
	logger := &Logger{
		level: ParseLevel(level),
		debug: log.New(output, "DEBUG: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		info:  log.New(output, "INFO: ", log.Ldate|log.Ltime|log.Lmicroseconds),
		warn:  log.New(output, "WARN: ", log.Ldate|log.Ltime|log.Lmicroseconds),
		error: log.New(output, "ERROR: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
		fatal: log.New(output, "FATAL: ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile),
	}

	return logger, nil
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
