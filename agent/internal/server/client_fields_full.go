//go:build !monitor_only

package server

import (
	"context"
	"io"
	"sync"

	"github.com/user/server-ops-agent/internal/monitor"
)

// clientOpsFields 全功能版的操作类字段
type clientOpsFields struct {
	// 容器终端会话
	dockerSessions     map[string]*containerExecSession
	dockerSessionsLock sync.Mutex

	// 容器日志流会话
	logStreams     map[string]*logStreamSession
	logStreamsLock sync.Mutex

	// 容器文件管理器临时缓存（按请求周期使用）
	dockerFileManagers sync.Map // key: requestID, value: *ContainerFileManager
}

// containerExecSession 容器 exec 会话
type containerExecSession struct {
	manager     *monitor.DockerManager
	execID      string
	containerID string
	stopCh      chan struct{}
}

// logStreamSession 容器日志流会话
type logStreamSession struct {
	reader      io.ReadCloser           // 解复用后的日志流
	cancel      context.CancelFunc      // 用于取消 Docker SDK 的 Follow 请求
	stopCh      chan struct{}            // 通知读取 goroutine 停止
	containerID string
	manager     *monitor.DockerManager  // 持有引用以便关闭时释放
}

// initOpsFields 初始化操作类字段
func (c *Client) initOpsFields() {
	c.dockerSessions = make(map[string]*containerExecSession)
	c.logStreams = make(map[string]*logStreamSession)
}
