package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

// maskIP 对 IP 地址进行脱敏处理
func maskIP(rawIP string) string {
	rawIP = strings.TrimSpace(rawIP)
	if rawIP == "" {
		return ""
	}

	// 处理多个IP地址
	if strings.ContainsAny(rawIP, ",; ") {
		var maskedIPs []string
		for _, ip := range strings.FieldsFunc(rawIP, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t'
		}) {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				maskedIPs = append(maskedIPs, maskIP(ip))
			}
		}
		return strings.Join(maskedIPs, ", ")
	}

	// 提取zone id
	zone := ""
	if idx := strings.Index(rawIP, "%"); idx >= 0 {
		zone = rawIP[idx:]
		rawIP = rawIP[:idx]
	}

	isIPv6 := strings.Contains(rawIP, ":")
	if !isIPv6 {
		parts := strings.Split(rawIP, ".")
		if len(parts) == 4 {
			maskedIP := parts[0] + "." + parts[1] + ".*.*"
			if zone != "" {
				maskedIP += zone
			}
			return maskedIP
		}
		return "****"
	}

	segments := strings.Split(rawIP, ":")
	var nonEmptySegments []string
	for _, seg := range segments {
		if seg != "" {
			nonEmptySegments = append(nonEmptySegments, seg)
		}
	}

	var maskedIP string
	if len(nonEmptySegments) >= 2 {
		maskedIP = nonEmptySegments[0] + ":" + nonEmptySegments[1] + ":****:****:****:****:****:****"
	} else if len(nonEmptySegments) == 1 {
		maskedIP = nonEmptySegments[0] + ":****:****:****:****:****:****:****"
	} else {
		maskedIP = "****:****:****:****:****:****:****:****"
	}

	if zone != "" {
		maskedIP += zone
	}
	return maskedIP
}

// WebSocket消息类型常量
const (
	TypeShellCommand    = "shell_command"
	TypeShellResponse   = "shell_response"
	TypeFileList        = "file_list"
	TypeFileContent     = "file_content"
	TypeFileUpload      = "file_upload"
	TypeFileDownload    = "file_download"
	TypeProcessList     = "process_list"
	TypeProcessKill     = "process_kill"
	TypeProcessResponse = "process_list_response"
	TypeProcessKillResp = "process_kill_response"
	TypeDockerCommand   = "docker_command"
	TypeNginxCommand    = "nginx_command"
	TypeError           = "error"
	TypeMonitor         = "monitor" // 监控数据类型
	TypeSystemInfo      = "system_info"
)

// WebSocket 请求超时常量
const (
	TimeoutSimpleQuery   = 30 * time.Second  // 简单查询操作（容器列表、进程列表等）
	TimeoutFileOperation = 60 * time.Second  // 文件操作（读取、保存、删除等）
	TimeoutLongOperation = 120 * time.Second // 长时间操作（Docker pull/compose up、镜像构建等）
	TimeoutTerminalCWD   = 10 * time.Second  // 终端工作目录查询
	TimeoutProcessQuery  = 10 * time.Second  // 进程查询
)

// WebSocket连接升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源的WebSocket连接，生产环境应该限制
	},
}

// SafeConn 线程安全的WebSocket连接
type SafeConn struct {
	*websocket.Conn
	mu sync.Mutex
}

// 安全地向WebSocket写入JSON数据
func (c *SafeConn) WriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(v)
}

// 安全地向WebSocket写入消息
func (c *SafeConn) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(messageType, data)
}

// 安全地关闭WebSocket连接
func (c *SafeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.Close()
}

// 安全地设置写入截止时间
func (c *SafeConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}

// 安全地发送关闭消息
func (c *SafeConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteControl(messageType, data, deadline)
}

// 读取WebSocket消息
// 注意：读取操作通常不需要互斥锁保护，因为WebSocket允许并发读取
// 但为了接口一致性，我们仍然提供这个方法
func (c *SafeConn) ReadMessage() (int, []byte, error) {
	return c.Conn.ReadMessage()
}

// 从查询参数中验证JWT
func verifyJWTFromQuery(tokenString string) (*utils.Claims, error) {
	return utils.ParseToken(tokenString)
}

// 全局变量导出，供其他控制器使用
// 存储活跃的Agent WebSocket连接
var ActiveAgentConnections sync.Map

// 存储活跃的用户终端WebSocket连接 - 按会话ID索引
var ActiveTerminalConnections sync.Map

// 存储活跃的日志流连接 - key: streamID, value: *SafeConn (用户连接)
var ActiveLogStreamConnections sync.Map

// 存储公开探针监控连接
var ActivePublicMonitorConnections sync.Map

// 存储上次广播时间，用于限流
var LastBroadcastTimes sync.Map

type publicConnSet struct {
	mu    sync.Mutex
	conns map[*SafeConn]struct{}
}

func (s *publicConnSet) add(conn *SafeConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns == nil {
		s.conns = make(map[*SafeConn]struct{})
	}
	s.conns[conn] = struct{}{}
}

