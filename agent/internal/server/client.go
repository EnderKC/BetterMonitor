package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/pkg/logger"
	"github.com/user/server-ops-agent/pkg/version"
)

// Client 与服务器通信的客户端
type Client struct {
	cfg               *config.Config
	log               *logger.Logger
	httpClient        *http.Client
	wsConn            *websocket.Conn
	lastHeartbeatTime time.Time // 添加最后心跳时间字段
	secretKey         string    // 服务器密钥

	// WebSocket连接状态管理
	wsConnected      bool
	wsMutex          sync.Mutex
	wsShutdown       bool
	reconnectHandler func()

	// WebSocket写入锁，防止并发写入
	wsWriteMutex sync.Mutex // WebSocket写入锁

	// 升级配置
	releaseRepo    string
	releaseChannel string
	releaseMirror  string

	// 容器终端会话
	dockerSessions     map[string]*containerExecSession
	dockerSessionsLock sync.Mutex

	// 容器文件管理器临时缓存（按请求周期使用）
	dockerFileManagers sync.Map // key: requestID, value: *ContainerFileManager
}

type containerExecSession struct {
	manager     *monitor.DockerManager
	execID      string
	containerID string
	stopCh      chan struct{}
}

// New 创建一个新的服务器客户端
func New(config *config.Config, log *logger.Logger) *Client {
	return &Client{
		cfg: config,
		log: log,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		secretKey:      config.SecretKey,
		releaseRepo:    config.UpdateRepo,
		releaseChannel: config.UpdateChannel,
		releaseMirror:  config.UpdateMirror,
		dockerSessions: make(map[string]*containerExecSession),
	}
}

// SetReconnectHandler 设置重连回调，供外部统一调度
func (c *Client) SetReconnectHandler(handler func()) {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	c.reconnectHandler = handler
}

func (c *Client) triggerReconnect() {
	c.wsMutex.Lock()
	handler := c.reconnectHandler
	shutdown := c.wsShutdown
	connected := c.wsConnected && c.wsConn != nil
	c.wsMutex.Unlock()

	if shutdown || handler == nil || connected {
		return
	}
	c.log.Debug("请求WebSocket重连")
	handler()
}

// SendHeartbeat 发送心跳消息
func (c *Client) SendHeartbeat() error {
	if c.cfg.ServerID == 0 || c.secretKey == "" {
		return fmt.Errorf("未配置服务器ID或密钥")
	}

	c.log.Debug("通过WebSocket发送心跳...")

	c.wsMutex.Lock()
	wsConnected := c.wsConnected && c.wsConn != nil
	c.wsMutex.Unlock()

	if !wsConnected {
		c.log.Warn("WebSocket未连接，无法发送心跳")
		c.triggerReconnect()
		return fmt.Errorf("WebSocket未连接")
	}

	heartbeatMsg := struct {
		Type      string `json:"type"`
		Timestamp int64  `json:"timestamp"`
		Status    string `json:"status"`
		Version   string `json:"version"`
		IsReply   bool   `json:"is_reply"`
	}{
		Type:      "heartbeat",
		Timestamp: time.Now().Unix(),
		Status:    "online",
		Version:   version.Version,
		IsReply:   false, // 标记为主动发送的心跳，非回复
	}

	if err := c.writeJSON(heartbeatMsg); err != nil {
		c.log.Warn("通过WebSocket发送心跳失败: %v", err)

		// 如果发送失败，标记连接断开并触发重连
		c.wsMutex.Lock()
		c.wsConnected = false
		if c.wsConn != nil {
			c.wsConn.Close()
			c.wsConn = nil
		}
		c.wsMutex.Unlock()

		c.triggerReconnect()

		return fmt.Errorf("WebSocket心跳发送失败: %w", err)
	}

	// 更新最后心跳时间
	c.wsMutex.Lock()
	c.lastHeartbeatTime = time.Now()
	c.wsMutex.Unlock()

	c.log.Debug("通过WebSocket发送心跳成功")
	return nil
}

// SendMonitorData 发送监控数据
func (c *Client) SendMonitorData(data *monitor.MonitorData) error {
	if c.cfg.ServerID == 0 || c.secretKey == "" {
		return fmt.Errorf("未配置服务器ID或密钥")
	}

	c.log.Debug("通过WebSocket发送监控数据...")

	c.wsMutex.Lock()
	wsConnected := c.wsConnected && c.wsConn != nil
	c.wsMutex.Unlock()

	if !wsConnected {
		c.log.Warn("WebSocket未连接，无法发送监控数据")
		c.triggerReconnect()
		return fmt.Errorf("websocket未连接")
	}

	msg := struct {
		Type    string               `json:"type"`
		Payload *monitor.MonitorData `json:"payload"`
	}{
		Type:    "monitor",
		Payload: data,
	}

	if err := c.writeJSON(msg); err != nil {
		c.log.Warn("通过WebSocket发送监控数据失败: %v", err)

		c.wsMutex.Lock()
		c.wsConnected = false
		if c.wsConn != nil {
			c.wsConn.Close()
			c.wsConn = nil
		}
		c.wsMutex.Unlock()

		c.triggerReconnect()

		return fmt.Errorf("websocket监控数据发送失败: %w", err)
	}

	c.log.Debug("通过WebSocket发送监控数据成功")
	return nil
}

// SendSystemInfo 发送系统信息
func (c *Client) SendSystemInfo(info *monitor.SystemInfo) error {
	if c.cfg.ServerID == 0 || c.secretKey == "" {
		return fmt.Errorf("未配置服务器ID或密钥")
	}

	c.log.Debug("通过WebSocket发送系统信息...")

	c.wsMutex.Lock()
	wsConnected := c.wsConnected && c.wsConn != nil
	c.wsMutex.Unlock()

	if !wsConnected {
		c.log.Warn("WebSocket未连接，无法发送系统信息")
		c.triggerReconnect()
		return fmt.Errorf("websocket未连接")
	}

	msg := struct {
		Type    string              `json:"type"`
		Payload *monitor.SystemInfo `json:"payload"`
	}{
		Type:    "system_info",
		Payload: info,
	}

	if err := c.writeJSON(msg); err != nil {
		c.log.Warn("通过WebSocket发送系统信息失败: %v", err)

		c.wsMutex.Lock()
		c.wsConnected = false
		if c.wsConn != nil {
			c.wsConn.Close()
			c.wsConn = nil
		}
		c.wsMutex.Unlock()

		c.triggerReconnect()

		return fmt.Errorf("websocket系统信息发送失败: %w", err)
	}

	c.log.Debug("通过WebSocket发送系统信息成功")
	return nil
}

// ConnectWebSocket 连接WebSocket
func (c *Client) ConnectWebSocket() error {
	if c.cfg.ServerID == 0 || c.secretKey == "" {
		return fmt.Errorf("未配置服务器ID或密钥")
	}

	// 加锁保护连接过程
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	c.wsShutdown = false

	// 如果已经连接，先关闭
	if c.wsConn != nil {
		c.log.Debug("已存在WebSocket连接，先关闭")
		c.wsConn.Close()
		c.wsConn = nil
	}

	c.log.Debug("连接WebSocket...")

	// 获取服务器URL（不带协议前缀）
	serverURL := c.cfg.ServerURL
	serverHost := removeProtocolPrefix(serverURL)

	// 尝试可能的WebSocket URL路径
	paths := []string{
		fmt.Sprintf("/api/servers/%d/ws", c.cfg.ServerID),
		fmt.Sprintf("/servers/%d/ws", c.cfg.ServerID),
		fmt.Sprintf("/api/ws/%d/server", c.cfg.ServerID),
		fmt.Sprintf("/ws/%d/server", c.cfg.ServerID),
	}

	var lastError error
	for _, path := range paths {
		// 构建完整的WebSocket URL
		wsProtocol := "ws://"
		if strings.HasPrefix(c.cfg.ServerURL, "https://") {
			wsProtocol = "wss://"
		}
		url := wsProtocol + serverHost + path + "?token=" + c.secretKey

		c.log.Debug("尝试连接WebSocket: %s", url)

		// 尝试连接
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			c.log.Debug("连接失败: %v，尝试下一个路径", err)
			lastError = err
			continue
		}

		// 如果连接成功
		c.wsConn = conn
		c.wsConnected = true // 设置连接状态
		c.log.Info("WebSocket连接成功: %s", url)

		// 立即发送一次心跳消息，确保服务器收到agent在线状态
		heartbeatMsg := struct {
			Type      string `json:"type"`
			Timestamp int64  `json:"timestamp"`
			Status    string `json:"status"`
			Version   string `json:"version"`
			IsReply   bool   `json:"is_reply"`
		}{
			Type:      "heartbeat",
			Timestamp: time.Now().Unix(),
			Status:    "online",
			Version:   version.Version,
			IsReply:   false, // 标记为主动发送的心跳，非回复
		}

		// 这里我们已经持有wsMutex锁，所以可以直接使用wsConn而不用writeJSON
		// 这样可以避免死锁（因为writeJSON也会尝试获取这个锁）
		if err := c.wsConn.WriteJSON(heartbeatMsg); err != nil {
			c.log.Warn("发送初始心跳消息失败: %v", err)
		} else {
			c.log.Info("发送初始心跳消息成功")
			c.lastHeartbeatTime = time.Now()
		}

		// 开始监听消息
		go c.handleWebSocketMessages()

		return nil
	}

	// 所有路径都失败了
	c.wsConnected = false // 确保连接状态为断开
	return fmt.Errorf("WebSocket连接失败，尝试了所有可能的路径: %w", lastError)
}

