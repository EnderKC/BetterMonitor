//go:build !monitor_only

package server

import (
	"sync"

	"github.com/user/server-ops-agent/internal/monitor"
)

// clientOpsFields 全功能版的操作类字段
type clientOpsFields struct {
	// 容器终端会话
	dockerSessions     map[string]*containerExecSession
	dockerSessionsLock sync.Mutex

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

// initOpsFields 初始化操作类字段
func (c *Client) initOpsFields() {
	c.dockerSessions = make(map[string]*containerExecSession)
}