func (s *publicConnSet) remove(conn *SafeConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

func (s *publicConnSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (s *publicConnSet) broadcast(v interface{}) {
	s.mu.Lock()
	conns := make([]*SafeConn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.mu.Unlock()

	for _, conn := range conns {
		if err := conn.WriteJSON(v); err != nil {
			log.Printf("广播公开监控数据失败: %v", err)
		}
	}
}

func registerPublicMonitorConnection(serverID uint, conn *SafeConn) {
	value, _ := ActivePublicMonitorConnections.LoadOrStore(serverID, &publicConnSet{})
	set, _ := value.(*publicConnSet)
	set.add(conn)
}

func unregisterPublicMonitorConnection(serverID uint, conn *SafeConn) {
	if value, ok := ActivePublicMonitorConnections.Load(serverID); ok {
		if set, _ := value.(*publicConnSet); set != nil {
			set.remove(conn)
			if set.len() == 0 {
				ActivePublicMonitorConnections.Delete(serverID)
			}
		}
	}
}

func broadcastPublicMonitor(serverID uint, data map[string]interface{}) {
	if value, ok := ActivePublicMonitorConnections.Load(serverID); ok {
		if set, _ := value.(*publicConnSet); set != nil {
			message := struct {
				Type string                 `json:"type"`
				Data map[string]interface{} `json:"data"`
			}{
				Type: TypeMonitor,
				Data: data,
			}
			set.broadcast(message)
		}
	}
}

// dockerResponseChannels 通用 request-response 关联映射。
// 虽然命名包含 "docker"，但实际被 Docker、Nginx、终端CWD查询等多种请求类型共用，
// 用于将 Agent 响应路由到发起请求的 goroutine。
// key: requestID (string), value: chan interface{} 或 chan map[string]interface{}
var dockerResponseChannels sync.Map

// 存储Docker命令的请求映射，用于将响应发送给正确的用户
var dockerRequestMap sync.Map

// 【安全修复】存储每个服务器的待处理请求列表，用于在连接断开时快速失败
// 键: serverID (uint), 值: *pendingRequestSet
var serverPendingRequests sync.Map

// pendingRequestSet 用于跟踪某个服务器的所有待处理请求
type pendingRequestSet struct {
	mu         sync.Mutex
	requestIDs map[string]struct{}
}

// registerPendingRequest 注册一个待处理请求
func registerPendingRequest(serverID uint, requestID string) {
	val, _ := serverPendingRequests.LoadOrStore(serverID, &pendingRequestSet{
		requestIDs: make(map[string]struct{}),
	})
	set := val.(*pendingRequestSet)
	set.mu.Lock()
	defer set.mu.Unlock()
	set.requestIDs[requestID] = struct{}{}
}

// unregisterPendingRequest 取消注册一个待处理请求
func unregisterPendingRequest(serverID uint, requestID string) {
	val, ok := serverPendingRequests.Load(serverID)
	if !ok {
		return
	}
	set := val.(*pendingRequestSet)
	set.mu.Lock()
	defer set.mu.Unlock()
	delete(set.requestIDs, requestID)
}

// failAllPendingRequests 使某个服务器的所有待处理请求立即失败
// 当Agent连接断开时调用
func failAllPendingRequests(serverID uint) {
	val, ok := serverPendingRequests.Load(serverID)
	if !ok {
		return
	}
	set := val.(*pendingRequestSet)
	set.mu.Lock()
	requestIDs := make([]string, 0, len(set.requestIDs))
	for id := range set.requestIDs {
		requestIDs = append(requestIDs, id)
	}
	set.requestIDs = make(map[string]struct{}) // 清空
	set.mu.Unlock()

	log.Printf("[安全修复] 服务器 %d 断开连接，使 %d 个待处理请求失败", serverID, len(requestIDs))

	// 构造统一的错误响应
	errorResponse := map[string]interface{}{
		"type":    "error",
		"error":   "Agent连接已断开",
		"message": "服务器Agent连接已断开，请求失败",
	}

	// 通知所有等待的响应通道
	for _, requestID := range requestIDs {
		// 尝试从Docker响应通道获取并发送错误
		// 使用类型开关处理多种可能的通道类型
		if respChanVal, ok := dockerResponseChannels.LoadAndDelete(requestID); ok {
			notifyDockerChannel(respChanVal, requestID, errorResponse)
		}
		// 清理Docker请求映射
		dockerRequestMap.Delete(requestID)

		// 尝试从文件请求通道获取并发送错误
		fileRequestMutex.Lock()
		if respChan, ok := fileRequestMap[requestID]; ok {
			delete(fileRequestMap, requestID)
			fileRequestMutex.Unlock()
			select {
			case respChan <- errorResponse:
				log.Printf("[安全修复] 已通知文件请求 %s 失败（Agent断开）", requestID)
			default:
				log.Printf("[安全修复] 文件请求 %s 的响应通道已满或关闭", requestID)
			}
		} else {
			fileRequestMutex.Unlock()
		}
	}
}

// notifyDockerChannel 使用类型开关安全地向Docker响应通道发送错误
// 支持 chan interface{} 和 chan map[string]interface{} 两种通道类型
func notifyDockerChannel(respChanVal interface{}, requestID string, errorResponse map[string]interface{}) {
	switch ch := respChanVal.(type) {
	case chan interface{}:
		select {
		case ch <- errorResponse:
			log.Printf("[安全修复] 已通知Docker请求 %s 失败（Agent断开）", requestID)
		default:
			log.Printf("[安全修复] Docker请求 %s 的响应通道已满或关闭", requestID)
		}
	case chan map[string]interface{}:
		select {
		case ch <- errorResponse:
			log.Printf("[安全修复] 已通知Docker请求 %s 失败（Agent断开，map通道）", requestID)
		default:
			log.Printf("[安全修复] Docker请求 %s 的响应通道已满或关闭（map通道）", requestID)
		}
	default:
		log.Printf("[安全修复] Docker请求 %s 的响应通道类型未知: %T", requestID, respChanVal)
	}
}

// 以下变量已经在process_controller.go中定义，这里注释掉
// var processResponseChannels sync.Map
// var processRequestMap sync.Map

// PublicWebSocketHandler 处理公开的WebSocket连接，不需要鉴权
func PublicWebSocketHandler(c *gin.Context) {
	// 记录请求URL
	log.Printf("公开WebSocket连接请求: %s", c.Request.URL.Path)

	// 尝试从不同的路由参数中获取服务器ID
	var idStr string
	idStr = c.Param("id")

	// 检查ID参数是否有效
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID格式"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 确保服务器有监控数据
	ensureMonitorDataExists(server.ID)

	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}

	// 创建一个安全的连接包装器
	safeConn := &SafeConn{Conn: conn}
	defer safeConn.Close()

	// 设置一个通道来接收中断信号
	interrupt := make(chan struct{})
	defer close(interrupt)

	// 启动WebSocket处理，sessionParam设为空表示这是一个监控连接
	handlePublicWebSocket(safeConn, server, interrupt)
}

// PublicServersWebSocketHandler 推送全部服务器列表
func PublicServersWebSocketHandler(c *gin.Context) {
	log.Printf("公开服务器列表WebSocket连接请求: %s", c.Request.URL.Path)

	// 检查是否已认证（通过Token）
	token := c.Query("token")
	isAuthenticated := false
	if token != "" {
		// 验证JWT Token
		claims, err := verifyJWTFromQuery(token)
		if err == nil && claims != nil {
			isAuthenticated = true
		}
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级公开服务器WebSocket失败: %v", err)
		return
	}
	defer conn.Close()

	sendServerList := func() error {
		servers, err := models.GetAllServers(0)
		if err != nil {
			return err
		}

		type PublicServer struct {
			ID              uint    `json:"id"`
			Name            string  `json:"name"`
			Status          string  `json:"status"`
			IP              string  `json:"ip"`
			PublicIP        string  `json:"public_ip"`
			LastSeen        int64   `json:"last_seen"`
			OS              string  `json:"os"`
			CPUUsage        float64 `json:"cpu_usage"`
			MemoryUsed      float64 `json:"memory_used"`
			MemoryTotal     float64 `json:"memory_total"`
			DiskUsed        float64 `json:"disk_used"`
			DiskTotal       float64 `json:"disk_total"`
			LoadAvg1        float64 `json:"load_avg_1"`
			LoadAvg5        float64 `json:"load_avg_5"`
			LoadAvg15       float64 `json:"load_avg_15"`
			CPUCores        int     `json:"cpu_cores"`
			CountryCode     string  `json:"country_code"`
			SwapUsed        uint64  `json:"swap_used"`
			SwapTotal       uint64  `json:"swap_total"`
			BootTime        uint64  `json:"boot_time"`
			NetworkIn       float64 `json:"network_in"`
			NetworkOut      float64 `json:"network_out"`
			NetworkInTotal  uint64  `json:"network_in_total"`
			NetworkOutTotal uint64  `json:"network_out_total"`
			Latency         float64 `json:"latency"`
			PacketLoss      float64 `json:"packet_loss"`
		}

		var list []PublicServer
		for _, server := range servers {
			systemInfo := make(map[string]interface{})
			if server.SystemInfo != "" {
				_ = json.Unmarshal([]byte(server.SystemInfo), &systemInfo)
			}

			status := "offline"
			if server.Online && time.Since(server.LastHeartbeat) <= 15*time.Second {
				status = "online"
			}

			monitorData, _ := models.GetLatestMonitorData(server.ID, 1)
			lastMonitor := models.ServerMonitor{}
			if len(monitorData) > 0 {
				lastMonitor = monitorData[0]
			}

			getFloat := func(m map[string]interface{}, key string) float64 {
				if v, ok := m[key]; ok {
					switch val := v.(type) {
					case float64:
						return val
					case float32:
						return float64(val)
					case int:
						return float64(val)
					case int64:
						return float64(val)
					}
				}
				return 0
			}

			ip := server.IP
			publicIP := server.PublicIP
			// 如果未认证，隐藏IP的最后两段
			if !isAuthenticated {
				ip = maskIP(ip)
				publicIP = maskIP(publicIP)
			}

			list = append(list, PublicServer{
				ID:              server.ID,
				Name:            server.Name,
				Status:          status,
				IP:              ip,
				PublicIP:        publicIP,
				LastSeen:        server.LastHeartbeat.Unix(),
				OS:              toString(systemInfo["platform"], toString(systemInfo["os"], "")),
				CPUUsage:        lastMonitor.CPUUsage,
				MemoryUsed:      float64(lastMonitor.MemoryUsed),
				MemoryTotal:     getFloat(systemInfo, "memory_total"),
				DiskUsed:        float64(lastMonitor.DiskUsed),
				DiskTotal:       getFloat(systemInfo, "disk_total"),
				LoadAvg1:        lastMonitor.LoadAvg1,
				LoadAvg5:        lastMonitor.LoadAvg5,
				LoadAvg15:       lastMonitor.LoadAvg15,
				CPUCores:        server.CPUCores,
				CountryCode:     server.CountryCode,
				SwapUsed:        lastMonitor.SwapUsed,
				SwapTotal:       lastMonitor.SwapTotal,
				BootTime:        lastMonitor.BootTime,
				NetworkIn:       lastMonitor.NetworkIn,
				NetworkOut:      lastMonitor.NetworkOut,
				NetworkInTotal:  server.NetworkInTotal,
				NetworkOutTotal: server.NetworkOutTotal,
				Latency:         server.Latency,
				PacketLoss:      server.PacketLoss,
			})
		}

		wrapper := map[string]interface{}{
			"type":    "server_list",
			"servers": list,
		}

		return conn.WriteJSON(wrapper)
	}

	if err := sendServerList(); err != nil {
		log.Printf("发送公开服务器列表失败: %v", err)
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 独立 goroutine 处理读消息，检测客户端断开
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("公开服务器WebSocket关闭: %v", err)
				}
				return
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			if err := sendServerList(); err != nil {
				log.Printf("刷新公开服务器列表失败: %v", err)
				return
			}
		case <-readDone:
			return
		}
	}
}