// CloseWebSocket 关闭WebSocket连接
func (c *Client) CloseWebSocket() {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	// 设置关闭标志，停止重连
	c.wsShutdown = true

	if c.wsConn != nil {
		c.wsConn.Close()
		c.wsConn = nil
		c.wsConnected = false
		c.log.Info("WebSocket连接已关闭")
	}
}

// 处理WebSocket消息
func (c *Client) handleWebSocketMessages() {
	if c.wsConn == nil {
		return
	}

	defer func() {
		// 连接已关闭，更新状态
		c.wsMutex.Lock()
		c.wsConnected = false
		c.wsMutex.Unlock()

		if !c.wsShutdown {
			c.triggerReconnect()
		}
	}()

	for {
		// 读取消息
		_, message, err := c.wsConn.ReadMessage()
		if err != nil {
			c.log.Error("读取WebSocket消息失败: %v", err)
			break
		}

		// 首先检查是哪种消息类型
		var baseMsg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message, &baseMsg); err != nil {
			c.log.Error("解析基本消息类型失败: %v", err)
			continue
		}

		c.log.Debug("收到WebSocket消息: %s", baseMsg.Type)

		// 根据消息类型使用不同的结构体解析
		// 复制消息内容，因为websocket库会重用缓冲区
		// 对于需要在goroutine中处理的消息，必须复制一份
		msgCopy := make([]byte, len(message))
		copy(msgCopy, message)

		// 根据消息类型使用不同的结构体解析
		switch baseMsg.Type {
		case "heartbeat":
			// 解析并处理心跳消息
			var heartbeatMsg struct {
				Type      string `json:"type"`
				Timestamp int64  `json:"timestamp"`
				Status    string `json:"status,omitempty"`
				IsReply   bool   `json:"is_reply,omitempty"` // 添加标记是否为回复的字段
			}
			if err := json.Unmarshal(message, &heartbeatMsg); err != nil {
				c.log.Error("解析心跳消息失败: %v", err)
				continue
			}

			// 更新最后心跳时间
			c.lastHeartbeatTime = time.Now()

			// 只有当不是回复时才回应心跳
			if !heartbeatMsg.IsReply {
				// 构造响应心跳消息
				replyMsg := struct {
					Type      string `json:"type"`
					Timestamp int64  `json:"timestamp"`
					Status    string `json:"status"`
					Version   string `json:"version"`
					IsReply   bool   `json:"is_reply"` // 添加标记是否为回复的字段
				}{
					Type:      "heartbeat",
					Timestamp: time.Now().Unix(),
					Status:    "online",
					Version:   version.Version,
					IsReply:   true, // 标记为回复
				}

				// 发送响应心跳
				c.wsWriteMutex.Lock()
				if err := c.wsConn.WriteJSON(replyMsg); err != nil {
					c.log.Error("发送响应心跳失败: %v", err)
				}
				c.wsWriteMutex.Unlock()
			}

		case "terminal_input":
			// 处理终端输入
			var termMsg struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
				Input     string `json:"input"`
			}
			if err := json.Unmarshal(message, &termMsg); err != nil {
				c.log.Error("解析终端输入消息失败: %v", err)
				continue
			}

			// 处理终端输入
			c.handleTerminalInput(termMsg.SessionID, termMsg.Input)

		case "terminal_resize":
			// 处理终端调整大小
			var resizeMsg struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
				Data      string `json:"data"` // JSON格式的cols和rows
			}
			if err := json.Unmarshal(message, &resizeMsg); err != nil {
				c.log.Error("解析终端调整大小消息失败: %v", err)
				continue
			}

			// 处理终端调整大小
			c.handleTerminalResize(resizeMsg.SessionID, resizeMsg.Data)

		case "terminal_create":
			// 处理创建终端会话
			var createMsg struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(message, &createMsg); err != nil {
				c.log.Error("解析创建终端会话消息失败: %v", err)
				continue
			}

			// 处理创建终端会话
			c.handleTerminalCreate(createMsg.SessionID)

		case "terminal_close":
			// 处理关闭终端会话
			var closeMsg struct {
				Type      string `json:"type"`
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(message, &closeMsg); err != nil {
				c.log.Error("解析关闭终端会话消息失败: %v", err)
				continue
			}

			// 处理关闭终端会话
			c.handleTerminalClose(closeMsg.SessionID)

		case "file_list":
			// 处理文件列表请求 - 异步处理
			go c.handleFileList(msgCopy)

		case "file_content":
			// 处理文件内容请求 - 异步处理
			go c.handleFileContent(msgCopy)

		case "file_upload":
			// 处理文件上传请求 - 异步处理
			go c.handleFileUpload(msgCopy)

		case "docker_file":
			// 处理容器文件操作 - 异步处理
			go c.handleDockerFile(msgCopy)

		case "process_list":
			// 处理进程列表请求 - 异步处理
			go c.handleProcessList(msgCopy)

		case "process_kill":
			// 处理进程终止请求 - 异步处理
			go c.handleProcessKill(msgCopy)

		case "docker_command":
			// 处理Docker命令 - 异步处理
			go c.handleDockerCommand(msgCopy)

		case "nginx_command":
			// 处理Nginx命令 - 异步处理
			go c.handleNginxCommand(msgCopy)

		case "shell_command":
			// 处理Shell命令 - 异步处理
			go c.handleShellCommand(msgCopy)

		case "agent_upgrade":
			// 处理Agent升级请求 - 异步处理
			go c.handleAgentUpgrade(msgCopy)

		default:
			c.log.Warn("收到未知类型的WebSocket消息: %s", baseMsg.Type)
		}
	}
}

