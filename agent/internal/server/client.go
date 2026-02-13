package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	cfg        *config.Config
	log        *logger.Logger
	httpClient *http.Client
	wsConn     *websocket.Conn
	secretKey  string // 服务器密钥

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

	// 操作类功能字段（通过 build tag 控制）
	clientOpsFields
}

// New 创建一个新的服务器客户端
func New(config *config.Config, log *logger.Logger) *Client {
	c := &Client{
		cfg: config,
		log: log,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		secretKey:      config.SecretKey,
		releaseRepo:    config.UpdateRepo,
		releaseChannel: config.UpdateChannel,
		releaseMirror:  config.UpdateMirror,
	}
	c.initOpsFields()
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
		case "agent_upgrade":
			// 处理Agent升级请求 - 异步处理
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
	serverURL := ensureURLProtocol(c.cfg.ServerURL)
	url := fmt.Sprintf("%s/api/servers/register", serverURL)

	hostname, _ := os.Hostname()

	payload := struct {
		Token    string `json:"token"`
		Hostname string `json:"hostname"`
	}{
		Token:    token,
		Hostname: hostname,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return 0, "", fmt.Errorf("注册请求失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success  bool   `json:"success"`
		ServerID uint   `json:"server_id"`
		Secret   string `json:"secret_key"`
		Message  string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("解析注册响应失败: %w", err)
	}
	if !result.Success {
		return 0, "", fmt.Errorf("注册失败: %s", result.Message)
	}

	return result.ServerID, result.Secret, nil
}

// removeProtocolPrefix 移除URL的协议前缀
func removeProtocolPrefix(url string) string {
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "wss://")
	url = strings.TrimPrefix(url, "ws://")
	return url
}

// ensureURLProtocol 确保URL有协议前缀
func ensureURLProtocol(url string) string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return "http://" + url
}

