package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/internal/upgrader"
	"github.com/user/server-ops-agent/pkg/logger"
	"github.com/user/server-ops-agent/pkg/version"
)

// Client 与服务器通信的客户端
type Client struct {
	cfg        *config.Config
	log        *logger.Logger
	httpClient *http.Client
	wsConn     *websocket.Conn
	secretKey  string // 服务器密钥

	// WebSocket连接状态管理
	wsConnected      bool
	wsMutex          sync.Mutex
	wsShutdown       bool
	wsCancel         context.CancelFunc
	reconnectHandler func()

	// WebSocket写入锁，防止并发写入
	wsWriteMutex sync.Mutex // WebSocket写入锁

	// 升级并发保护：同一时间只允许一个升级任务
	upgrading int32

	// 操作类功能字段（通过 build tag 控制）
	clientOpsFields
}

// New 创建一个新的服务器客户端
func New(config *config.Config, log *logger.Logger) *Client {
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 10 * time.Second
	}
	c := &Client{
		cfg: config,
		log: log,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		secretKey: config.SecretKey,
	}
	c.initOpsFields()

	// 将升级相关配置同步到环境变量，供 upgrader 包使用
	if c.cfg.UpdateRepo != "" {
		os.Setenv("BETTER_MONITOR_AGENT_GITHUB_REPO", c.cfg.UpdateRepo)
	}

	return c
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

	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()
	c.wsShutdown = false

	if c.wsCancel != nil {
		c.wsCancel()
		c.wsCancel = nil
	}
	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
	}
	c.wsConnected = false

	hello := c.agentHello()
	websocketURL, err := BuildAgentWebSocketURL(c.cfg.ServerURL, c.cfg.ServerID)
	if err != nil {
		return err
	}
	headers := BuildAgentWebSocketHeaders(c.secretKey, hello)

	conn, response, err := websocket.DefaultDialer.Dial(websocketURL, headers)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}
	if err := c.writeJSONToConnection(conn, hello); err != nil {
		_ = conn.Close()
		return fmt.Errorf("发送Agent握手失败: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.wsConn = conn
	c.wsConnected = true
	c.wsCancel = cancel
	c.log.Info("WebSocket连接成功")

	go c.handleWebSocketMessages(conn, cancel)
	go c.handleAgentHeartbeat(ctx, conn)
	return nil
}

// CloseWebSocket 关闭WebSocket连接
func (c *Client) CloseWebSocket() {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	// 设置关闭标志，停止重连
	c.wsShutdown = true
	if c.wsCancel != nil {
		c.wsCancel()
		c.wsCancel = nil
	}

	if c.wsConn != nil {
		_ = c.wsConn.Close()
		c.wsConn = nil
		c.wsConnected = false
		c.log.Info("WebSocket连接已关闭")
	}
}

// 处理WebSocket消息
func (c *Client) handleWebSocketMessages(conn *websocket.Conn, cancel context.CancelFunc) {
	defer func() {
		cancel()
		_ = conn.Close()
		c.wsMutex.Lock()
		isCurrent := c.wsConn == conn
		if isCurrent {
			c.wsConn = nil
			c.wsConnected = false
			c.wsCancel = nil
		}
		shutdown := c.wsShutdown
		c.wsMutex.Unlock()

		if isCurrent && !shutdown {
			c.triggerReconnect()
		}
	}()

	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
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
		case "agent_upgrade":
			// 处理Agent升级请求 - 委托给 upgrader 包的统一升级流程
			go c.handleAgentUpgrade(msgCopy)

		case "error":
			// Dashboard/Server 可能会返回 error 消息（例如服务端不识别某些响应类型）。
			// 解析并输出可读信息，避免误报"未知类型"。
			var errMsg struct {
				Type      string `json:"type"`
				Message   string `json:"message"`
				Timestamp int64  `json:"timestamp"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(message, &errMsg); err != nil {
				c.log.Warn("收到服务端 error 消息，但解析失败: %v", err)
				continue
			}
			if strings.TrimSpace(errMsg.Message) != "" {
				if errMsg.RequestID != "" {
					c.log.Warn("收到服务端错误: %s (request_id=%s)", errMsg.Message, errMsg.RequestID)
				} else {
					c.log.Warn("收到服务端错误: %s", errMsg.Message)
				}
			} else {
				c.log.Warn("收到服务端 error 消息（无详细信息）")
			}

		default:
			// 将操作类消息和未知消息委托给 handleOperationMessage
			// 该方法在 full 版本中处理所有操作命令，在 monitor 版本中拒绝操作命令
			c.handleOperationMessage(baseMsg.Type, message, msgCopy)
		}
	}
}

func (c *Client) agentHello() AgentHello {
	agentType := strings.ToLower(strings.TrimSpace(version.AgentType))
	if agentType != "full" && agentType != "monitor" {
		agentType = "full"
	}
	return AgentHello{
		Type:                     "agent_hello",
		Version:                  version.Version,
		AgentType:                agentType,
		HeartbeatIntervalSeconds: int(c.effectiveHeartbeatInterval() / time.Second),
		UpgradeRequestID:         "",
	}
}

func (c *Client) effectiveHeartbeatInterval() time.Duration {
	interval := c.cfg.HeartbeatInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	seconds := (interval + time.Second - 1) / time.Second
	if seconds < 1 {
		seconds = 1
	}
	return seconds * time.Second
}

func (c *Client) handleAgentHeartbeat(ctx context.Context, conn *websocket.Conn) {
	err := runAgentHeartbeat(ctx, c.effectiveHeartbeatInterval(), time.Now, func(heartbeat AgentHeartbeat) error {
		return c.writeJSONToConnection(conn, heartbeat)
	})
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	c.log.Warn("Agent心跳发送失败: %v", err)
	_ = conn.Close()
}

// 安全地向当前WebSocket写入JSON数据
func (c *Client) writeJSON(v interface{}) error {
	c.wsMutex.Lock()
	defer c.wsMutex.Unlock()

	conn := c.wsConn
	if conn == nil || !c.wsConnected {
		return fmt.Errorf("WebSocket连接为空")
	}
	if err := c.writeJSONToConnection(conn, v); err != nil {
		if c.wsConn == conn {
			c.wsConnected = false
			c.wsConn = nil
			if c.wsCancel != nil {
				c.wsCancel()
				c.wsCancel = nil
			}
			_ = conn.Close()
		}
		return err
	}
	return nil
}

func (c *Client) writeJSONToConnection(conn *websocket.Conn, v interface{}) error {
	if conn == nil {
		return fmt.Errorf("WebSocket连接为空")
	}
	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()
	return conn.WriteJSON(v)
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

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送WebSocket响应失败: type=%s, requestID=%s, error=%v", responseType, requestID, err)
	}
}

// ensureURLProtocol 确保URL有协议前缀
func ensureURLProtocol(url string) string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return "http://" + url
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
		c.log.Info("检测到Secret Key更新")
		c.secretKey = response.SecretKey
		configChanged = true
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

	if repo := strings.TrimSpace(response.AgentReleaseRepo); repo != "" && repo != c.cfg.UpdateRepo {
		c.log.Info("更新Release仓库: %s -> %s", c.cfg.UpdateRepo, repo)
		c.cfg.UpdateRepo = repo
		// 同步到环境变量，供 upgrader 包的 resolveDownloadURL 使用
		os.Setenv("BETTER_MONITOR_AGENT_GITHUB_REPO", repo)
		configChanged = true
	}

	if ch := strings.TrimSpace(response.AgentReleaseChannel); ch != "" && ch != c.cfg.UpdateChannel {
		c.log.Info("更新Release通道: %s -> %s", c.cfg.UpdateChannel, ch)
		c.cfg.UpdateChannel = ch
		configChanged = true
	}

	if mirror := strings.TrimSpace(response.AgentReleaseMirror); mirror != c.cfg.UpdateMirror {
		c.log.Info("更新Release镜像: %s -> %s", c.cfg.UpdateMirror, mirror)
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

// ─── Agent 升级（统一使用 upgrader 包） ─────────────────────────────────────────

type agentUpgradePayload struct {
	Action          string `json:"action"`
	TargetVersion   string `json:"target_version"`
	Channel         string `json:"channel"`
	ServerID        uint   `json:"server_id"`
	DownloadURL     string `json:"download_url,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	TargetAgentType string `json:"target_agent_type,omitempty"`
}

// handleAgentUpgrade 处理面板端下发的升级指令，委托给 upgrader 包执行
func (c *Client) handleAgentUpgrade(message []byte) {
	c.log.Info("收到Agent升级请求")

	// 并发保护：同一时间只允许一个升级任务
	if !atomic.CompareAndSwapInt32(&c.upgrading, 0, 1) {
		c.log.Warn("升级任务正在进行中，忽略重复请求")
		return
	}
	defer atomic.StoreInt32(&c.upgrading, 0)

	var envelope struct {
		RequestID string          `json:"request_id"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(message, &envelope); err != nil {
		c.log.Error("解析升级消息失败: %v", err)
		return
	}

	requestID := strings.TrimSpace(envelope.RequestID)
	if requestID == "" {
		requestID = fmt.Sprintf("upgrade-%d-%d", c.cfg.ServerID, time.Now().Unix())
	}

	c.sendUpgradeStatus(requestID, "received", "收到升级指令", map[string]interface{}{
		"platform": runtime.GOOS,
		"arch":     runtime.GOARCH,
	})

	var p agentUpgradePayload
	if err := json.Unmarshal(envelope.Payload, &p); err != nil {
		c.sendUpgradeStatus(requestID, "failed", fmt.Sprintf("解析升级 payload 失败: %v", err), nil)
		return
	}

	if p.ServerID == 0 {
		p.ServerID = uint(c.cfg.ServerID)
	}
	if strings.TrimSpace(p.Action) == "" {
		p.Action = "upgrade"
	}
	if p.Action != "upgrade" {
		c.sendUpgradeStatus(requestID, "failed", fmt.Sprintf("不支持的升级动作: %s", p.Action), nil)
		return
	}
	if strings.TrimSpace(p.TargetVersion) == "" {
		c.sendUpgradeStatus(requestID, "failed", "缺少 target_version", nil)
		return
	}
	if strings.TrimSpace(p.Channel) == "" {
		p.Channel = "stable"
	}

	current := version.GetVersion()
	if current != nil && strings.TrimSpace(current.Version) != "" &&
		strings.TrimSpace(current.Version) == strings.TrimSpace(p.TargetVersion) {
		c.sendUpgradeStatus(requestID, "noop", "当前版本已是目标版本，无需升级", map[string]interface{}{
			"current_version": current.Version,
			"target_version":  p.TargetVersion,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	req := upgrader.UpgradeRequest{
		RequestID:       requestID,
		TargetVersion:   strings.TrimSpace(p.TargetVersion),
		Channel:         strings.TrimSpace(p.Channel),
		DownloadURL:     strings.TrimSpace(p.DownloadURL),
		SHA256:          strings.TrimSpace(p.SHA256),
		TargetAgentType: strings.TrimSpace(p.TargetAgentType),
		ServerID:        p.ServerID,
		SecretKey:       c.secretKey,
		Args:            os.Args,
		Env:             os.Environ(),
	}

	c.sendUpgradeStatus(requestID, "starting", "开始执行升级流程", map[string]interface{}{
		"current_version": safeVersion(current),
		"target_version":  req.TargetVersion,
		"channel":         req.Channel,
	})

	err := upgrader.Upgrade(ctx, req, func(pr upgrader.Progress) {
		fields := map[string]interface{}{
			"current_version": safeVersion(version.GetVersion()),
			"target_version":  req.TargetVersion,
			"channel":         req.Channel,
		}
		if pr.DownloadURL != "" {
			fields["download_url"] = pr.DownloadURL
		}
		if pr.SHA256 != "" {
			fields["sha256"] = pr.SHA256
		}
		if pr.BytesDownloaded > 0 {
			fields["bytes_downloaded"] = pr.BytesDownloaded
		}
		c.sendUpgradeStatus(requestID, pr.Status, pr.Message, fields)
	})
	if err != nil {
		c.sendUpgradeStatus(requestID, "failed", fmt.Sprintf("升级失败: %v", err), nil)
		return
	}

	// Upgrade 成功时通常会直接触发进程重启（Unix: exec；Windows: 退出后由 updater 拉起），
	// 不会执行到这里。但若到达此处，仍发送 success 状态。
	c.sendUpgradeStatus(requestID, "success", "升级流程完成", nil)
}

func safeVersion(info *version.Info) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.Version)
}

// sendUpgradeStatus 向面板端发送升级状态消息
func (c *Client) sendUpgradeStatus(requestID, status, message string, extra map[string]interface{}) {
	info := version.GetVersion()

	payload := map[string]interface{}{
		"status":  status,
		"message": message,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"agent": map[string]interface{}{
			"version":    safeVersion(info),
			"commit":     strings.TrimSpace(info.Commit),
			"build_date": strings.TrimSpace(info.BuildDate),
			"go_version": strings.TrimSpace(info.GoVersion),
			"platform":   strings.TrimSpace(info.Platform),
			"arch":       strings.TrimSpace(info.Arch),
		},
	}
	for k, v := range extra {
		payload[k] = v
	}

	msg := map[string]interface{}{
		"type":       "agent_upgrade_status",
		"request_id": requestID,
		"payload":    payload,
	}

	if err := c.writeJSON(msg); err != nil {
		c.log.Error("发送升级状态消息失败: %v", err)
	}
}