// 处理Shell命令
func (c *Client) handleShellCommand(message []byte) {
	c.log.Debug("收到Shell命令请求")

	// 解析命令
	var cmd struct {
		Type    string `json:"type"`
		Payload struct {
			Type        string   `json:"type"`
			Data        string   `json:"data"`
			Session     string   `json:"session"`
			ContainerID string   `json:"container_id,omitempty"`
			Command     []string `json:"command,omitempty"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(message, &cmd); err != nil {
		c.log.Error("解析Shell命令失败: %v", err)
		return
	}

	c.log.Debug("处理Shell命令: 类型=%s, 会话=%s", cmd.Payload.Type, cmd.Payload.Session)

	// 如果指定了容器ID，则使用容器内的 Exec 作为终端
	if cmd.Payload.ContainerID != "" {
		c.handleContainerTerminalCommand(cmd.Payload.ContainerID, cmd.Payload.Session, cmd.Payload.Type, cmd.Payload.Data, cmd.Payload.Command)
		return
	}

	// 根据命令类型处理（宿主机终端）
	switch cmd.Payload.Type {
	case "input":
		c.handleTerminalInput(cmd.Payload.Session, cmd.Payload.Data)
	case "resize":
		c.handleTerminalResize(cmd.Payload.Session, cmd.Payload.Data)
	case "create":
		c.handleTerminalCreate(cmd.Payload.Session)
	case "close":
		c.handleTerminalClose(cmd.Payload.Session)
	case "get_cwd":
		c.handleTerminalGetWorkingDirectory(cmd.Payload.Session)
	default:
		c.log.Warn("未知的Shell命令类型: %s", cmd.Payload.Type)
	}
}

// 处理终端输入
func (c *Client) handleTerminalInput(sessionID, input string) {
	c.log.Debug("处理终端输入: 会话=%s", sessionID)

	// 获取会话，如果不存在则创建
	var session *TerminalSession
	session = GetTerminalSession(sessionID)
	if session == nil {
		var err error
		session, err = StartTerminalSession(sessionID, c.log)
		if err != nil {
			c.log.Error("启动终端会话失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("启动终端会话失败: %v", err))
			return
		}
		// 开始读取输出
		go c.readTerminalOutput(session)
	}

	// 写入输入
	if err := WriteToTerminal(sessionID, input, c.log); err != nil {
		c.log.Error("向终端写入数据失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("向终端写入数据失败: %v", err))
	}
}

// 处理终端大小调整
func (c *Client) handleTerminalResize(sessionID, data string) {
	c.log.Debug("处理终端大小调整: 会话=%s", sessionID)

	// 解析大小数据
	var dimensions struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal([]byte(data), &dimensions); err != nil {
		c.log.Error("解析终端大小数据失败: %v", err)
		return
	}

	// 调整大小
	if err := ResizeTerminal(sessionID, dimensions.Cols, dimensions.Rows, c.log); err != nil {
		c.log.Error("调整终端大小失败: %v", err)
	}
}

// 处理终端创建
func (c *Client) handleTerminalCreate(sessionID string) {
	c.log.Debug("处理终端创建: 会话=%s", sessionID)

	// 检查会话是否已存在
	if session := GetTerminalSession(sessionID); session != nil {
		c.log.Debug("会话已存在，无需创建: %s", sessionID)
		return
	}

	// 创建新会话
	session, err := StartTerminalSession(sessionID, c.log)
	if err != nil {
		c.log.Error("创建终端会话失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("创建终端会话失败: %v", err))
		return
	}

	// 开始读取输出
	go c.readTerminalOutput(session)
}

// 处理终端关闭
func (c *Client) handleTerminalClose(sessionID string) {
	c.log.Debug("处理终端关闭: 会话=%s", sessionID)
	CloseTerminalSession(sessionID, c.log)
}

// 处理获取终端工作目录
func (c *Client) handleTerminalGetWorkingDirectory(sessionID string) {
	c.log.Debug("处理获取终端工作目录: 会话=%s", sessionID)

	// 获取当前工作目录
	workingDir, err := GetTerminalWorkingDirectory(sessionID, c.log)
	if err != nil {
		c.log.Error("获取终端工作目录失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("获取工作目录失败: %v", err))
		return
	}

	// 发送工作目录响应
	response := struct {
		Type       string `json:"type"`
		Session    string `json:"session"`
		WorkingDir string `json:"working_dir"`
	}{
		Type:       "working_directory",
		Session:    sessionID,
		WorkingDir: workingDir,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送工作目录响应失败: %v", err)
	} else {
		c.log.Debug("已发送工作目录响应: 会话=%s, 目录=%s", sessionID, workingDir)
	}
}

func (c *Client) handleContainerTerminalCommand(containerID, sessionID, cmdType, data string, command []string) {
	switch cmdType {
	case "create":
		c.dockerSessionsLock.Lock()
		if _, exists := c.dockerSessions[sessionID]; exists {
			c.dockerSessionsLock.Unlock()
			c.log.Debug("容器终端会话已存在: %s", sessionID)
			return
		}
		c.dockerSessionsLock.Unlock()

		manager, err := monitor.NewDockerManager(c.log)
		if err != nil {
			c.log.Error("创建Docker管理器失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("创建容器终端失败: %v", err))
			return
		}

		execSession, err := manager.StartExecSession(containerID, command)
		if err != nil {
			manager.Close()
			c.log.Error("启动容器终端失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("启动容器终端失败: %v", err))
			return
		}

		sess := &containerExecSession{
			manager:     manager,
			execID:      execSession.ExecID,
			containerID: containerID,
			stopCh:      make(chan struct{}),
		}

		c.dockerSessionsLock.Lock()
		c.dockerSessions[sessionID] = sess
		c.dockerSessionsLock.Unlock()

		c.sendTerminalOutput(sessionID, fmt.Sprintf("已连接到容器 %s\r\n", containerID))
		go c.streamContainerExecOutput(sessionID, sess)

	case "input":
		sess, ok := c.getContainerExecSession(sessionID)
		if !ok {
			c.sendTerminalError(sessionID, "容器终端会话不存在")
			return
		}
		if err := sess.manager.WriteExec(sess.execID, data); err != nil {
			c.log.Error("容器终端写入失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("写入失败: %v", err))
		}

	case "resize":
		var dimensions struct {
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if err := json.Unmarshal([]byte(data), &dimensions); err != nil {
			c.log.Error("解析容器终端大小数据失败: %v", err)
			return
		}
		sess, ok := c.getContainerExecSession(sessionID)
		if !ok {
			return
		}
		if err := sess.manager.ResizeExec(sess.execID, uint(dimensions.Cols), uint(dimensions.Rows)); err != nil {
			c.log.Error("调整容器终端大小失败: %v", err)
		}

	case "close":
		c.closeContainerExecSession(sessionID)

	default:
		c.log.Warn("未知的容器终端命令: %s", cmdType)
	}
}

func (c *Client) streamContainerExecOutput(sessionID string, sess *containerExecSession) {
	reader, err := sess.manager.ExecOutput(sess.execID)
	if err != nil {
		c.log.Error("获取容器输出失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("容器输出失败: %v", err))
		c.closeContainerExecSession(sessionID)
		return
	}

	buffer := make([]byte, 4096)
	for {
		select {
		case <-sess.stopCh:
			return
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			c.sendTerminalOutput(sessionID, string(buffer[:n]))
		}
		if err != nil {
			if err != io.EOF {
				c.log.Error("读取容器输出失败: %v", err)
			}
			c.closeContainerExecSession(sessionID)
			return
		}
	}
}

func (c *Client) getContainerExecSession(sessionID string) (*containerExecSession, bool) {
	c.dockerSessionsLock.Lock()
	defer c.dockerSessionsLock.Unlock()
	sess, ok := c.dockerSessions[sessionID]
	return sess, ok
}

func (c *Client) closeContainerExecSession(sessionID string) {
	c.dockerSessionsLock.Lock()
	sess, ok := c.dockerSessions[sessionID]
	if ok {
		delete(c.dockerSessions, sessionID)
	}
	c.dockerSessionsLock.Unlock()

	if !ok || sess == nil {
		return
	}

	select {
	case <-sess.stopCh:
	default:
		close(sess.stopCh)
	}

	_ = sess.manager.CloseExec(sess.execID)
	_ = sess.manager.Close()

	c.sendTerminalClose(sessionID)
}

// 读取终端输出
func (c *Client) readTerminalOutput(session *TerminalSession) {
	c.log.Debug("开始读取终端输出: 会话=%s", session.ID)

	// 创建Done通道的副本，以便在函数返回后仍能访问
	done := session.Done

	// 创建输出通道和退出通道
	outputChan := make(chan string, 10) // 缓冲区大小为10
	quitChan := make(chan struct{})

	// 启动专用的输出发送goroutine，确保所有websocket写入由同一个goroutine处理
	go func() {
		defer close(quitChan)
		for {
			select {
			case output, ok := <-outputChan:
				if !ok {
					// 通道已关闭，退出
					return
				}
				// 发送输出到客户端
				c.sendTerminalOutput(session.ID, output)
			case <-done:
				// 会话已结束，退出
				return
			}
		}
	}()

	// 检查是否使用PTY
	if session.Pty != nil {
		// 使用PTY时，stdout和stderr合并到一个流
		go func() {
			// 为每个goroutine创建独立的buffer
			buffer := make([]byte, 4096)
			defer close(outputChan) // 读取结束后关闭通道
			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Pty.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取PTY输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()
	} else {
		// 使用标准管道时，分别读取stdout和stderr
		// 读取标准输出
		go func() {
			// 为每个goroutine创建独立的buffer
			buffer := make([]byte, 4096)
			defer func() {
				// 只有当stderr读取也完成时才关闭输出通道
				select {
				case <-done:
					close(outputChan)
				default:
					// stderr读取仍在进行，不关闭通道
				}
			}()

			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Stdout.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取终端标准输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()

		// 读取标准错误
		go func() {
			// 为每个goroutine创建独立的buffer
			buffer := make([]byte, 4096)
			defer func() {
				// 只有当stdout读取也完成时才关闭输出通道
				select {
				case <-done:
					close(outputChan)
				default:
					// stdout读取仍在进行，不关闭通道
				}
			}()

			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Stderr.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取终端标准错误输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()
	}

	// 等待会话结束
	<-done
	c.log.Debug("终端会话已结束: %s", session.ID)

	// 等待输出处理goroutine退出
	<-quitChan

	// 发送会话关闭消息
	c.sendTerminalClose(session.ID)
}

// 安全地向WebSocket写入JSON数据
func (c *Client) writeJSON(v interface{}) error {
	// 使用互斥锁保护WebSocket写入操作
	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	if c.wsConn == nil {
		return fmt.Errorf("WebSocket连接为空")
	}

	return c.wsConn.WriteJSON(v)
}

// 发送终端输出
func (c *Client) sendTerminalOutput(sessionID, output string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端输出")
		return
	}

	// 直接发送标准shell_response格式，不嵌套payload
	response := struct {
		Type    string `json:"type"`    // 顶层消息类型
		Session string `json:"session"` // 会话ID
		Data    string `json:"data"`    // 终端输出数据
	}{
		Type:    "shell_response",
		Session: sessionID,
		Data:    output,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端输出失败: %v", err)
	}
}

// 发送终端错误
func (c *Client) sendTerminalError(sessionID, errMsg string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端错误")
		return
	}

	response := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Error   string `json:"error"`
	}{
		Type:    "shell_error",
		Session: sessionID,
		Error:   errMsg,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端错误失败: %v", err)
	}
}

// 发送终端关闭消息
func (c *Client) sendTerminalClose(sessionID string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端关闭消息")
		return
	}

	response := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Message string `json:"message"`
	}{
		Type:    "shell_close",
		Session: sessionID,
		Message: "终端会话已关闭",
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端关闭消息失败: %v", err)
	}
}

// 处理文件列表请求
func (c *Client) handleFileList(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path string `json:"path"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析文件列表请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到文件列表请求: 路径=%s", msg.Payload.Path)

	// 创建文件管理器
	fileManager := NewFileManager(c.log)

	// 获取文件列表
	files, err := fileManager.ListFiles(msg.Payload.Path)
	if err != nil {
		c.log.Error("获取文件列表失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取文件列表失败: %v", err),
		})
		return
	}

	// 发送响应
	c.sendResponse(msg.RequestID, "file_list_response", map[string]interface{}{
		"path":  msg.Payload.Path,
		"files": files,
	})

	c.log.Info("已发送文件列表响应: %d个文件", len(files))
}