// getWSProtocolURL 将HTTP URL转换为WebSocket URL
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
		c.log.Info("检测到Secret Key更新，旧值: %s, 新值: %s", c.secretKey, response.SecretKey)
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
		Payload   struct {
			Action        string `json:"action"`
			TargetVersion string `json:"target_version"`
			Channel       string `json:"channel"`
			ServerID      uint64 `json:"server_id"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &upgradeMsg); err != nil {
		c.log.Error("解析升级消息失败: %v", err)
		return
	}

	channel := c.effectiveChannel(upgradeMsg.Payload.Channel)
	targetVersion := strings.TrimSpace(upgradeMsg.Payload.TargetVersion)

	c.sendResponse(upgradeMsg.RequestID, "agent_upgrade_response", map[string]interface{}{
		"status":         "started",
		"message":        "开始执行升级流程",
		"target_version": targetVersion,
		"channel":        channel,
	})

	go c.performAgentUpgrade(upgradeMsg.RequestID, targetVersion, channel)
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

	// Windows 不能在运行中替换/启动自身（文件锁），需要退出让外部 updater 完成替换并拉起新进程。
	if runtime.GOOS == "windows" {
		time.Sleep(1 * time.Second)
		os.Exit(0)
		return
	}

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

	// 升级类型锁：monitor 版只匹配 monitor 变体资产，full 版排除 monitor 变体
	isMonitor := version.AgentType == "monitor"

	var fallback string
	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)

		// 类型锁过滤: monitor 版只能匹配含 "monitor" 的资产，full 版排除含 "-monitor-" 的资产
		containsMonitor := strings.Contains(nameLower, "-monitor-")
		if isMonitor && !containsMonitor {
			continue
		}
		if !isMonitor && containsMonitor {
			continue
		}

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

	// 如果过滤后没有匹配，说明 Release 中没有对应变体的资产
	return "", fmt.Errorf("未找到适用于 %s 变体的 %s 资产", version.AgentType, targetSignature)
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

	c.log.Info("开始下载升级包: %s", downloadURL)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("创建下载请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "better-monitor-agent")
	req.Header.Set("Accept", "application/octet-stream")

	// 下载升级包可能比普通 API 请求耗时更久，避免复用 10s 的短超时导致在慢网络下升级必然失败。
	downloadClient := &http.Client{Timeout: 20 * time.Minute}
	if c.httpClient != nil {
		clone := *c.httpClient
		clone.Timeout = 20 * time.Minute
		downloadClient = &clone
	}

	// 发送HTTP请求
	resp, err := downloadClient.Do(req)
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

	_ = tempFile.Sync()
	c.log.Info("升级包下载完成: %s", tempFile.Name())

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
		// Windows 下运行中的 exe 被锁定，无法 rename/overwrite。
		// 采用外部 PowerShell updater：等待当前 PID 退出后，再完成替换并拉起新进程。
		if err := c.startWindowsUpdater(currentPath, tempFile); err != nil {
			return fmt.Errorf("启动 updater 失败: %v", err)
		}
	} else {
		// 在Unix系统上，可以直接替换
		if err := c.copyFile(tempFile, currentPath); err != nil {
			return fmt.Errorf("复制新程序失败: %v", err)
		}
	}

	return nil
}

func (c *Client) startWindowsUpdater(oldExePath, newExePath string) error {
	// 仅在 Windows 上调用；为保证跨平台编译，这里不使用 Windows 特有的 syscall 字段。
	argsJSON, _ := json.Marshal(os.Args[1:]) // 不包含 argv[0]

	dir := filepath.Dir(oldExePath)
	scriptPath := filepath.Join(dir, fmt.Sprintf("bm-agent-upgrade-%d.ps1", time.Now().UnixNano()))
	script := strings.TrimSpace(`
param(
  [Parameter(Mandatory=$true)][int]$Pid,
  [Parameter(Mandatory=$true)][string]$OldExe,
  [Parameter(Mandatory=$true)][string]$NewExe,
  [Parameter(Mandatory=$false)][string]$ArgsJson
)

function Try-Remove([string]$Path) {
  try { if (Test-Path $Path) { Remove-Item -Force -ErrorAction SilentlyContinue $Path } } catch {}
}

function Try-Move([string]$From, [string]$To) {
  try { Move-Item -Force -ErrorAction Stop $From $To; return $true } catch { return $false }
}

$args = @()
try {
  if ($ArgsJson -and $ArgsJson.Trim().Length -gt 0) {
    $args = ConvertFrom-Json -InputObject $ArgsJson
  }
} catch {
  $args = @()
}

# wait for old process to exit (max ~120s)
for ($i = 0; $i -lt 120; $i++) {
  try {
    $p = Get-Process -Id $Pid -ErrorAction Stop
    Start-Sleep -Seconds 1
  } catch {
    break
  }
}

$backup = "$OldExe.old"
Try-Remove $backup

# replace: OldExe -> backup, NewExe -> OldExe (retry a few times)
for ($i = 0; $i -lt 30; $i++) {
  try {
    if (Test-Path $OldExe) { Try-Move $OldExe $backup | Out-Null }
    if (Try-Move $NewExe $OldExe) { break }
  } catch {}
  Start-Sleep -Milliseconds 500
}

try {
  Start-Process -FilePath $OldExe -ArgumentList $args -WindowStyle Hidden
} catch {
  # best-effort rollback
  try {
    if (Test-Path $backup) { Try-Move $backup $OldExe | Out-Null }
  } catch {}
}

Try-Remove $NewExe
Try-Remove $MyInvocation.MyCommand.Path
`) + "\r\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("写入 updater 脚本失败: %v", err)
	}

	pid := os.Getpid()
	var lastErr error
	for _, ps := range []string{"powershell.exe", "powershell", "pwsh"} {
		cmd := exec.Command(
			ps,
			"-NoProfile",
			"-ExecutionPolicy",
			"Bypass",
			"-File",
			scriptPath,
			"-Pid",
			strconv.Itoa(pid),
			"-OldExe",
			oldExePath,
			"-NewExe",
			newExePath,
			"-ArgsJson",
			string(argsJSON),
		)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	_ = os.Remove(scriptPath)
	if lastErr != nil {
		return fmt.Errorf("启动 PowerShell 失败: %v", lastErr)
	}
	return fmt.Errorf("启动 PowerShell 失败")
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

	argv := append([]string{currentPath}, os.Args[1:]...)

	// Unix 上优先使用 exec 替换当前进程，避免 systemd/OpenRC 等服务管理器误判为"退出并重启"导致双进程。
	// Windows 不支持该行为（execSelf 会返回错误），会走下方的 fallback。
	if err := execSelf(currentPath, argv); err != nil {
		c.log.Warn("exec 重启失败，尝试启动新进程: %v", err)
	}

	// 在新进程中启动程序
	cmd := exec.Command(currentPath, os.Args[1:]...)
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

	// 直接对正在运行的可执行文件做 os.Create(dst) 可能触发 ETXTBSY（text file busy）。
	// 使用"同目录临时文件 + rename"的方式原子替换，避免写入目标路径。
	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	destFile, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时目标文件失败: %v", err)
	}
	tempPath := destFile.Name()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		destFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("复制文件内容失败: %v", err)
	}
	_ = destFile.Sync()
	if err := destFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("关闭临时目标文件失败: %v", err)
	}

	// 设置文件权限（仅在Unix系统上）
	if runtime.GOOS != "windows" {
		sourceInfo, err := os.Stat(src)
		if err != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("获取源文件权限失败: %v", err)
		}

		if err := os.Chmod(tempPath, sourceInfo.Mode()); err != nil {
			_ = os.Remove(tempPath)
			return fmt.Errorf("设置临时目标文件权限失败: %v", err)
		}
	}

	// Windows 下 os.Rename 不能覆盖已存在的文件；行为上与旧实现（Create+Truncate）一致，尽力移除旧文件。
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst)
	}

	if err := os.Rename(tempPath, dst); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("替换目标文件失败: %v", err)
	}

	return nil
}
