//go:build !monitor_only

package server

import (
	"context"
	"io"
	"sync"

	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/pkg/logger"
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

	// 分片上传管理器
	chunkedUploadMgr *ChunkedUploadManager
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
	reader      io.ReadCloser      // 解复用后的日志流
	cancel      context.CancelFunc // 用于取消 Docker SDK 的 Follow 请求
	stopCh      chan struct{}      // 通知读取 goroutine 停止
	containerID string
	manager     *monitor.DockerManager // 持有引用以便关闭时释放
}

type dockerCommandManager interface {
	Close() error
	GetContainers(all bool) ([]monitor.ContainerInfo, error)
	GetContainerLogs(containerID string, tail int) (string, error)
	StartContainer(containerID string) error
	StopContainer(containerID string, timeout int) error
	RestartContainer(containerID string, timeout int) error
	RemoveContainer(containerID string, force bool) error
	CreateContainer(
		name string,
		image string,
		ports []string,
		volumes []string,
		env map[string]string,
		cmd string,
		restart string,
		network string,
	) (string, error)
	GetImages() ([]monitor.ImageInfo, error)
	PullImage(imageRef string) error
	RemoveImage(imageID string, force bool) error
	GetComposes() ([]monitor.ComposeInfo, error)
	GetComposeConfig(projectName string) (string, error)
	ComposeUp(projectName string) error
	ComposeDown(projectName string) error
	RemoveCompose(projectName string) error
	CreateCompose(projectName string, content string) error
}

var newDockerCommandManager = func(log *logger.Logger) (dockerCommandManager, error) {
	return monitor.NewDockerManager(log)
}

// initOpsFields 初始化操作类字段
func (c *Client) initOpsFields() {
	c.dockerSessions = make(map[string]*containerExecSession)
	c.logStreams = make(map[string]*logStreamSession)
	c.chunkedUploadMgr = NewChunkedUploadManager(c.log)
	c.chunkedUploadMgr.StartCleanup()
}

func (c *Client) closeOperationResources() {
	c.closeAllLogStreams()
}