// 处理文件内容请求
func (c *Client) handleFileContent(message []byte) {
	// 解析消息
	var req struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path    string `json:"path"`
			Action  string `json:"action"`
			Content string `json:"content"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &req); err != nil {
		c.log.Error("解析文件内容请求失败: %v", err)
		c.sendResponse(req.RequestID, "error", map[string]interface{}{
			"error": "无效的请求格式",
		})
		return
	}

	c.log.Debug("处理文件内容请求: %s, 路径: %s", req.Payload.Action, req.Payload.Path)

	// 创建文件管理器
	fileManager := NewFileManager(c.log)

	// 根据操作类型处理
	switch req.Payload.Action {
	case "get":
		// 获取文件内容
		content, err := fileManager.GetFileContent(req.Payload.Path)
		if err != nil {
			c.log.Error("获取文件内容失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// 修复：使用file_content_response作为响应类型以匹配后端期望
		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"content": content,
		})
		c.log.Debug("文件内容获取成功: %s (%d字节)", req.Payload.Path, len(content))

	case "save":
		// 保存文件内容
		c.log.Debug("开始保存文件: %s", req.Payload.Path)

		// 添加额外的错误恢复机制
		defer func() {
			if r := recover(); r != nil {
				c.log.Error("保存文件时发生严重错误: %v", r)
				c.sendResponse(req.RequestID, "error", map[string]interface{}{
					"error": fmt.Sprintf("保存文件时发生严重错误: %v", r),
				})
			}
		}()

		// 在保存前备份文件
		backupPath := req.Payload.Path + ".bak"
		if _, err := os.Stat(req.Payload.Path); err == nil {
			// 文件存在，创建备份
			c.log.Debug("创建文件备份: %s -> %s", req.Payload.Path, backupPath)
			backupContent, readErr := os.ReadFile(req.Payload.Path)
			if readErr == nil {
				// 忽略备份错误，不影响主流程
				_ = os.WriteFile(backupPath, backupContent, 0644)
			}
		}

		// 保存文件内容
		if err := fileManager.SaveFileContent(req.Payload.Path, req.Payload.Content); err != nil {
			c.log.Error("保存文件内容失败: %v", err)

			// 尝试恢复备份
			if _, statErr := os.Stat(backupPath); statErr == nil {
				c.log.Info("尝试从备份恢复文件: %s", backupPath)
				if backupContent, readErr := os.ReadFile(backupPath); readErr == nil {
					if writeErr := os.WriteFile(req.Payload.Path, backupContent, 0644); writeErr == nil {
						c.log.Info("成功从备份恢复文件")
					} else {
						c.log.Error("从备份恢复文件失败: %v", writeErr)
					}
				}
			}

			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// 保存成功后删除备份
		if _, err := os.Stat(backupPath); err == nil {
			_ = os.Remove(backupPath)
		}

		c.log.Debug("文件保存成功: %s", req.Payload.Path)
		// 修复：使用file_content_response作为响应类型以匹配后端期望
		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "文件保存成功",
		})

	case "create":
		// 创建文件
		if err := fileManager.CreateFile(req.Payload.Path, req.Payload.Content); err != nil {
			c.log.Error("创建文件失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// 修复：使用file_content_response作为响应类型以匹配后端期望
		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "文件创建成功",
		})

	case "mkdir":
		// 创建目录
		if err := fileManager.CreateDirectory(req.Payload.Path); err != nil {
			c.log.Error("创建目录失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// 修复：使用file_content_response作为响应类型以匹配后端期望
		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "目录创建成功",
		})

	case "tree":
		// 获取目录树，用于目录子节点加载
		depth := 3
		if req.Payload.Content != "" {
			if parsedDepth, err := strconv.Atoi(req.Payload.Content); err == nil && parsedDepth > 0 {
				depth = parsedDepth
			}
		}

		tree, err := fileManager.GetDirectoryTree(req.Payload.Path, depth)
		if err != nil {
			c.log.Error("获取目录树失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		c.sendResponse(req.RequestID, "file_tree_response", map[string]interface{}{
			"path": req.Payload.Path,
			"tree": tree,
		})

	default:
		c.log.Error("未知的文件操作: %s", req.Payload.Action)
		c.sendResponse(req.RequestID, "error", map[string]interface{}{
			"error": "未知的文件操作",
		})
	}
}

// 处理文件上传
func (c *Client) handleFileUpload(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path     string `json:"path"`
			Filename string `json:"filename"`
			Content  string `json:"content"` // Base64编码的文件内容
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析文件上传请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到文件上传请求: 路径=%s, 文件名=%s", msg.Payload.Path, msg.Payload.Filename)

	// 创建文件管理器
	fileManager := NewFileManager(c.log)

	// 上传文件
	err := fileManager.UploadFile(msg.Payload.Path, msg.Payload.Filename, msg.Payload.Content)
	if err != nil {
		c.log.Error("上传文件失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("上传文件失败: %v", err),
		})
		return
	}

	// 发送响应
	c.sendResponse(msg.RequestID, "file_upload_response", map[string]interface{}{
		"path":     msg.Payload.Path,
		"filename": msg.Payload.Filename,
		"success":  true,
		"message":  "文件上传成功",
	})

	c.log.Info("文件已上传: %s/%s", msg.Payload.Path, msg.Payload.Filename)
}

// 处理容器文件操作
func (c *Client) handleDockerFile(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			ContainerID string   `json:"container_id"`
			Path        string   `json:"path"`
			Action      string   `json:"action"`
			Content     string   `json:"content,omitempty"`
			Filename    string   `json:"filename,omitempty"`
			Paths       []string `json:"paths,omitempty"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析容器文件请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的容器文件请求参数",
		})
		return
	}

	if msg.Payload.ContainerID == "" {
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "缺少容器ID",
		})
		return
	}

	manager, err := NewContainerFileManager(c.log, msg.Payload.ContainerID)
	if err != nil {
		c.log.Error("创建容器文件管理器失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("创建容器文件管理器失败: %v", err),
		})
		return
	}
	defer manager.Close()

	switch msg.Payload.Action {
	case "list":
		files, err := manager.ListFiles(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_list", map[string]interface{}{
			"path":  msg.Payload.Path,
			"files": files,
		})

	case "get":
		content, err := manager.GetFileContent(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"content": content,
		})

	case "save":
		if err := manager.SaveFileContent(msg.Payload.Path, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "保存成功",
		})

	case "create":
		if err := manager.CreateFile(msg.Payload.Path, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "创建成功",
		})

	case "mkdir":
		if err := manager.CreateDirectory(msg.Payload.Path); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "目录创建成功",
		})

	case "tree":
		depth := 3
		if msg.Payload.Content != "" {
			if v, err := strconv.Atoi(msg.Payload.Content); err == nil && v > 0 {
				depth = v
			}
		}
		tree, err := manager.GetDirectoryTree(msg.Payload.Path, depth)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_tree", map[string]interface{}{
			"path": msg.Payload.Path,
			"tree": tree,
		})

	case "upload":
		if err := manager.UploadFile(msg.Payload.Path, msg.Payload.Filename, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_upload", map[string]interface{}{
			"path":     msg.Payload.Path,
			"filename": msg.Payload.Filename,
			"success":  true,
		})

	case "download":
		data, err := manager.DownloadFile(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"content": base64.StdEncoding.EncodeToString(data),
		})

	case "delete":
		if len(msg.Payload.Paths) == 0 && msg.Payload.Path != "" {
			msg.Payload.Paths = []string{msg.Payload.Path}
		}
		if err := manager.DeleteFiles(msg.Payload.Paths); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "删除成功",
		})

	default:
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "未知的容器文件操作",
		})
	}
}