// 处理公开的WebSocket连接
func handlePublicWebSocket(conn *SafeConn, server *models.Server, interrupt chan struct{}) {
	log.Printf("开始处理服务器 %d 的公开WebSocket连接", server.ID)

	// 发送欢迎消息
	welcomeMsg := struct {
		Type       string          `json:"type"`
		Message    string          `json:"message"`
		ServerID   uint            `json:"server_id"`
		SystemInfo json.RawMessage `json:"system_info"`
		Status     string          `json:"status"`
		Name       string          `json:"name"`
		Hostname   string          `json:"hostname"`
		IP         string          `json:"ip"`
		OS         string          `json:"os"`
		Arch       string          `json:"arch"`
		CPUCores   int             `json:"cpu_cores"`
		CPUModel   string          `json:"cpu_model"`
		Region     string          `json:"region"`
	}{
		Type:       "welcome",
		Message:    "连接成功，服务器ID: " + strconv.Itoa(int(server.ID)),
		ServerID:   server.ID,
		SystemInfo: safeSystemInfo(server),
		Status:     server.Status,
		Name:       server.Name,
		Hostname:   server.Hostname,
		IP:         maskIP(server.IP),
		OS:         server.OS,
		Arch:       server.Arch,
		CPUCores:   server.CPUCores,
		CPUModel:   server.CPUModel,
		Region:     server.CountryCode,
	}

	log.Printf("向服务器 %d 的WebSocket发送欢迎消息", server.ID)
	if err := conn.WriteJSON(welcomeMsg); err != nil {
		log.Printf("发送欢迎消息失败: %v", err)
		return
	}

	if err := sendInitialMonitorData(conn, server); err != nil {
		log.Printf("发送服务器 %d 的初始监控数据失败: %v", server.ID, err)
		return
	}

	registerPublicMonitorConnection(server.ID, conn)
	defer unregisterPublicMonitorConnection(server.ID, conn)

	// 处理接收到的消息
	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("服务器 %d 的WebSocket读取错误: %v", server.ID, err)
			} else {
				log.Printf("服务器 %d 的WebSocket连接正常关闭", server.ID)
			}
			break
		}

		// 解析消息
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("服务器 %d 的WebSocket解析消息错误: %v", server.ID, err)
			sendErrorMessage(conn, "消息格式错误")
			continue
		}
	}

	log.Printf("服务器 %d 的WebSocket处理循环结束", server.ID)
}

// 检查数据库中是否有监控数据的函数
func ensureMonitorDataExists(serverID uint) {
	// 检查是否有监控数据
	latestData, err := models.GetLatestMonitorData(serverID, 1)
	if err != nil || len(latestData) == 0 {
		log.Printf("服务器 %d 没有监控数据", serverID)
		// 移除所有生成测试数据的代码，只保留日志记录
	} else {
		log.Printf("服务器 %d 已有监控数据，最新数据时间戳: %v", serverID, latestData[0].Timestamp)
	}
}

// 返回一个安全的JSON系统信息，如果为空或无效则返回空对象
func safeSystemInfo(server *models.Server) json.RawMessage {
	if len(server.SystemInfo) == 0 {
		return json.RawMessage("{}")
	}

	var js json.RawMessage
	if err := json.Unmarshal([]byte(server.SystemInfo), &js); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(server.SystemInfo)
}

// 获取服务器的最新监控记录
func getLatestMonitorRecord(serverID uint) (*models.ServerMonitor, error) {
	records, err := models.GetLatestMonitorData(serverID, 1)
	if err != nil || len(records) == 0 {
		return nil, err
	}
	record := records[0]
	return &record, nil
}

// 构建带有BootTime等完整信息的监控数据映射
func buildMonitorData(server *models.Server, monitor *models.ServerMonitor) map[string]interface{} {
	if monitor == nil {
		return nil
	}

	data := map[string]interface{}{
		"timestamp":         monitor.Timestamp.Unix(),
		"cpu_usage":         monitor.CPUUsage,
		"memory_used":       monitor.MemoryUsed,
		"memory_total":      monitor.MemoryTotal,
		"disk_used":         monitor.DiskUsed,
		"disk_total":        monitor.DiskTotal,
		"network_in":        monitor.NetworkIn,
		"network_out":       monitor.NetworkOut,
		"load_avg_1":        monitor.LoadAvg1,
		"load_avg_5":        monitor.LoadAvg5,
		"load_avg_15":       monitor.LoadAvg15,
		"swap_used":         monitor.SwapUsed,
		"swap_total":        monitor.SwapTotal,
		"boot_time":         monitor.BootTime,
		"status":            server.Status,
		"network_in_total":  server.NetworkInTotal,
		"network_out_total": server.NetworkOutTotal,
		"latency":           monitor.Latency,
		"packet_loss":       monitor.PacketLoss,
	}

	// 只有当进程数、TCP/UDP连接数大于0时才发送这些字段
	// 避免用旧数据或无效数据覆盖前端已显示的正常值
	if monitor.Processes > 0 {
		data["processes"] = monitor.Processes
	}
	if monitor.TCPConnections > 0 {
		data["tcp_connections"] = monitor.TCPConnections
	}
	if monitor.UDPConnections > 0 {
		data["udp_connections"] = monitor.UDPConnections
	}

	// 兼容旧数据中未设置的延迟/丢包
	if monitor.Latency == 0 {
		data["latency"] = server.Latency
	}
	if monitor.PacketLoss == 0 {
		data["packet_loss"] = server.PacketLoss
	}
	return data
}

func sendMonitorDataMessage(conn *SafeConn, server *models.Server, monitor *models.ServerMonitor) error {
	if monitor == nil {
		return sendNoMonitorData(conn)
	}

	monitorMsg := struct {
		Type string                 `json:"type"`
		Data map[string]interface{} `json:"data"`
	}{
		Type: TypeMonitor,
		Data: buildMonitorData(server, monitor),
	}

	return conn.WriteJSON(monitorMsg)
}

func sendNoMonitorData(conn *SafeConn) error {
	noDataMsg := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{
		Type:    "no_data",
		Message: "没有监控数据",
	}
	return conn.WriteJSON(noDataMsg)
}

func sendInitialMonitorData(conn *SafeConn, server *models.Server) error {
	monitor, err := getLatestMonitorRecord(server.ID)
	if err != nil || monitor == nil {
		return sendNoMonitorData(conn)
	}
	return sendMonitorDataMessage(conn, server, monitor)
}

