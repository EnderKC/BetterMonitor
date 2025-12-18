package controllers

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
)

// 生成随机密钥
func generateRandomKey() string {
	bytes := make([]byte, 16)
	if _, err := cryptorand.Read(bytes); err != nil {
		return "fallback-secret-key"
	}
	return hex.EncodeToString(bytes)
}

// CreateServer 创建新服务器
func CreateServer(c *gin.Context) {
	// 解析请求数据，处理字段映射
	var createData struct {
		Name        string `json:"name"`
		Notes       string `json:"notes"`       // 前端发送的字段名
		Description string `json:"description"` // 也支持直接的description字段
		Tags        string `json:"tags"`
	}

	if err := c.ShouldBindJSON(&createData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证服务器名称
	if createData.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务器名称不能为空"})
		return
	}

	// 创建服务器对象
	server := models.Server{
		Name:      createData.Name,
		Tags:      createData.Tags,
		SecretKey: generateRandomKey(), // 自动生成随机密钥
		Status:    "offline",           // 设置默认状态
	}

	// 处理描述字段的映射：优先使用notes字段，如果为空则使用description字段
	if createData.Notes != "" {
		server.Description = createData.Notes
	} else if createData.Description != "" {
		server.Description = createData.Description
	}

	if err := models.CreateServer(&server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务器失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "服务器创建成功",
		"server":  server,
	})
}

// GetAllServers 获取所有服务器
func GetAllServers(c *gin.Context) {
	servers, err := models.GetAllServers(0) // 传入0表示获取所有服务器
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

// GetServer 获取单个服务器详情
func GetServer(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"server": server})
}

// GetServerStatus 获取服务器状态（公开API，不需要认证）
func GetServerStatus(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的服务器ID",
		})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "服务器不存在",
		})
		return
	}

	// 检查服务器是否真正在线 - 使用Online字段和心跳时间双重判断
	isOnline := server.Online && time.Since(server.LastHeartbeat) <= 15*time.Second

	// 如果数据库状态不一致，确保更新数据库
	if isOnline != (server.Status == "online") {
		status := "offline"
		if isOnline {
			status = "online"
		}
		// 异步更新数据库状态，不阻塞API响应
		go models.UpdateServerStatusOnly(server.ID, status)
	}

	// 确定返回的状态
	serverStatus := "offline"
	if isOnline {
		serverStatus = "online"
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"online":         isOnline,
		"status":         serverStatus,
		"last_heartbeat": server.LastHeartbeat,
		"name":           server.Name,
	})
}

// GetPublicServerMonitor 获取服务器监控历史数据（公开API，不需要认证）
func GetPublicServerMonitor(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}
	_ = server // 服务器存在即可，不需要额外检查

	// 获取查询参数
	hoursStr := c.DefaultQuery("hours", "1")
	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 1
	}

	// 限制最大查询时间为24小时
	if hours > 24 {
		hours = 24
	}

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hours) * time.Hour)

	// 获取监控数据
	data, err := models.GetServerMonitorData(id, startTime, endTime)
	if err != nil {
		log.Printf("[ERROR] 获取服务器ID=%d公开监控数据失败: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取监控数据失败"})
		return
	}

	log.Printf("[DEBUG] 公开监控数据查询: server_id=%d, hours=%d, 数据条数=%d", id, hours, len(data))

	// 如果数据量太大，进行采样
	if len(data) > 500 {
		data = sampleMonitorData(data, 500)
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}