// 处理进程列表请求
func (c *Client) handleProcessList(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Action string `json:"action"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析进程列表请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到进程列表请求")

	// 创建进程管理器
	pm := monitor.NewProcessManager(c.log)

	// 获取进程列表
	processes, err := pm.GetProcessList()
	if err != nil {
		c.log.Error("获取进程列表失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取进程列表失败: %v", err),
		})
		return
	}

	// 发送进程列表响应
	c.sendResponse(msg.RequestID, "process_list_response", map[string]interface{}{
		"processes": processes,
		"count":     len(processes),
		"timestamp": time.Now().Unix(),
	})

	c.log.Info("已发送进程列表，共 %d 个进程", len(processes))
}

// 处理进程终止请求
func (c *Client) handleProcessKill(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			PID int32 `json:"pid"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析进程终止请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到进程终止请求: PID=%d", msg.Payload.PID)

	// 创建进程管理器
	pm := monitor.NewProcessManager(c.log)

	// 获取进程信息
	proc, err := pm.GetProcess(msg.Payload.PID)
	if err != nil {
		c.log.Error("获取进程 %d 信息失败: %v", msg.Payload.PID, err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取进程信息失败: %v", err),
		})
		return
	}

	// 终止进程
	if err := pm.KillProcess(msg.Payload.PID); err != nil {
		c.log.Error("终止进程 %d 失败: %v", msg.Payload.PID, err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("终止进程失败: %v", err),
		})
		return
	}

	// 发送响应
	c.sendResponse(msg.RequestID, "process_kill_response", map[string]interface{}{
		"pid":       msg.Payload.PID,
		"name":      proc.Name,
		"success":   true,
		"message":   "进程已成功终止",
		"timestamp": time.Now().Unix(),
	})

	c.log.Info("进程 %d(%s) 已成功终止", msg.Payload.PID, proc.Name)
}

// 处理Docker命令
func (c *Client) handleDockerCommand(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Command string          `json:"command"`
			Action  string          `json:"action"`
			Params  json.RawMessage `json:"params,omitempty"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析Docker命令请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到Docker命令请求: 操作=%s, 命令=%s", msg.Payload.Action, msg.Payload.Command)

	// 创建Docker管理器
	dockerManager, err := monitor.NewDockerManager(c.log)
	if err != nil {
		c.log.Error("创建Docker管理器失败: %v", err)
		c.sendResponse(msg.RequestID, "docker_error", map[string]interface{}{
			"error": fmt.Sprintf("创建Docker管理器失败: %v", err),
		})
		return
	}
	defer dockerManager.Close()

	// 根据不同的命令和操作执行对应的Docker操作
	switch msg.Payload.Command {
	case "containers":
		c.handleContainersCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	case "images":
		c.handleImagesCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	case "composes":
		c.handleComposesCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	default:
		c.log.Error("未知的Docker命令: %s", msg.Payload.Command)
		c.sendResponse(msg.RequestID, "docker_error", map[string]interface{}{
			"error": fmt.Sprintf("未知的Docker命令: %s", msg.Payload.Command),
		})
	}
}

// 处理Nginx命令
func (c *Client) handleNginxCommand(message []byte) {
	var msg struct {
		RequestID string                 `json:"request_id"`
		Payload   map[string]interface{} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析Nginx命令请求失败: %v", err)
		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到Nginx命令请求: RequestID=%s", msg.RequestID)

	// 提取action和其他参数
	action, ok := msg.Payload["action"].(string)
	if !ok {
		c.log.Error("Nginx命令请求缺少action字段")
		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": "请求缺少action字段",
		})
		return
	}

	action = strings.TrimSpace(strings.ToLower(action))

	c.log.Info("处理Nginx命令: %s", action)

	// 调用nginx_monitor.go中的处理函数
	result, err := monitor.HandleNginxCommand(action, msg.Payload)
	if err != nil {
		c.log.Error("执行Nginx命令失败: %v", err)

		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	c.log.Info("Nginx命令执行成功: %s", action)
	c.log.Debug("Nginx命令执行结果: %s", result)

	// 始终使用sendRawResponse发送响应，确保类型为nginx_success
	c.sendRawResponse(msg.RequestID, "nginx_success", result)
}

// sendRawResponse 发送原始响应，不包装result字段
func (c *Client) sendRawResponse(requestID, responseType, jsonData string) {
	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	// 确保响应类型正确，统一使用nginx_success或nginx_error
	if responseType == "success" && requestID != "" {
		responseType = "nginx_success"
	} else if responseType == "error" && requestID != "" {
		responseType = "nginx_error"
	}

	c.log.Debug("发送原始响应，类型: %s, 请求ID: %s", responseType, requestID)

	// 构造完整的响应结构
	response := struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}{
		Type:      responseType,
		RequestID: requestID,
		Data:      json.RawMessage(jsonData),
	}

	if c.wsConn != nil {
		if err := c.wsConn.WriteJSON(response); err != nil {
			c.log.Error("发送WebSocket响应失败: %v", err)
		}
	} else {
		c.log.Error("WebSocket连接未建立，无法发送响应")
	}
}

// sendResponse 发送WebSocket响应
func (c *Client) sendResponse(requestID, responseType string, data map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("发送响应时panic: %v", r)
		}
	}()

	response := map[string]interface{}{
		"type":       responseType,
		"request_id": requestID,
		"data":       data,
	}

	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	if c.wsConn != nil {
		if err := c.wsConn.WriteJSON(response); err != nil {
			c.log.Error("发送WebSocket响应失败: type=%s, requestID=%s, error=%v", responseType, requestID, err)
		}
	} else {
		c.log.Error("WebSocket连接未建立，无法发送响应")
	}
}

// RegisterAgent 向服务端注册 Agent
func (c *Client) RegisterAgent(token string) (uint, string, error) {
	if token == "" {
		return 0, "", fmt.Errorf("注册令牌不能为空")
	}

	c.log.Debug("开始注册 Agent...")

	// 确保URL格式正确
	serverURL := ensureURLProtocol(c.cfg.ServerURL)
	url := fmt.Sprintf("%s/api/agent/register", serverURL)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return 0, "", fmt.Errorf("创建请求失败: %w", err)
	}

	// 添加注册令牌
	req.Header.Set("X-Register-Token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("服务器返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Message   string `json:"message"`
		ServerID  uint   `json:"server_id"`
		SecretKey string `json:"secret_key"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("解析响应失败: %w", err)
	}

	c.log.Info("Agent 注册成功，服务器ID: %d", result.ServerID)
	return result.ServerID, result.SecretKey, nil
}