// WebSocketHandler 处理WebSocket连接
func WebSocketHandler(c *gin.Context) {
	// 鉴权

	// 记录请求URL和Token（便于调试）
	log.Printf("WebSocket连接请求: %s, Token: %s", c.Request.URL.Path, c.Query("token"))

	// 尝试从不同的路由参数中获取服务器ID
	var idStr string
	idStr = c.Param("id")

	// 检查ID参数是否有效
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID格式"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查认证来源（JWT或Secret Key）
	var authenticated bool
	var isAgent bool

	// 尝试JWT认证
	userId, exists := c.Get("userId")
	if exists {
		authenticated = true
		log.Printf("WebSocket通过JWT认证: 用户ID=%v", userId)
	}

	if !authenticated {
		token := c.Query("token")
		log.Printf("尝试Secret Key认证: token_len=%d, match=%t", len(token), token == server.SecretKey)
		if token != "" && token == server.SecretKey {
			authenticated = true
			isAgent = true // 标记为Agent连接
			log.Printf("WebSocket通过Secret Key认证成功")
		} else if token != "" {
			// 如果提供了Token但不匹配，尝试作为JWT验证
			log.Printf("Secret Key不匹配，尝试作为JWT验证")
			claims, err := verifyJWTFromQuery(token)
			if err == nil && claims != nil {
				authenticated = true
				log.Printf("WebSocket通过JWT认证成功: 用户=%s, 角色=%s", claims.Username, claims.Role)
				c.Set("userId", claims.UserID)
				c.Set("username", claims.Username)
				c.Set("role", claims.Role)
			} else {
				log.Printf("JWT验证失败: %v", err)
			}
		}
	}

	// 如果都未通过认证
	if !authenticated {
		log.Printf("WebSocket认证失败")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未经授权"})
		return
	}

	// 获取会话参数（用于后续使用）
	sessionParam := c.Query("session")

	// 检查是否是监控专用WebSocket
	isMonitorWs := strings.HasSuffix(c.Request.URL.Path, "/monitor-ws")

	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("升级WebSocket连接失败: %v", err)
		return
	}

	// 创建安全连接包装器
	safeConn := &SafeConn{Conn: conn}
	defer safeConn.Close()

	// 如果是Agent连接，保存到全局映射中
	if isAgent {
		log.Printf("发现Agent连接，保存到连接映射中，服务器ID: %d", server.ID)
		// 如果已存在，先关闭旧连接
		if oldConn, loaded := ActiveAgentConnections.LoadAndDelete(server.ID); loaded {
			if old, ok := oldConn.(*SafeConn); ok {
				log.Printf("关闭服务器 %d 的旧Agent连接", server.ID)
				old.Close()
			}
		}
		// 存储新连接
		ActiveAgentConnections.Store(server.ID, safeConn)

		// 设置函数在连接关闭时从映射中移除，并使所有待处理请求失败
		defer func(id uint) {
			log.Printf("Agent连接关闭，从映射中移除，服务器ID: %d", id)
			ActiveAgentConnections.Delete(id)
			// 【安全修复】使该服务器的所有待处理请求立即失败
			failAllPendingRequests(id)

			// 通知前端监控订阅者Agent已离线
			broadcastPublicMonitor(id, map[string]interface{}{
				"type":      "agent_offline",
				"server_id": id,
				"message":   "Agent连接已断开",
				"timestamp": time.Now().Unix(),
			})

			// 通知该服务器所有终端会话用户Agent已断开
			terminalSessions.Range(func(key, value interface{}) bool {
				session, ok := value.(TerminalSession)
				if !ok || session.ServerID != id {
					return true
				}
				sessionID, ok := key.(string)
				if !ok {
					return true
				}
				if userConnVal, ok := ActiveTerminalConnections.Load(sessionID); ok {
					if userConn, ok := userConnVal.(*SafeConn); ok {
						userConn.WriteJSON(map[string]interface{}{
							"type":       "terminal_error",
							"session_id": sessionID,
							"message":    "Agent连接已断开，终端会话不可用",
							"timestamp":  time.Now().Unix(),
						})
					}
				}
				return true
			})
		}(server.ID)

		// 更新服务器状态为在线
		server.Status = "online"
		err = models.UpdateServerStatus(server.ID, "online")
		if err != nil {
			log.Printf("更新服务器状态失败: %v", err)
		} else {
			log.Printf("服务器 %d 状态已更新为在线", server.ID)
		}
	}

	// 设置一个通道来接收中断信号
	interrupt := make(chan struct{})
	defer close(interrupt)

	// 如果是专用监控WebSocket，启动简化版处理
	if isMonitorWs {
		log.Printf("启动服务器 %d 的监控专用WebSocket处理", server.ID)
		handleMonitorWebSocket(safeConn, server, interrupt)
		return
	}

	// 启动标准WebSocket处理
	handleWebSocket(safeConn, server, interrupt, sessionParam, isAgent)
}

// 新增：处理监控专用WebSocket连接
func handleMonitorWebSocket(conn *SafeConn, server *models.Server, interrupt chan struct{}) {
	log.Printf("开始处理服务器 %d 的监控专用WebSocket", server.ID)

	// 发送欢迎消息
	welcomeMsg := struct {
		Type       string          `json:"type"`
		Message    string          `json:"message"`
		ServerID   uint            `json:"server_id"`
		Name       string          `json:"name"`
		IP         string          `json:"ip"`
		LastSeen   int64           `json:"last_seen"`
		SystemInfo json.RawMessage `json:"system_info"`
		Status     string          `json:"status"`
	}{
		Type:       "welcome",
		Message:    "连接成功，服务器ID: " + strconv.Itoa(int(server.ID)),
		ServerID:   server.ID,
		Name:       server.Name,
		IP:         server.IP,
		LastSeen:   server.LastHeartbeat.Unix(),
		SystemInfo: safeSystemInfo(server),
		Status:     server.Status,
	}

	if err := conn.WriteJSON(welcomeMsg); err != nil {
		log.Printf("发送欢迎消息失败: %v", err)
		return
	}

	if err := sendInitialMonitorData(conn, server); err != nil {
		log.Printf("发送监控专用WebSocket初始数据失败: %v", err)
		return
	}

	registerPublicMonitorConnection(server.ID, conn)
	defer unregisterPublicMonitorConnection(server.ID, conn)

	// 处理接收到的消息
	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("监控WebSocket读取错误: %v", err)
			} else {
				log.Printf("监控WebSocket连接正常关闭")
			}
			break
		}

		// 解析消息
		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("解析WebSocket消息错误: %v", err)
			sendErrorMessage(conn, "消息格式错误")
			continue
		}
	}
}