// UpdateServer 更新服务器信息
func UpdateServer(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 解析请求数据，处理字段映射
	var updateData struct {
		Name        string `json:"name"`
		Notes       string `json:"notes"`       // 前端发送的字段名
		Description string `json:"description"` // 也支持直接的description字段
		Tags        string `json:"tags"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 更新服务器字段
	if updateData.Name != "" {
		server.Name = updateData.Name
	}

	// 处理描述字段的映射：优先使用notes字段，如果为空则使用description字段
	if updateData.Notes != "" {
		server.Description = updateData.Notes
	} else if updateData.Description != "" {
		server.Description = updateData.Description
	}

	if updateData.Tags != "" {
		server.Tags = updateData.Tags
	}

	// 保持ID不变
	server.ID = id

	// 更新服务器
	if err := models.UpdateServer(server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "服务器更新成功",
		"server":  server,
	})
}

// DeleteServer 删除服务器
func DeleteServer(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	if err := models.DeleteServer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除服务器失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "服务器删除成功"})
}

// ReportMonitorData 接收服务器监控数据
func ReportMonitorData(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 验证密钥
	secretKey := c.GetHeader("X-Secret-Key")
	if secretKey != server.SecretKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的密钥"})
		return
	}

	// 解析监控数据
	var monitorData MonitorPayload
	if err := c.ShouldBindJSON(&monitorData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的监控数据"})
		return
	}

	record, err := persistMonitorPayload(server, &monitorData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存监控数据失败"})
		return
	}

	if snapshot := buildMonitorData(server, record); snapshot != nil {
		broadcastPublicMonitor(server.ID, snapshot)
	}

	c.JSON(http.StatusOK, gin.H{"message": "监控数据上报成功"})
}

// GetServerMonitor 获取服务器监控数据
func GetServerMonitor(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取查询参数
	startTimeStr := c.DefaultQuery("start_time", "")
	endTimeStr := c.DefaultQuery("end_time", "")
	limitStr := c.DefaultQuery("limit", "100")

	var startTime, endTime time.Time
	var limit int

	// 如果没有指定开始和结束时间，则使用系统设置中的历史数据时间范围
	if startTimeStr == "" && endTimeStr == "" {
		// 获取系统设置
		settings, err := models.GetSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取系统设置失败"})
			return
		}

		// 默认为当前时间和过去设定的小时数
		endTime = time.Now()
		startTime = endTime.Add(-time.Duration(settings.ChartHistoryHours) * time.Hour)
	} else {
		// 解析开始时间和结束时间
		if startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无效的开始时间格式"})
				return
			}
		} else {
			// 默认查询最近24小时
			startTime = time.Now().Add(-24 * time.Hour)
		}

		if endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "无效的结束时间格式"})
				return
			}
		} else {
			endTime = time.Now()
		}
	}

	// 记录时间范围，帮助调试
	log.Printf("[DEBUG] 服务器监控查询时间范围: server_id=%d, start=%v, end=%v",
		id, startTime, endTime)

	// 解析限制数
	limit, err = strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	var data []models.ServerMonitor

	// 仅返回真实监控数据（无数据时返回空数组）
	data, err = models.GetServerMonitorData(uint(id), startTime, endTime)
	if err != nil {
		log.Printf("[ERROR] 获取服务器ID=%d监控数据失败: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取监控数据失败"})
		return
	}

	log.Printf("[DEBUG] 查询到服务器ID=%d的监控数据 %d 条", id, len(data))

	// 如果数据量太大，需要进行采样
	if len(data) > 1000 {
		data = sampleMonitorData(data, 1000)
		log.Printf("[DEBUG] 采样后剩余数据点: %d", len(data))
	}

	// 为调试目的，打印第一条数据
	if len(data) > 0 {
		sampleData := data[0]
		log.Printf("[DEBUG] 数据样例: 时间=%v, CPU=%v%%, 内存=%v/%vMB, 网络流量=%v/%vB/s",
			sampleData.Timestamp,
			sampleData.CPUUsage,
			sampleData.MemoryUsed/(1024*1024),
			sampleData.MemoryTotal/(1024*1024),
			sampleData.NetworkIn,
			sampleData.NetworkOut)
	}

	// 返回数据
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// sampleMonitorData 对监控数据进行采样，减少数据点数量
func sampleMonitorData(data []models.ServerMonitor, targetPoints int) []models.ServerMonitor {
	dataLen := len(data)
	if dataLen <= targetPoints {
		return data
	}

	// 计算采样间隔
	interval := float64(dataLen) / float64(targetPoints)
	sampledData := make([]models.ServerMonitor, 0, targetPoints)

	for i := 0; i < targetPoints; i++ {
		idx := int(float64(i) * interval)
		if idx >= dataLen {
			idx = dataLen - 1
		}
		sampledData = append(sampledData, data[idx])
	}

	return sampledData
}

// RegisterServer 处理Agent自动注册
func RegisterServer(c *gin.Context) {
	// 获取并验证令牌
	token := c.GetHeader("X-Register-Token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "注册令牌不能为空"})
		return
	}

	// 查找匹配的服务器
	servers, err := models.GetAllServers(0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	var matchedServer *models.Server
	for _, server := range servers {
		if server.SecretKey == token {
			matchedServer = &server
			break
		}
	}

	// 如果没有找到匹配的服务器
	if matchedServer == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "无效的注册令牌，未找到匹配的服务器"})
		return
	}

	// 获取客户端IP地址
	clientIP := c.ClientIP()

	// 如果IP地址为空或变更，则更新IP地址
	if matchedServer.IP != clientIP {
		matchedServer.IP = clientIP
	}

	// 使用统一的方法更新服务器状态和心跳时间
	if err := models.UpdateServerHeartbeatAndStatus(matchedServer.ID, "online"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器状态失败"})
		return
	}

	// 更新服务器IP地址
	if err := models.DB.Model(&models.Server{}).Where("id = ?", matchedServer.ID).Update("ip", clientIP).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器IP地址失败"})
		return
	}

	// 异步更新国家代码
	go updateServerCountry(matchedServer.ID, clientIP)

	// 返回服务器信息
	c.JSON(http.StatusOK, gin.H{
		"message":    "注册成功",
		"server_id":  matchedServer.ID,
		"secret_key": matchedServer.SecretKey,
	})
}

// UpdateAgentSystemInfo 处理Agent上报系统信息
func UpdateAgentSystemInfo(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 验证密钥
	secretKey := c.GetHeader("X-Secret-Key")
	if secretKey != server.SecretKey {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的密钥"})
		return
	}

	// 解析请求体
	var systemInfoData map[string]interface{}
	if err := c.ShouldBindJSON(&systemInfoData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的系统信息数据"})
		return
	}

	// 添加日志，打印接收到的系统信息
	log.Printf("[DEBUG] 收到服务器ID=%d的系统信息: %+v", id, systemInfoData)

	// 将系统信息转换为JSON字符串
	systemInfoJSON, err := json.Marshal(systemInfoData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "处理系统信息失败"})
		return
	}

	// 输出系统信息的JSON字符串
	log.Printf("[DEBUG] 系统信息JSON字符串: %s", systemInfoJSON)

	// 获取客户端IP地址
	clientIP := strings.TrimSpace(c.ClientIP())
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
			log.Printf("[DEBUG] 更新PublicIP字段: %s", publicIP)
		}
	}

	// 从systemInfoData中提取各个字段并更新server对象
	if os, ok := systemInfoData["os"].(string); ok && os != "" {
		server.OS = os
		log.Printf("[DEBUG] 更新OS字段: %s", os)
	}

	if platform, ok := systemInfoData["platform"].(string); ok && platform != "" {
		server.Arch = systemInfoData["kernel_arch"].(string) // 使用kernel_arch更新arch字段
		log.Printf("[DEBUG] 更新Arch字段: %s", server.Arch)
	}

	if cpuCores, ok := systemInfoData["cpu_cores"].(float64); ok && cpuCores > 0 {
		server.CPUCores = int(cpuCores)
		log.Printf("[DEBUG] 更新CPUCores字段: %d", server.CPUCores)
	}

	if cpuModel, ok := systemInfoData["cpu_model"].(string); ok && cpuModel != "" {
		server.CPUModel = cpuModel
		log.Printf("[DEBUG] 更新CPUModel字段: %s", cpuModel)
	}

	if memoryTotal, ok := systemInfoData["memory_total"].(float64); ok && memoryTotal > 0 {
		server.MemoryTotal = int64(memoryTotal)
		log.Printf("[DEBUG] 更新MemoryTotal字段: %d", server.MemoryTotal)
	}

	if hostname, ok := systemInfoData["hostname"].(string); ok && hostname != "" {
		server.Hostname = hostname
		log.Printf("[DEBUG] 更新Hostname字段: %s", hostname)
	}

	// 从系统信息中提取磁盘总量并更新
	if diskTotal, ok := systemInfoData["disk_total"].(float64); ok && diskTotal > 0 {
		server.DiskTotal = int64(diskTotal)
		log.Printf("[DEBUG] 更新DiskTotal字段: %d", server.DiskTotal)
	}

	// 更新服务器信息
	server.SystemInfo = string(systemInfoJSON)

	// 使用统一的方法更新服务器状态和心跳时间
	if err := models.UpdateServerHeartbeatAndStatus(server.ID, "online"); err != nil {
		log.Printf("[ERROR] 更新服务器状态失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器状态失败"})
		return
	}

	// 更新服务器系统信息和IP
	updates := map[string]interface{}{
		"system_info":  server.SystemInfo,
		"ip":           server.IP,
		"public_ip":    server.PublicIP,
		"os":           server.OS,
		"arch":         server.Arch,
		"cpu_cores":    server.CPUCores,
		"cpu_model":    server.CPUModel,
		"memory_total": server.MemoryTotal,
		"disk_total":   server.DiskTotal,
		"hostname":     server.Hostname,
	}

	if err := models.DB.Model(&models.Server{}).Where("id = ?", server.ID).Updates(updates).Error; err != nil {
		log.Printf("[ERROR] 更新服务器信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器信息失败"})
		return
	}

	geoIP := server.PublicIP
	shouldUpdateCountry := publicIPChanged && geoIP != ""
	if !shouldUpdateCountry && server.PublicIP == "" && clientIPChanged {
		geoIP = server.IP
		shouldUpdateCountry = geoIP != ""
	}
	if shouldUpdateCountry {
		go updateServerCountry(server.ID, geoIP)
	}

	log.Printf("[INFO] 服务器ID=%d的系统信息已成功更新", id)
	c.JSON(http.StatusOK, gin.H{"message": "系统信息已更新"})
}

// updateServerCountry 更新服务器国家代码
func updateServerCountry(serverID uint, ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil || parsedIP.IsLoopback() || parsedIP.IsPrivate() {
		return
	}

	resp, err := http.Get("http://ip-api.com/json/" + ip)
	if err != nil {
		log.Printf("GeoIP查询失败: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取GeoIP响应失败: %v", err)
		return
	}

	var result struct {
		CountryCode string `json:"countryCode"`
		Status      string `json:"status"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("解析GeoIP响应失败: %v", err)
		return
	}

	if result.Status == "success" && result.CountryCode != "" {
		if err := models.DB.Model(&models.Server{}).Where("id = ?", serverID).Update("country_code", result.CountryCode).Error; err != nil {
			log.Printf("更新服务器国家代码失败: %v", err)
		} else {
			log.Printf("更新服务器 %d 国家代码为 %s", serverID, result.CountryCode)
		}
	}
}

// ReorderServers 批量更新服务器顺序
func ReorderServers(c *gin.Context) {
	// 解析请求数据
	var requestData struct {
		OrderedIDs []uint `json:"orderedIds" binding:"required"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证数据
	if len(requestData.OrderedIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务器ID列表不能为空"})
		return
	}

	// 检查是否有重复的 ID
	idSet := make(map[uint]bool)
	for _, id := range requestData.OrderedIDs {
		if idSet[id] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "服务器ID列表包含重复项"})
			return
		}
		idSet[id] = true
	}

	// 验证所有 ID 是否都存在于数据库中
	var count int64
	if err := models.DB.Model(&models.Server{}).Where("id IN ?", requestData.OrderedIDs).Count(&count).Error; err != nil {
		log.Printf("验证服务器ID失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "验证服务器ID失败"})
		return
	}

	if int(count) != len(requestData.OrderedIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部分服务器ID不存在"})
		return
	}

	// 调用模型层方法更新顺序
	if err := models.ReorderServers(requestData.OrderedIDs); err != nil {
		log.Printf("更新服务器顺序失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器顺序失败"})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "服务器顺序更新成功",
	})
}