// removeProtocolPrefix 移除URL中的协议前缀
func removeProtocolPrefix(url string) string {
	// 移除http:// 或 https:// 前缀
	for _, prefix := range []string{"http://", "https://"} {
		if strings.HasPrefix(url, prefix) {
			return url[len(prefix):]
		}
	}
	return url
}

// ensureURLProtocol 确保URL有http://前缀
func ensureURLProtocol(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "http://" + url
	}
	return url
}

// getWSProtocolURL 根据HTTP URL获取对应的WebSocket URL
func getWSProtocolURL(url string) string {
	// 先确保有协议前缀
	urlWithProtocol := ensureURLProtocol(url)

	// 再将HTTP协议转换为WS协议
	if strings.HasPrefix(urlWithProtocol, "https://") {
		return "wss://" + urlWithProtocol[8:]
	} else if strings.HasPrefix(urlWithProtocol, "http://") {
		return "ws://" + urlWithProtocol[7:]
	}

	// 默认使用ws协议
	return "ws://" + url
}

// 终端处理器接口
type TerminalHandler interface {
	StartSession(sessionID string) (*TerminalSession, error)
	GetSession(sessionID string) error
	WriteToTerminal(sessionID string, data string) error
	ResizeTerminal(sessionID string, cols, rows uint16) error
	CloseSession(sessionID string)
}

// RegisterTerminalHandler 注册终端处理器
func (c *Client) RegisterTerminalHandler(handler TerminalHandler) {
	c.log.Info("注册终端处理器")
	// 这里可以保存处理器引用，或者设置回调函数
	// 暂时不实现具体逻辑，我们会直接使用server包中的函数
}

// 处理容器相关命令
func (c *Client) handleContainersCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		// 获取容器列表
		containers, err := dockerManager.GetContainers(true) // 获取所有容器，包括未运行的
		if err != nil {
			c.log.Error("获取容器列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取容器列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_containers", map[string]interface{}{
			"containers": containers,
		})

	case "logs":
		// 获取容器日志
		var logParams struct {
			ContainerID string `json:"container_id"`
			Tail        int    `json:"tail"`
		}
		if err := json.Unmarshal(params, &logParams); err != nil {
			c.log.Error("解析容器日志参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的容器日志参数",
			})
			return
		}

		if logParams.Tail <= 0 {
			logParams.Tail = 100 // 默认获取100行日志
		}

		logs, err := dockerManager.GetContainerLogs(logParams.ContainerID, logParams.Tail)
		if err != nil {
			c.log.Error("获取容器日志失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取容器日志失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_container_logs", map[string]interface{}{
			"logs": logs,
		})

	case "start":
		// 启动容器
		var startParams struct {
			ContainerID string `json:"container_id"`
		}
		if err := json.Unmarshal(params, &startParams); err != nil {
			c.log.Error("解析启动容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的启动容器参数",
			})
			return
		}

		if err := dockerManager.StartContainer(startParams.ContainerID); err != nil {
			c.log.Error("启动容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("启动容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器启动成功",
		})

	case "stop":
		// 停止容器
		var stopParams struct {
			ContainerID string `json:"container_id"`
			Timeout     int    `json:"timeout,omitempty"`
		}
		if err := json.Unmarshal(params, &stopParams); err != nil {
			c.log.Error("解析停止容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的停止容器参数",
			})
			return
		}

		if err := dockerManager.StopContainer(stopParams.ContainerID, stopParams.Timeout); err != nil {
			c.log.Error("停止容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("停止容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器停止成功",
		})

	case "restart":
		// 重启容器
		var restartParams struct {
			ContainerID string `json:"container_id"`
			Timeout     int    `json:"timeout,omitempty"`
		}
		if err := json.Unmarshal(params, &restartParams); err != nil {
			c.log.Error("解析重启容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的重启容器参数",
			})
			return
		}

		if err := dockerManager.RestartContainer(restartParams.ContainerID, restartParams.Timeout); err != nil {
			c.log.Error("重启容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("重启容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器重启成功",
		})

	case "remove":
		// 删除容器
		var removeParams struct {
			ContainerID string `json:"container_id"`
			Force       bool   `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除容器参数",
			})
			return
		}

		if err := dockerManager.RemoveContainer(removeParams.ContainerID, removeParams.Force); err != nil {
			c.log.Error("删除容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器删除成功",
		})

	case "create":
		// 创建容器
		var createParams struct {
			Name    string            `json:"name"`
			Image   string            `json:"image"`
			Ports   []string          `json:"ports"`
			Volumes []string          `json:"volumes"`
			Env     map[string]string `json:"env"`
			Command string            `json:"command"`
			Restart string            `json:"restart"`
			Network string            `json:"network"`
		}
		if err := json.Unmarshal(params, &createParams); err != nil {
			c.log.Error("解析创建容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的创建容器参数",
			})
			return
		}

		// 调用Docker管理器创建容器
		containerID, err := dockerManager.CreateContainer(createParams.Name, createParams.Image,
			createParams.Ports, createParams.Volumes, createParams.Env,
			createParams.Command, createParams.Restart, createParams.Network)

		if err != nil {
			c.log.Error("创建容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("创建容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message":      "容器创建成功",
			"container_id": containerID,
		})

	default:
		c.log.Error("未知的容器操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的容器操作: %s", action),
		})
	}
}

// 处理镜像相关命令
func (c *Client) handleImagesCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		// An List of Docker Images
		images, err := dockerManager.GetImages()
		if err != nil {
			c.log.Error("获取镜像列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取镜像列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_images", map[string]interface{}{
			"images": images,
		})

	case "pull":
		// Pull a Docker Image
		var pullParams struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(params, &pullParams); err != nil {
			c.log.Error("解析拉取镜像参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的拉取镜像参数",
			})
			return
		}

		// 拉取镜像可能耗时较长，应该在后台执行
		go func() {
			if err := dockerManager.PullImage(pullParams.Image); err != nil {
				c.log.Error("拉取镜像失败: %v", err)
				// 可以考虑通过WebSocket推送拉取状态
				return
			}
			c.log.Info("镜像 %s 拉取成功", pullParams.Image)
			// 可以考虑通过WebSocket推送拉取成功的消息
		}()

		// 立即返回正在拉取的响应
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": fmt.Sprintf("正在拉取镜像: %s，请稍后刷新", pullParams.Image),
		})

	case "remove":
		// Remove a Docker Image
		var removeParams struct {
			ImageID string `json:"image_id"`
			Force   bool   `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除镜像参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除镜像参数",
			})
			return
		}

		if err := dockerManager.RemoveImage(removeParams.ImageID, removeParams.Force); err != nil {
			c.log.Error("删除镜像失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除镜像失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "镜像删除成功",
		})

	default:
		c.log.Error("未知的镜像操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的镜像操作: %s", action),
		})
	}
}