// 处理WebSocket连接
func handleWebSocket(conn *SafeConn, server *models.Server, interrupt chan struct{}, sessionParam string, isAgent bool) {
	// WebSocket消息结构
	type Message struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}

	// 发送欢迎消息
	welcomeMsg := struct {
		Type       string          `json:"type"`
		Message    string          `json:"message"`
		ServerID   uint            `json:"server_id"`
		SystemInfo json.RawMessage `json:"system_info"`
		Status     string          `json:"status"` // 添加状态字段
	}{
		Type:       "welcome",
		Message:    "连接成功，服务器ID: " + strconv.Itoa(int(server.ID)),
		ServerID:   server.ID,
		SystemInfo: safeSystemInfo(server),
		Status:     server.Status, // 添加状态字段
	}

	// 如果不是Agent，发送欢迎消息
	if !isAgent {
		if err := conn.WriteJSON(welcomeMsg); err != nil {
			log.Printf("发送欢迎消息失败: %v", err)
		}
	}

	// 根据URL参数判断是否为监控连接（排除Agent）
	isMonitor := sessionParam == "" && !isAgent

	// 如果是监控连接，立即发送一次监控数据
	if isMonitor {
		if err := sendInitialMonitorData(conn, server); err != nil {
			log.Printf("发送监控初始数据失败: %v", err)
		}

		registerPublicMonitorConnection(server.ID, conn)
		defer unregisterPublicMonitorConnection(server.ID, conn)
	}

	// 如果是普通用户连接且有会话参数，说明是终端连接
	if !isAgent && sessionParam != "" {
		// 存储会话ID对应的用户连接
		log.Printf("存储终端会话 %s 的用户连接，服务器ID: %d", sessionParam, server.ID)

		// 如果已有连接，先关闭旧连接
		if oldConn, loaded := ActiveTerminalConnections.LoadAndDelete(sessionParam); loaded {
			if old, ok := oldConn.(*SafeConn); ok {
				log.Printf("关闭终端会话 %s 的旧用户连接", sessionParam)
				old.Close()
			}
		}

		// 存储新连接
		ActiveTerminalConnections.Store(sessionParam, conn)

		// 设置函数在连接关闭时从映射中移除
		defer func(sessionID string) {
			log.Printf("用户连接关闭，从映射中移除终端会话连接: %s", sessionID)
			ActiveTerminalConnections.Delete(sessionID)
		}(sessionParam)
	}

	// Agent连接启用ping/pong心跳，及时感知断连
	if isAgent {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(appData string) error {
			conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			return nil
		})

		pingDone := make(chan struct{})
		defer close(pingDone)
		go func() {
			pingTicker := time.NewTicker(30 * time.Second)
			defer pingTicker.Stop()
			for {
				select {
				case <-pingTicker.C:
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						log.Printf("服务器 %d 的ping发送失败: %v", server.ID, err)
						return
					}
				case <-pingDone:
					return
				}
			}
		}()
	}

	// 处理接收到的消息
	for {
		// 读取消息
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("服务器 %d 的WebSocket读取错误: %v", server.ID, err)
			} else {
				log.Printf("服务器 %d 的WebSocket连接正常关闭", server.ID)
			}
			break
		}

		// 解析消息
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("服务器 %d 的WebSocket解析消息错误: %v", server.ID, err)
			sendErrorMessage(conn, "消息格式错误")
			continue
		}

		// 根据消息类型处理
		switch msg.Type {
		case TypeShellCommand:
			// Shell命令的处理
			handleShellCommand(conn, server, msg.Payload)
		case TypeProcessList:
			// 进程列表的处理
			handleProcessList(conn, server, msg.Payload)
		case TypeProcessKill:
			// 进程终止的处理
			handleProcessKill(conn, server, msg.Payload)
		case TypeDockerCommand:
			// Docker命令的处理
			handleDockerCommand(conn, server, msg.Payload)
		case "docker_logs_stream":
			// Docker日志流的处理（start / stop）
			handleDockerLogsStream(conn, server, msg.Payload)
		case TypeMonitor:
			// Agent 上报监控数据
			if !isAgent {
				log.Printf("非Agent连接发送监控数据，已忽略")
				continue
			}

			if len(msg.Payload) == 0 {
				log.Printf("监控数据为空，服务器ID: %d", server.ID)
				continue
			}

			var monitorPayload MonitorPayload

			if err := json.Unmarshal(msg.Payload, &monitorPayload); err != nil {
				log.Printf("解析监控数据失败: %v", err)
				continue
			}

			record, err := persistMonitorPayload(server, &monitorPayload)
			if err != nil {
				log.Printf("保存监控数据失败: %v", err)
				continue
			}

			// 推送给公开探针的订阅者
			broadcastData := buildMonitorData(server, record)
			// 限流：每秒最多广播一次
			lastTime, ok := LastBroadcastTimes.Load(server.ID)
			if !ok || time.Since(lastTime.(time.Time)) >= 1*time.Second {
				broadcastPublicMonitor(server.ID, broadcastData)
				LastBroadcastTimes.Store(server.ID, time.Now())
			}
		case TypeSystemInfo:
			// Agent 上报系统信息
			if !isAgent {
				log.Printf("非Agent连接发送系统信息，已忽略")
				continue
			}

			if len(msg.Payload) == 0 {
				log.Printf("系统信息为空，服务器ID: %d", server.ID)
				continue
			}

			var systemInfoData map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &systemInfoData); err != nil {
				log.Printf("解析系统信息失败: %v", err)
				continue
			}

			systemInfoJSON, err := json.Marshal(systemInfoData)
			if err != nil {
				log.Printf("系统信息序列化失败: %v", err)
				continue
			}

			clientIP := ""
			if conn != nil && conn.RemoteAddr() != nil {
				clientIP = conn.RemoteAddr().String()
				if idx := strings.LastIndex(clientIP, ":"); idx > 0 {
					clientIP = clientIP[:idx]
				}
			}
			clientIP = strings.TrimSpace(clientIP)

			// 如果clientIP是本地回环地址（127.0.0.1或::1），尝试从systemInfoData的public_ip获取
			if clientIP == "127.0.0.1" || clientIP == "::1" || clientIP == "localhost" {
				if publicIPFromData, ok := systemInfoData["public_ip"].(string); ok && strings.TrimSpace(publicIPFromData) != "" {
					clientIP = strings.TrimSpace(publicIPFromData)
					log.Printf("检测到本地回环连接，使用Agent上报的公网IP作为连接IP: %s", clientIP)
				}
			}

			clientIPChanged := clientIP != "" && server.IP != clientIP
			if clientIPChanged {
				server.IP = clientIP
			}

			publicIP := ""
			publicIPChanged := false
			if val, ok := systemInfoData["public_ip"].(string); ok {
				publicIP = strings.TrimSpace(val)
				if publicIP != "" && publicIP != server.PublicIP {
					server.PublicIP = publicIP
					publicIPChanged = true
				}
			}

			if osVal, ok := systemInfoData["os"].(string); ok && osVal != "" {
				server.OS = osVal
			}

			if arch, ok := systemInfoData["kernel_arch"].(string); ok && arch != "" {
				server.Arch = arch
			}

			if cpuCores, ok := systemInfoData["cpu_cores"].(float64); ok && cpuCores > 0 {
				server.CPUCores = int(cpuCores)
			}

			if cpuModel, ok := systemInfoData["cpu_model"].(string); ok && cpuModel != "" {
				server.CPUModel = cpuModel
			}

			if agentVersion, ok := systemInfoData["agent_version"].(string); ok {
				agentVersion = strings.TrimSpace(agentVersion)
				if agentVersion != "" {
					server.AgentVersion = agentVersion
				}
			}

			if agentType, ok := systemInfoData["agent_type"].(string); ok {
				agentType = strings.TrimSpace(agentType)
				if agentType == "full" || agentType == "monitor" {
					// 仅在数据库中 agent_type 为空时初始化，避免覆盖 SwitchAgentType 的设置。
					// Agent 上报的是编译时类型，在类型切换期间（旧二进制尚未替换），
					// 无条件覆盖会将用户刚切换的类型回写为旧值，产生竞态。
					if server.AgentType == "" {
						server.AgentType = agentType
					} else if server.AgentType != agentType {
						log.Printf("server=%d agent_type 不一致: db=%s, reported=%s (以数据库为准，跳过覆盖)",
							server.ID, server.AgentType, agentType)
					}
				}
			}

			if memoryTotal, ok := systemInfoData["memory_total"].(float64); ok && memoryTotal > 0 {
				server.MemoryTotal = int64(memoryTotal)
			}

			if diskTotal, ok := systemInfoData["disk_total"].(float64); ok && diskTotal > 0 {
				server.DiskTotal = int64(diskTotal)
			}

			server.SystemInfo = string(systemInfoJSON)
			server.Status = "online"
			server.Online = true
			server.LastHeartbeat = time.Now()

			if err := models.UpdateServerHeartbeatAndStatus(server.ID, server.Status); err != nil {
				log.Printf("更新服务器状态失败: %v", err)
			}

			updates := map[string]interface{}{
				"system_info":   server.SystemInfo,
				"ip":            server.IP,
				"public_ip":     server.PublicIP,
				"os":            server.OS,
				"arch":          server.Arch,
				"cpu_cores":     server.CPUCores,
				"cpu_model":     server.CPUModel,
				"agent_version": server.AgentVersion,
				"memory_total":  server.MemoryTotal,
				"disk_total":    server.DiskTotal,
			}
			// agent_type 不在 system_info 路径中更新，完全由 SwitchAgentType 和创建时管理

			if err := models.DB.Model(&models.Server{}).Where("id = ?", server.ID).Updates(updates).Error; err != nil {
				log.Printf("更新服务器信息失败: %v", err)
			} else {
				geoIP := server.PublicIP
				shouldUpdateCountry := publicIPChanged && geoIP != ""
				if !shouldUpdateCountry && server.PublicIP == "" && clientIPChanged {
					geoIP = server.IP
					shouldUpdateCountry = geoIP != ""
				}
				if shouldUpdateCountry {
					go updateServerCountry(server.ID, geoIP)
				}
			}
		case "working_directory":
			// 处理工作目录响应
			log.Printf("收到工作目录响应消息，服务器ID: %d", server.ID)

			// 解析工作目录响应消息
			var workingDirMsg struct {
				Type       string `json:"type"`
				Session    string `json:"session"`
				WorkingDir string `json:"working_dir"`
			}
			if err := json.Unmarshal(message, &workingDirMsg); err != nil {
				log.Printf("解析工作目录响应消息失败: %v", err)
				continue
			}

			if isAgent {
				// 当是agent发送的working_directory响应时，需要通知等待的API请求
				log.Printf("从Agent收到会话 %s 的工作目录响应: %s", workingDirMsg.Session, workingDirMsg.WorkingDir)

				// 构造响应数据
				responseData := map[string]interface{}{
					"working_dir": workingDirMsg.WorkingDir,
				}

				// 查找对应的响应通道
				requestID := fmt.Sprintf("cwd_%s", workingDirMsg.Session)
				dockerResponseChannels.Range(func(key, value interface{}) bool {
					if keyStr, ok := key.(string); ok && strings.HasPrefix(keyStr, requestID) {
						if responseChan, ok := value.(chan map[string]interface{}); ok {
							select {
							case responseChan <- responseData:
								log.Printf("成功发送工作目录响应到等待通道")
							default:
								log.Printf("响应通道已满，无法发送工作目录响应")
							}
						}
						return false // 停止遍历
					}
					return true // 继续遍历
				})
			}
		case TypeShellResponse:
			// 处理Shell响应消息
			log.Printf("收到Shell响应消息，服务器ID: %d", server.ID)

			// 解析响应消息以获取会话ID
			var responseMsg struct {
				Type    string `json:"type"`
				Session string `json:"session"`
				Data    string `json:"data"`
			}
			if err := json.Unmarshal(message, &responseMsg); err != nil {
				log.Printf("解析Shell响应消息失败: %v", err)
				continue
			}

			if isAgent {
				// 当是agent发送的shell_response时，转发给对应会话的用户连接
				sessionID := responseMsg.Session
				log.Printf("从Agent收到会话 %s 的Shell响应，尝试转发给用户", sessionID)

				// 查找对应会话的用户连接
				userConnVal, ok := ActiveTerminalConnections.Load(sessionID)
				if !ok {
					log.Printf("找不到会话 %s 的用户连接，无法转发响应", sessionID)
					continue
				}

				userConn, ok := userConnVal.(*SafeConn)
				if !ok {
					log.Printf("会话 %s 的用户连接类型错误", sessionID)
					continue
				}

				// 转发响应给用户
				if err := userConn.WriteJSON(responseMsg); err != nil {
					log.Printf("转发Shell响应到用户失败: %v", err)
				} else {
					log.Printf("成功转发Shell响应到会话 %s 的用户", sessionID)
				}
			} else {
				// 如果当前连接是用户连接且收到shell_response，这可能是意外情况
				log.Printf("用户连接收到Shell响应消息，这可能是意外情况")
			}
		case TypeProcessResponse, TypeProcessKillResp:
			// 处理进程相关响应
			var processResponse struct {
				Type      string                 `json:"type"`
				RequestID string                 `json:"request_id"`
				Data      map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(message, &processResponse); err != nil {
				log.Printf("解析进程响应消息失败: %v", err)
				continue
			}

			// 调用进程控制器的响应处理函数
			if processResponse.RequestID != "" {
				// 将响应传递给HTTP API的等待通道
				HandleProcessResponse(processResponse.RequestID, processResponse.Data)

				// 同时转发响应到WebSocket客户端
				connVal, ok := processRequestMap.Load(processResponse.RequestID)
				if ok {
					if userConn, ok := connVal.(*SafeConn); ok {
						if err := userConn.WriteJSON(processResponse); err != nil {
							log.Printf("发送进程响应到用户失败: %v", err)
						}
					}
				}
			}
		case "docker_containers", "docker_images", "docker_composes", "docker_container_logs", "docker_compose_config", "success", "error":
			// 处理Docker相关响应
			var dockerResponse struct {
				Type      string                 `json:"type"`
				RequestID string                 `json:"request_id"`
				Data      map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(message, &dockerResponse); err != nil {
				log.Printf("解析Docker响应消息失败: %v, 消息内容: %s", err, string(message))
				continue
			}

			log.Printf("收到Docker响应消息: 类型=%s, 请求ID=%s", dockerResponse.Type, dockerResponse.RequestID)

			// 处理Docker响应
			if dockerResponse.RequestID != "" {
				// 转发响应到WebSocket客户端
				connVal, ok := dockerRequestMap.Load(dockerResponse.RequestID)
				if !ok {
					log.Printf("错误: 未找到Docker请求ID=%s的WebSocket连接", dockerResponse.RequestID)
					continue
				}

				if userConn, ok := connVal.(*SafeConn); ok {
					if err := userConn.WriteJSON(dockerResponse); err != nil {
						log.Printf("发送Docker响应到用户失败: %v", err)
					} else {
						log.Printf("成功转发Docker响应 [%s] 到用户, 请求ID=%s", dockerResponse.Type, dockerResponse.RequestID)
					}
				} else {
					log.Printf("错误: Docker请求ID=%s的WebSocket连接类型错误, 实际类型: %T", dockerResponse.RequestID, connVal)
				}

				// 获取响应通道并发送响应数据
				respChanVal, ok := dockerResponseChannels.Load(dockerResponse.RequestID)
				if !ok {
					log.Printf("错误: 未找到Docker请求ID=%s的响应通道", dockerResponse.RequestID)
				} else {
					respChan, ok := respChanVal.(chan interface{})
					if !ok {
						log.Printf("错误: Docker请求ID=%s的响应通道类型错误", dockerResponse.RequestID)
					} else {
						// 发送响应到通道
						select {
						case respChan <- dockerResponse.Data:
							log.Printf("成功发送Docker响应数据到通道, 请求ID=%s", dockerResponse.RequestID)
						default:
							log.Printf("错误: Docker请求ID=%s的响应通道已满或已关闭", dockerResponse.RequestID)
						}
					}
				}

				// 响应处理完成后从映射中删除
				dockerRequestMap.Delete(dockerResponse.RequestID)
				dockerResponseChannels.Delete(dockerResponse.RequestID)
				log.Printf("已清理Docker请求ID=%s的映射和通道", dockerResponse.RequestID)
			} else {
				log.Printf("警告: 收到的Docker响应消息没有请求ID")
			}

		case "docker_logs_stream_data", "docker_logs_stream_end":
			// 处理Agent发回的日志流数据/结束消息，转发给对应的用户连接
			var streamMsg struct {
				Type     string                 `json:"type"`
				StreamID string                 `json:"stream_id"`
				Data     map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(message, &streamMsg); err != nil {
				log.Printf("解析日志流消息失败: %v", err)
				continue
			}

			if streamMsg.StreamID == "" {
				log.Printf("警告: 收到的日志流消息没有 stream_id")
				continue
			}

			// 查找对应的用户连接
			userConnVal, ok := ActiveLogStreamConnections.Load(streamMsg.StreamID)
			if !ok {
				log.Printf("未找到日志流 %s 的用户连接", streamMsg.StreamID)
				continue
			}

			if userConn, ok := userConnVal.(*SafeConn); ok {
				if err := userConn.WriteJSON(streamMsg); err != nil {
					log.Printf("转发日志流消息到用户失败: stream_id=%s, error=%v", streamMsg.StreamID, err)
				}
			}

			// 如果是流结束消息，清理映射
			if msg.Type == "docker_logs_stream_end" {
				ActiveLogStreamConnections.Delete(streamMsg.StreamID)
				log.Printf("日志流 %s 已结束，已清理连接映射", streamMsg.StreamID)
			}

		case "nginx_success", "nginx_error":
			// 处理Nginx成功/错误响应
			// 使用json.RawMessage接收任何JSON格式
			var baseResp struct {
				Type      string          `json:"type"`
				RequestID string          `json:"request_id"`
				Data      json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(message, &baseResp); err != nil {
				log.Printf("解析Nginx响应消息基础结构失败: %v, 消息内容: %s", err, string(message))
				continue
			}

			log.Printf("收到Nginx响应消息: 类型=%s, 请求ID=%s", baseResp.Type, baseResp.RequestID)

			// 处理Nginx响应
			if baseResp.RequestID != "" {
				// 确保utils包中的响应处理器能够处理该响应
				// 直接将原始消息传递给HandleAgentResponse
				utils.HandleAgentResponse(message)

				// 检查是否为Nginx请求ID (通常以数字-数字格式)
				respChanVal, ok := dockerResponseChannels.Load(baseResp.RequestID)
				if !ok {
					log.Printf("警告: 未找到Nginx请求ID=%s的响应通道，可能是请求已超时", baseResp.RequestID)
					continue
				}

				// 获取对应的WebSocket连接
				connVal, ok := dockerRequestMap.Load(baseResp.RequestID)
				if !ok {
					log.Printf("警告: 未找到Nginx请求ID=%s的WebSocket连接，但找到了响应通道", baseResp.RequestID)
				} else {
					if userConn, ok := connVal.(*SafeConn); ok {
						// 转发原始响应给客户端，这样可以避免类型转换问题
						// 创建一个新的响应结构，保持Data字段为RawMessage
						response := struct {
							Type      string          `json:"type"`
							RequestID string          `json:"request_id"`
							Data      json.RawMessage `json:"data"`
						}{
							Type:      baseResp.Type,
							RequestID: baseResp.RequestID,
							Data:      baseResp.Data,
						}

						// 转发响应到用户
						if err := userConn.WriteJSON(response); err != nil {
							log.Printf("发送Nginx响应到用户失败: %v", err)
						} else {
							log.Printf("成功转发Nginx响应 [%s] 到用户, 请求ID=%s", baseResp.Type, baseResp.RequestID)
						}
					}
				}

				// 发送响应到通道
				respChan, ok := respChanVal.(chan interface{})
				if ok {
					// 将Data字段解析为interface{}，以便接收任何类型
					var dataValue interface{}
					if err := json.Unmarshal(baseResp.Data, &dataValue); err != nil {
						log.Printf("解析Nginx响应数据失败: %v", err)
					} else {
						select {
						case respChan <- dataValue:
							log.Printf("成功发送Nginx响应数据到通道, 请求ID=%s", baseResp.RequestID)
						default:
							log.Printf("错误: Nginx请求ID=%s的响应通道已满或已关闭", baseResp.RequestID)
						}
					}
				}

				// 响应处理完成后从映射中删除
				dockerRequestMap.Delete(baseResp.RequestID)
				dockerResponseChannels.Delete(baseResp.RequestID)
				log.Printf("已清理Nginx请求ID=%s的映射和通道", baseResp.RequestID)
			} else {
				log.Printf("警告: 收到的Nginx响应消息没有请求ID")
			}

		case "file_list_response", "file_content_response", "file_tree_response", "file_upload_response",
			"docker_file_list", "docker_file_content", "docker_file_tree", "docker_file_upload":
			// 处理文件 / 容器文件操作响应
			var fileResponse struct {
				Type      string                 `json:"type"`
				RequestID string                 `json:"request_id"`
				Data      map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(message, &fileResponse); err != nil {
				log.Printf("解析文件响应消息失败: %v", err)
				continue
			}
			// 调用文件控制器的响应处理函数
			if fileResponse.RequestID != "" {
				HandleFileResponse(fileResponse.RequestID, map[string]interface{}{
					"type": fileResponse.Type,
					"data": fileResponse.Data,
				})
			}
		case "agent_upgrade_response", "agent_upgrade_status":
			// Agent 升级进度/结果回传，兼容两种消息格式：
			//   旧路径 (client.go)  → type="agent_upgrade_response", 数据在 "data" 字段
			//   新路径 (handler包)  → type="agent_upgrade_status",   数据在 "payload" 字段
			if !isAgent {
				continue
			}

			var upgradeResp struct {
				Type      string                 `json:"type"`
				RequestID string                 `json:"request_id"`
				Data      map[string]interface{} `json:"data"`
				Payload   map[string]interface{} `json:"payload"`
			}
			if err := json.Unmarshal(message, &upgradeResp); err != nil {
				log.Printf("解析Agent升级响应失败: %v", err)
				continue
			}

			// 统一取数据：优先从 data 取（旧路径），若为空则从 payload 取（新路径）
			upgradeData := upgradeResp.Data
			if len(upgradeData) == 0 {
				upgradeData = upgradeResp.Payload
			}
			if len(upgradeData) == 0 {
				log.Printf("收到空的Agent升级消息: server=%d request_id=%s type=%s", server.ID, upgradeResp.RequestID, upgradeResp.Type)
				continue
			}

			status, _ := upgradeData["status"].(string)
			msgText, _ := upgradeData["message"].(string)
			if status != "" || msgText != "" {
				log.Printf("收到Agent升级状态: server=%d request_id=%s status=%s message=%s", server.ID, upgradeResp.RequestID, status, msgText)
			} else {
				log.Printf("收到Agent升级响应: server=%d request_id=%s", server.ID, upgradeResp.RequestID)
			}

			// 推送升级状态到前端监控订阅者
			broadcastPublicMonitor(server.ID, map[string]interface{}{
				"type":       "agent_upgrade_status",
				"server_id":  server.ID,
				"request_id": upgradeResp.RequestID,
				"status":     status,
				"message":    msgText,
				"data":       upgradeData,
				"timestamp":  time.Now().Unix(),
			})
		default:
			log.Printf("未知的消息类型: %s", msg.Type)
			sendErrorMessage(conn, "未知的消息类型")
		}
	}
}

// 处理Shell命令
func handleShellCommand(conn *SafeConn, server *models.Server, payload json.RawMessage) {
	log.Printf("处理终端命令")

	// 解析命令数据
	var cmdData struct {
		Type        string   `json:"type"`
		Data        string   `json:"data"`    // 输入数据
		Session     string   `json:"session"` // 会话ID
		ContainerID string   `json:"container_id,omitempty"`
		Command     []string `json:"command,omitempty"`
	}

	if err := json.Unmarshal(payload, &cmdData); err != nil {
		log.Printf("解析Shell命令失败: %v", err)
		sendErrorMessage(conn, "命令格式错误")
		return
	}

	// 获取会话ID
	sessionID := cmdData.Session

	isDockerSession := cmdData.ContainerID != ""

	// 检查会话是否存在（仅处理input和resize类型的消息）
	if (cmdData.Type == "input" || cmdData.Type == "resize") && !isDockerSession {
		_, ok := terminalSessions.Load(sessionID)
		if !ok {
			log.Printf("会话不存在: %s", sessionID)
			sendErrorMessage(conn, "会话不存在或已过期")
			return
		}
	}

	// 如果是create类型的消息，确保保存当前用户连接到会话映射
	if cmdData.Type == "create" {
		log.Printf("创建终端会话 %s，存储用户连接", sessionID)
		ActiveTerminalConnections.Store(sessionID, conn)
	}

	// 如果是close类型的消息，清理会话资源
	if cmdData.Type == "close" && !isDockerSession {
		log.Printf("关闭终端会话: %s", sessionID)

		// 从活跃会话中删除
		ActiveTerminalConnections.Delete(sessionID)
		terminalSessions.Delete(sessionID)
	}

	payloadData := map[string]interface{}{
		"type":    cmdData.Type,
		"data":    cmdData.Data,
		"session": sessionID,
	}
	if cmdData.ContainerID != "" {
		payloadData["container_id"] = cmdData.ContainerID
	}
	if len(cmdData.Command) > 0 {
		payloadData["command"] = cmdData.Command
	}

	// 构建发送到Agent的消息
	agentMsg := map[string]interface{}{
		"type":    TypeShellCommand,
		"payload": payloadData,
	}

	// 发送到Agent
	// 通过ActiveAgentConnections查找该服务器的Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		log.Printf("服务器 %d 的Agent未连接", server.ID)

		// 使用新函数发送错误消息给用户
		sendTerminalError(sessionID, "服务器Agent未连接")
		return
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		log.Printf("服务器 %d 的连接类型错误", server.ID)

		// 使用新函数发送错误消息给用户
		sendTerminalError(sessionID, "服务器连接错误")
		return
	}

	// 发送命令到Agent
	if err := agentConn.WriteJSON(agentMsg); err != nil {
		log.Printf("发送命令到Agent失败: %v", err)

		// 使用新函数发送错误消息给用户
		sendTerminalError(sessionID, "发送命令失败")
		return
	}

	log.Printf("命令已发送到Agent")
}

// 处理文件列表
// 处理进程列表
func handleProcessList(conn *SafeConn, server *models.Server, payload json.RawMessage) {
	log.Printf("处理进程列表请求，服务器ID: %d", server.ID)

	// 解析请求
	var reqData struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(payload, &reqData); err != nil {
		log.Printf("解析进程列表请求参数失败: %v", err)
		sendErrorMessage(conn, "请求格式错误")
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "process_list",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"action": reqData.Action,
		},
	}

	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		log.Printf("服务器 %d 的Agent未连接", server.ID)
		sendErrorMessage(conn, "服务器Agent未连接")
		return
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		log.Printf("服务器 %d 的连接类型错误", server.ID)
		sendErrorMessage(conn, "服务器连接错误")
		return
	}

	// 创建响应通道并注册，以便后续处理响应
	responseChan := make(chan interface{}, 1)
	processResponseChannels.Store(requestID, responseChan)

	// 在函数返回时清理通道
	defer processResponseChannels.Delete(requestID)

	// 将用户连接与请求ID关联，以便将响应发送回用户
	processRequestMap.Store(requestID, conn)
	defer processRequestMap.Delete(requestID)

	// 发送消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		log.Printf("发送进程列表请求到Agent失败: %v", err)
		sendErrorMessage(conn, "发送请求到Agent失败")
		return
	}

	log.Printf("进程列表请求已发送到Agent，请求ID: %s", requestID)
}

// 处理进程终止
func handleProcessKill(conn *SafeConn, server *models.Server, payload json.RawMessage) {
	log.Printf("处理进程终止请求，服务器ID: %d", server.ID)

	// 解析请求
	var reqData struct {
		PID int32 `json:"pid"`
	}
	if err := json.Unmarshal(payload, &reqData); err != nil {
		log.Printf("解析进程终止请求参数失败: %v", err)
		sendErrorMessage(conn, "请求格式错误")
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "process_kill",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"pid": reqData.PID,
		},
	}

	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		log.Printf("服务器 %d 的Agent未连接", server.ID)
		sendErrorMessage(conn, "服务器Agent未连接")
		return
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		log.Printf("服务器 %d 的连接类型错误", server.ID)
		sendErrorMessage(conn, "服务器连接错误")
		return
	}

	// 创建响应通道并注册，以便后续处理响应
	responseChan := make(chan interface{}, 1)
	processResponseChannels.Store(requestID, responseChan)

	// 在函数返回时清理通道
	defer processResponseChannels.Delete(requestID)

	// 将用户连接与请求ID关联，以便将响应发送回用户
	processRequestMap.Store(requestID, conn)
	defer processRequestMap.Delete(requestID)

	// 发送消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		log.Printf("发送进程终止请求到Agent失败: %v", err)
		sendErrorMessage(conn, "发送请求到Agent失败")
		return
	}

	log.Printf("进程终止请求已发送到Agent，请求ID: %s", requestID)
}