// 处理Compose相关命令
func (c *Client) handleComposesCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		// 获取Compose项目列表
		composes, err := dockerManager.GetComposes()
		if err != nil {
			c.log.Error("获取Compose项目列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取Compose项目列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_composes", map[string]interface{}{
			"composes": composes,
		})

	case "up":
		// 启动Compose项目
		var upParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &upParams); err != nil {
			c.log.Error("解析启动Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的启动Compose项目参数",
			})
			return
		}

		if err := dockerManager.ComposeUp(upParams.Name); err != nil {
			c.log.Error("启动Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("启动Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目启动成功",
		})

	case "down":
		// 停止Compose项目
		var downParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &downParams); err != nil {
			c.log.Error("解析停止Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的停止Compose项目参数",
			})
			return
		}

		if err := dockerManager.ComposeDown(downParams.Name); err != nil {
			c.log.Error("停止Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("停止Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目停止成功",
		})

	case "config":
		// 获取Compose项目配置
		var configParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &configParams); err != nil {
			c.log.Error("解析获取Compose配置参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的获取Compose配置参数",
			})
			return
		}

		config, err := dockerManager.GetComposeConfig(configParams.Name)
		if err != nil {
			c.log.Error("获取Compose配置失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取Compose配置失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_compose_config", map[string]interface{}{
			"config": config,
		})

	case "create":
		// 创建Compose项目
		var createParams struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(params, &createParams); err != nil {
			c.log.Error("解析创建Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的创建Compose项目参数",
			})
			return
		}

		if err := dockerManager.CreateCompose(createParams.Name, createParams.Content); err != nil {
			c.log.Error("创建Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("创建Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目创建成功",
		})

	case "remove":
		// 删除Compose项目
		var removeParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除Compose项目参数",
			})
			return
		}

		if err := dockerManager.RemoveCompose(removeParams.Name); err != nil {
			c.log.Error("删除Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目删除成功",
		})

	default:
		c.log.Error("未知的Compose操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的Compose操作: %s", action),
		})
	}
}