// 处理Docker命令
func handleDockerCommand(conn *SafeConn, server *models.Server, payload json.RawMessage) {
	log.Printf("处理Docker命令，服务器ID: %d", server.ID)

	// 解析基本的Docker命令请求，获取命令类型和操作
	var reqData struct {
		Command string          `json:"command"`
		Action  string          `json:"action"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(payload, &reqData); err != nil {
		log.Printf("解析Docker命令请求参数失败: %v", err)
		sendErrorMessage(conn, "Docker请求格式错误")
		return
	}

	log.Printf("收到Docker命令请求: 命令=%s, 操作=%s", reqData.Command, reqData.Action)

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": reqData.Command,
			"action":  reqData.Action,
			"params":  json.RawMessage(reqData.Params),
		},
	}

	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		log.Printf("服务器 %d 的Agent未连接", server.ID)
		sendErrorMessage(conn, "服务器Agent未连接")
		return
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		log.Printf("服务器 %d 的连接类型错误", server.ID)
		sendErrorMessage(conn, "服务器连接错误")
		return
	}

	// 创建响应通道
	responseChan := make(chan interface{}, 1)
	dockerResponseChannels.Store(requestID, responseChan)

	// 在函数返回时清理通道
	defer dockerResponseChannels.Delete(requestID)

	// 将用户连接与请求ID关联，以便将响应发送回用户
	dockerRequestMap.Store(requestID, conn)
	defer dockerRequestMap.Delete(requestID)

	// 发送消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		log.Printf("发送Docker命令请求到Agent失败: %v", err)
		sendErrorMessage(conn, "发送请求到Agent失败")
		return
	}

	log.Printf("Docker命令请求已发送到Agent，请求ID: %s", requestID)
}

// handleDockerLogsStream 处理Docker日志流请求（用户 → Agent 转发）
func handleDockerLogsStream(conn *SafeConn, server *models.Server, payload json.RawMessage) {
	var reqData struct {
		Action   string `json:"action"`
		StreamID string `json:"stream_id"`
	}
	if err := json.Unmarshal(payload, &reqData); err != nil {
		log.Printf("解析日志流请求参数失败: %v", err)
		sendErrorMessage(conn, "日志流请求格式错误")
		return
	}

	log.Printf("收到日志流请求: action=%s, stream_id=%s, 服务器ID=%d", reqData.Action, reqData.StreamID, server.ID)

	if reqData.StreamID == "" {
		sendErrorMessage(conn, "日志流请求缺少 stream_id")
		return
	}

	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		log.Printf("服务器 %d 的Agent未连接", server.ID)
		sendErrorMessage(conn, "服务器Agent未连接")
		return
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		log.Printf("服务器 %d 的连接类型错误", server.ID)
		sendErrorMessage(conn, "服务器连接错误")
		return
	}

	// start: 注册用户连接映射，以便后续转发日志流数据
	if reqData.Action == "start" {
		ActiveLogStreamConnections.Store(reqData.StreamID, conn)
		log.Printf("已注册日志流 %s 的用户连接", reqData.StreamID)
	}

	// 构建转发给Agent的消息（保持原始 payload）
	agentMsg := map[string]interface{}{
		"type":    "docker_logs_stream",
		"payload": json.RawMessage(payload),
	}

	if err := agentConn.WriteJSON(agentMsg); err != nil {
		log.Printf("发送日志流请求到Agent失败: %v", err)
		sendErrorMessage(conn, "发送日志流请求到Agent失败")
		// 发送失败时清理映射
		if reqData.Action == "start" {
			ActiveLogStreamConnections.Delete(reqData.StreamID)
		}
		return
	}

	// stop: 清理用户连接映射
	if reqData.Action == "stop" {
		ActiveLogStreamConnections.Delete(reqData.StreamID)
		log.Printf("已清理日志流 %s 的用户连接映射", reqData.StreamID)
	}

	log.Printf("日志流请求已转发到Agent: action=%s, stream_id=%s", reqData.Action, reqData.StreamID)
}

// 发送错误消息
// 可选的 requestIDs 参数用于关联原始请求ID，便于前端追踪错误来源。
// 不传则自动生成新的请求ID。
func sendErrorMessage(conn *SafeConn, message string, requestIDs ...string) {
	reqID := generateRequestID()
	if len(requestIDs) > 0 && requestIDs[0] != "" {
		reqID = requestIDs[0]
	}

	errMsg := struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"`
		RequestID string `json:"request_id"`
	}{
		Type:      TypeError,
		Message:   message,
		Timestamp: time.Now().Unix(),
		RequestID: reqID,
	}

	log.Printf("发送错误消息 [%s]: %s", errMsg.RequestID, message)

	if err := conn.WriteJSON(errMsg); err != nil {
		log.Printf("发送错误消息失败 [%s]: %v", errMsg.RequestID, err)
	}
}

// 生成唯一的请求ID
// 【安全修复】使用UUID替代math/rand，提供更强的唯一性保证和密码学安全性
func generateRequestID() string {
	return uuid.New().String()
}

// 发送终端错误消息给特定会话的用户
func sendTerminalError(sessionID string, errMsg string) {
	// 查找对应会话的用户连接
	userConnVal, ok := ActiveTerminalConnections.Load(sessionID)
	if !ok {
		log.Printf("找不到会话 %s 的用户连接，无法发送错误消息", sessionID)
		return
	}

	userConn, ok := userConnVal.(*SafeConn)
	if !ok {
		log.Printf("会话 %s 的用户连接类型错误", sessionID)
		return
	}

	// 构建错误消息
	errResponse := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Error   string `json:"error"`
	}{
		Type:    "shell_error",
		Session: sessionID,
		Error:   errMsg,
	}

	// 发送错误消息
	if err := userConn.WriteJSON(errResponse); err != nil {
		log.Printf("发送终端错误消息失败: %v", err)
	} else {
		log.Printf("成功发送错误消息到会话 %s", sessionID)
	}
}

// 发送终端关闭消息给特定会话的用户
func sendTerminalClose(sessionID string) {
	// 查找对应会话的用户连接
	userConnVal, ok := ActiveTerminalConnections.Load(sessionID)
	if !ok {
		log.Printf("找不到会话 %s 的用户连接，无法发送关闭消息", sessionID)
		return
	}

	userConn, ok := userConnVal.(*SafeConn)
	if !ok {
		log.Printf("会话 %s 的用户连接类型错误", sessionID)
		return
	}

	// 构建关闭消息
	closeResponse := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Message string `json:"message"`
	}{
		Type:    "shell_close",
		Session: sessionID,
		Message: "终端会话已关闭",
	}

	// 发送关闭消息
	if err := userConn.WriteJSON(closeResponse); err != nil {
		log.Printf("发送终端关闭消息失败: %v", err)
	} else {
		log.Printf("成功发送关闭消息到会话 %s", sessionID)
	}

	// 从活跃会话中移除
	ActiveTerminalConnections.Delete(sessionID)
	terminalSessions.Delete(sessionID)
}

// 导出函数：获取ActiveAgentConnections中的agent连接
// 供utils.GetAgentConnectionFunc使用
func GetAgentConnection(serverID uint) (*websocket.Conn, error) {
	val, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器(ID: %d)未连接", serverID)
	}

	safeConn, ok := val.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器(ID: %d)连接类型错误", serverID)
	}

	if safeConn == nil || safeConn.Conn == nil {
		return nil, fmt.Errorf("服务器(ID: %d)连接为空", serverID)
	}

	// 检查连接是否存活
	err := safeConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
	if err != nil {
		// 连接已断开，从映射中移除
		ActiveAgentConnections.Delete(serverID)
		return nil, fmt.Errorf("服务器(ID: %d)连接已断开: %v", serverID, err)
	}

	return safeConn.Conn, nil
}

// 在package init函数中设置utils.GetAgentConnectionFunc
func init() {
	// 导入utils包
	utils.GetAgentConnectionFunc = GetAgentConnection
}

// requestTerminalWorkingDirectoryViaWebSocket 通过WebSocket获取终端当前工作目录
func requestTerminalWorkingDirectoryViaWebSocket(serverID uint, sessionID string) (string, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return "", fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return "", fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("cwd_%s_%d", sessionID, time.Now().UnixNano())

	// 创建 buffered(1) 响应通道，存入 dockerResponseChannels
	responseChan := make(chan map[string]interface{}, 1)
	dockerResponseChannels.Store(requestID, responseChan)
	defer dockerResponseChannels.Delete(requestID)

	// 注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造获取工作目录的消息
	request := map[string]interface{}{
		"type": "shell_command",
		"payload": map[string]interface{}{
			"type":    "get_cwd",
			"session": sessionID,
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		return "", fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-responseChan:
		if resp == nil {
			return "", fmt.Errorf("Agent连接已断开")
		}
		if workingDir, ok := resp["working_dir"].(string); ok {
			return workingDir, nil
		}
		return "", fmt.Errorf("无效的响应格式")
	case <-time.After(TimeoutTerminalCWD):
		return "", fmt.Errorf("请求超时")
	}
}
func toString(value interface{}, fallback string) string {
	if str, ok := value.(string); ok {
		return str
	}
	return fallback
}