// FetchSettings 从服务器获取最新配置
func (c *Client) FetchSettings() error {
	if c.cfg.ServerID == 0 || c.secretKey == "" {
		c.log.Error("未注册的Agent，无法获取配置")
		return fmt.Errorf("未注册的Agent")
	}

	// 构建请求URL
	serverURL := ensureURLProtocol(c.cfg.ServerURL)
	url := fmt.Sprintf("%s/api/servers/%d/settings", serverURL, c.cfg.ServerID)

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 添加认证头
	req.Header.Set("X-Secret-Key", c.secretKey)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误状态码: %d, 响应内容: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		// 服务器返回的配置
		ServerID            uint   `json:"server_id"`
		SecretKey           string `json:"secret_key"`
		HeartbeatInterval   string `json:"heartbeat_interval"`
		MonitorInterval     string `json:"monitor_interval"`
		AgentReleaseRepo    string `json:"agent_release_repo"`
		AgentReleaseChannel string `json:"agent_release_channel"`
		AgentReleaseMirror  string `json:"agent_release_mirror"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查响应状态
	if !response.Success {
		return fmt.Errorf("服务器返回错误: %s", response.Message)
	}

	configChanged := false

	// 更新Secret Key (如果服务器返回了新的值)
	if response.SecretKey != "" && response.SecretKey != c.secretKey {
		c.log.Info("检测到Secret Key更新，旧值: %s, 新值: %s", c.secretKey, response.SecretKey)
		c.secretKey = response.SecretKey
		configChanged = true
	}

	// 解析心跳间隔值
	if response.HeartbeatInterval != "" {
		heartbeatInterval, err := time.ParseDuration(response.HeartbeatInterval)
		if err != nil {
			c.log.Error("解析心跳间隔失败: %s", err)
		} else if heartbeatInterval != c.cfg.HeartbeatInterval {
			c.log.Info("更新心跳间隔: %s -> %s", c.cfg.HeartbeatInterval, heartbeatInterval)
			c.cfg.HeartbeatInterval = heartbeatInterval
			configChanged = true
		}
	} else {
		c.log.Warn("服务器返回的心跳间隔为空")
	}

	// 解析监控间隔值
	if response.MonitorInterval != "" {
		monitorInterval, err := time.ParseDuration(response.MonitorInterval)
		if err != nil {
			c.log.Error("解析监控间隔失败: %s", err)
		} else if monitorInterval != c.cfg.MonitorInterval {
			c.log.Info("更新监控间隔: %s -> %s", c.cfg.MonitorInterval, monitorInterval)
			c.cfg.MonitorInterval = monitorInterval
			configChanged = true
		}
	} else {
		c.log.Warn("服务器返回的监控间隔为空")
	}

	if repo := strings.TrimSpace(response.AgentReleaseRepo); repo != "" && repo != c.releaseRepo {
		c.log.Info("更新Release仓库: %s -> %s", c.releaseRepo, repo)
		c.releaseRepo = repo
		c.cfg.UpdateRepo = repo
		configChanged = true
	}

	if ch := strings.TrimSpace(response.AgentReleaseChannel); ch != "" && ch != c.releaseChannel {
		c.log.Info("更新Release通道: %s -> %s", c.releaseChannel, ch)
		c.releaseChannel = ch
		c.cfg.UpdateChannel = ch
		configChanged = true
	}

	if mirror := strings.TrimSpace(response.AgentReleaseMirror); mirror != c.releaseMirror {
		c.log.Info("更新Release镜像: %s -> %s", c.releaseMirror, mirror)
		c.releaseMirror = mirror
		c.cfg.UpdateMirror = mirror
		configChanged = true
	}

	// 保存更新后的配置
	if configChanged {
		c.log.Info("配置已更新，正在保存...")
		if err := config.SaveConfig(c.cfg, ""); err != nil {
			c.log.Error("保存配置失败: %s", err)
		} else {
			c.log.Info("配置已保存")
		}
	} else {
		c.log.Debug("配置未发生变化，无需更新")
	}

	return nil
}

// IsConnected 检查WebSocket连接是否正常连接
func (c *Client) IsConnected() bool {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	return c.wsConnected && c.wsConn != nil
}

// IsConnectionError 判断错误是否为连接错误
func (c *Client) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// 检查错误是否包含常见的连接错误字符串
	errStr := err.Error()
	connectionErrorStrings := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"closed network connection",
		"use of closed network connection",
		"i/o timeout",
		"EOF",
		"context canceled",
		"no route to host",
		"websocket",
		"websocket: close",
	}

	for _, s := range connectionErrorStrings {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(s)) {
			return true
		}
	}

	// 检查是否为WebSocket关闭错误
	if websocket.IsCloseError(err, websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseNoStatusReceived) {
		return true
	}

	return false
}

// 处理Agent升级请求
func (c *Client) handleAgentUpgrade(message []byte) {
	c.log.Info("收到Agent升级请求")

	var upgradeMsg struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Data      struct {
			TargetVersion string `json:"target_version"`
			Channel       string `json:"channel"`
		} `json:"data"`
	}

	if err := json.Unmarshal(message, &upgradeMsg); err != nil {
		c.log.Error("解析升级消息失败: %v", err)
		return
	}

	c.sendResponse(upgradeMsg.RequestID, "agent_upgrade_response", map[string]interface{}{
		"status":  "started",
		"message": "开始执行升级流程",
	})

	go c.performAgentUpgrade(upgradeMsg.RequestID, upgradeMsg.Data.TargetVersion, upgradeMsg.Data.Channel)
}

// 执行升级流程
func (c *Client) performAgentUpgrade(requestID, targetVersion, channel string) {
	repo := c.getReleaseRepo()
	if repo == "" {
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": "未配置Agent Release仓库",
		})
		return
	}

	asset, err := c.resolveReleaseAsset(repo, targetVersion, channel)
	if err != nil {
		c.log.Error("解析发布信息失败: %v", err)
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": fmt.Sprintf("获取发布信息失败: %v", err),
		})
		return
	}

	c.log.Info("准备下载版本 %s，下载地址: %s", asset.Version, asset.DownloadURL)
	c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
		"status":  "downloading",
		"message": fmt.Sprintf("正在下载版本 %s", asset.Version),
	})

	tempFile, err := c.downloadReleaseFile(asset.DownloadURL)
	if err != nil {
		c.log.Error("下载版本失败: %v", err)
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": fmt.Sprintf("下载失败: %v", err),
		})
		return
	}
	defer os.Remove(tempFile)

	if err := c.verifyReleaseFile(tempFile, asset.Checksum); err != nil {
		c.log.Error("校验下载文件失败: %v", err)
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": fmt.Sprintf("校验失败: %v", err),
		})
		return
	}

	backupFile, err := c.backupCurrentProgram()
	if err != nil {
		c.log.Error("备份当前程序失败: %v", err)
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": fmt.Sprintf("备份失败: %v", err),
		})
		return
	}

	c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
		"status":  "installing",
		"message": "正在安装新版本",
	})

	if err := c.installReleaseFile(tempFile); err != nil {
		c.log.Error("安装新版本失败: %v", err)
		if restoreErr := c.restoreBackup(backupFile); restoreErr != nil {
			c.log.Error("恢复备份失败: %v", restoreErr)
		}
		c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
			"status":  "failed",
			"message": fmt.Sprintf("安装失败: %v", err),
		})
		return
	}

	c.sendResponse(requestID, "agent_upgrade_response", map[string]interface{}{
		"status":  "completed",
		"message": fmt.Sprintf("升级完成，版本 %s 即将生效", asset.Version),
		"version": asset.Version,
	})

	time.Sleep(2 * time.Second)
	c.restartAgent()
}

type releaseAsset struct {
	Version     string
	DownloadURL string
	Checksum    string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (c *Client) resolveReleaseAsset(repo, targetVersion, channel string) (*releaseAsset, error) {
	channel = c.effectiveChannel(channel)
	apiBase := "https://api.github.com"
	tag := strings.TrimSpace(targetVersion)

	var endpoint string
	if tag != "" {
		endpoint = fmt.Sprintf("%s/repos/%s/releases/tags/%s", apiBase, repo, c.formatTag(tag))
	} else if channel == "stable" || channel == "" {
		endpoint = fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	} else {
		endpoint = fmt.Sprintf("%s/repos/%s/releases?per_page=1", apiBase, repo)
	}

	release, err := c.fetchRelease(endpoint, channel != "stable" && channel != "")
	if err != nil {
		return nil, err
	}

	assetURL, err := c.pickAssetURL(release)
	if err != nil {
		return nil, err
	}

	return &releaseAsset{
		Version:     strings.TrimPrefix(release.TagName, "v"),
		DownloadURL: c.applyDownloadMirror(assetURL),
		Checksum:    "",
	}, nil
}

func (c *Client) fetchRelease(endpoint string, isList bool) (*githubRelease, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	if token := c.githubToken(); token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API返回状态码: %d", resp.StatusCode)
	}

	if isList {
		var list []githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("发布列表为空")
		}
		return &list[0], nil
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func (c *Client) pickAssetURL(release *githubRelease) (string, error) {
	if release == nil || len(release.Assets) == 0 {
		return "", fmt.Errorf("发布中没有可用的资产")
	}

	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH
	targetSignature := fmt.Sprintf("%s-%s", targetOS, targetArch)

	var fallback string
	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)
		if strings.Contains(nameLower, targetSignature) {
			return asset.BrowserDownloadURL, nil
		}
		if strings.Contains(nameLower, targetOS) {
			fallback = asset.BrowserDownloadURL
		}
	}

	if fallback != "" {
		return fallback, nil
	}

	return release.Assets[0].BrowserDownloadURL, nil
}

func (c *Client) effectiveChannel(channel string) string {
	val := strings.TrimSpace(channel)
	if val != "" {
		return strings.ToLower(val)
	}
	if c.releaseChannel != "" {
		return strings.ToLower(c.releaseChannel)
	}
	if c.cfg.UpdateChannel != "" {
		return strings.ToLower(c.cfg.UpdateChannel)
	}
	return "stable"
}

func (c *Client) getReleaseRepo() string {
	if c.releaseRepo != "" {
		return c.releaseRepo
	}
	return c.cfg.UpdateRepo
}

func (c *Client) releaseMirrorHost() string {
	if c.releaseMirror != "" {
		return strings.TrimRight(c.releaseMirror, "/")
	}
	if c.cfg.UpdateMirror != "" {
		return strings.TrimRight(c.cfg.UpdateMirror, "/")
	}
	return ""
}

func (c *Client) formatTag(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return version
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func (c *Client) applyDownloadMirror(url string) string {
	mirror := c.releaseMirrorHost()
	if mirror == "" {
		return url
	}
	if strings.HasPrefix(url, "https://github.com") {
		return mirror + strings.TrimPrefix(url, "https://github.com")
	}
	return url
}

func (c *Client) githubToken() string {
	if token := strings.TrimSpace(os.Getenv("AGENT_RELEASE_GITHUB_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
}

// 下载发布文件
func (c *Client) downloadReleaseFile(downloadURL string) (string, error) {
	// 创建临时文件
	tempFile, err := os.CreateTemp("", "ota_update_*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer tempFile.Close()

	// 发送HTTP请求
	resp, err := http.Get(downloadURL)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("下载文件失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("下载请求失败，状态码: %d", resp.StatusCode)
	}

	// 将响应内容写入临时文件
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("写入临时文件失败: %v", err)
	}

	return tempFile.Name(), nil
}

// 验证发布文件
func (c *Client) verifyReleaseFile(filePath string, expectedChecksum string) error {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("文件不存在: %v", err)
	}

	// 检查文件是否为可执行文件
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %v", err)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("文件为空")
	}

	// 计算文件校验和
	if expectedChecksum != "" {
		actualChecksum, err := c.calculateFileChecksum(filePath)
		if err != nil {
			return fmt.Errorf("计算文件校验和失败: %v", err)
		}

		if actualChecksum != expectedChecksum {
			return fmt.Errorf("文件校验和不匹配: 期望 %s, 实际 %s", expectedChecksum, actualChecksum)
		}
		c.log.Info("文件校验和验证成功")
	}

	// 在Unix系统上检查文件权限
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filePath, 0755); err != nil {
			return fmt.Errorf("设置文件权限失败: %v", err)
		}
	}

	return nil
}

// calculateFileChecksum 计算文件校验和
func (c *Client) calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// 备份当前程序
func (c *Client) backupCurrentProgram() (string, error) {
	// 获取当前程序的路径
	currentPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("获取当前程序路径失败: %v", err)
	}

	// 创建备份文件名
	backupPath := currentPath + ".backup"

	// 复制当前程序到备份文件
	sourceFile, err := os.Open(currentPath)
	if err != nil {
		return "", fmt.Errorf("打开当前程序失败: %v", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("创建备份文件失败: %v", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("复制文件失败: %v", err)
	}

	// 设置备份文件权限
	if runtime.GOOS != "windows" {
		if err := os.Chmod(backupPath, 0755); err != nil {
			c.log.Warn("设置备份文件权限失败: %v", err)
		}
	}

	return backupPath, nil
}

// 安装发布文件
func (c *Client) installReleaseFile(tempFile string) error {
	// 获取当前程序的路径
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %v", err)
	}

	// 在Windows上需要特殊处理
	if runtime.GOOS == "windows" {
		// 在Windows上，运行中的程序不能直接替换
		// 需要重命名当前程序，然后复制新程序
		oldPath := currentPath + ".old"
		if err := os.Rename(currentPath, oldPath); err != nil {
			return fmt.Errorf("重命名当前程序失败: %v", err)
		}

		// 复制新程序到当前位置
		if err := c.copyFile(tempFile, currentPath); err != nil {
			// 恢复原程序
			os.Rename(oldPath, currentPath)
			return fmt.Errorf("复制新程序失败: %v", err)
		}

		// 删除旧程序
		os.Remove(oldPath)
	} else {
		// 在Unix系统上，可以直接替换
		if err := c.copyFile(tempFile, currentPath); err != nil {
			return fmt.Errorf("复制新程序失败: %v", err)
		}
	}

	return nil
}

// 恢复备份
func (c *Client) restoreBackup(backupFile string) error {
	// 获取当前程序的路径
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前程序路径失败: %v", err)
	}

	// 复制备份文件到当前位置
	return c.copyFile(backupFile, currentPath)
}

// 重启Agent
func (c *Client) restartAgent() {
	c.log.Info("正在重启Agent...")

	// 获取当前程序的路径
	currentPath, err := os.Executable()
	if err != nil {
		c.log.Error("获取当前程序路径失败: %v", err)
		return
	}

	// 获取启动参数
	args := os.Args[1:]

	// 在新进程中启动程序
	cmd := exec.Command(currentPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		c.log.Error("启动新进程失败: %v", err)
		return
	}

	c.log.Info("新进程已启动，PID: %d", cmd.Process.Pid)

	// 当前进程退出
	os.Exit(0)
}

// 复制文件的辅助方法
func (c *Client) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %v", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %v", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 设置文件权限（仅在Unix系统上）
	if runtime.GOOS != "windows" {
		sourceInfo, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("获取源文件权限失败: %v", err)
		}

		if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
			return fmt.Errorf("设置目标文件权限失败: %v", err)
		}
	}

	return nil
}
