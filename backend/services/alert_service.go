package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

// 全局AlertService实例
var (
	globalAlertService *AlertService
	alertServiceOnce   sync.Once
)

// MetricState 指标状态缓存结构
type MetricState struct {
	Value      float64
	ExceedTime time.Time
	Alerted    bool
}

// AlertService 预警服务
type AlertService struct {
	metricStates map[string]map[uint]MetricState // 格式: map[metricType]map[serverID]state
	mu           sync.RWMutex                    // 用于保护metricStates的并发访问
	stopChan     chan struct{}
	testing      bool // 测试模式标志，用于单元测试
}

// NewAlertService 创建预警服务
func NewAlertService() *AlertService {
	return &AlertService{
		metricStates: make(map[string]map[uint]MetricState),
		stopChan:     make(chan struct{}),
	}
}

// GetAlertService 获取全局预警服务实例
func GetAlertService() *AlertService {
	alertServiceOnce.Do(func() {
		globalAlertService = NewAlertService()
	})
	return globalAlertService
}

// Start 启动预警服务
func (s *AlertService) Start() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Println("预警服务已启动")

	for {
		select {
		case <-ticker.C:
			s.checkAllServers()
		case <-s.stopChan:
			log.Println("预警服务已停止")
			return
		}
	}
}

// Stop 停止预警服务
func (s *AlertService) Stop() {
	close(s.stopChan)
}

// checkAllServers 检查所有服务器的指标
func (s *AlertService) checkAllServers() {
	if s.testing {
		return
	}

	// 获取全局预警设置
	globalSettings, err := models.GetGlobalAlertSettings()
	if err != nil {
		log.Printf("获取全局预警设置失败: %v", err)
		return
	}

	// 如果没有任何设置，跳过检查
	if len(globalSettings) == 0 {
		return
	}

	// 将设置转换为map便于查找
	settingsMap := make(map[string]models.AlertSetting)
	for _, setting := range globalSettings {
		if setting.Enabled {
			settingsMap[setting.Type] = setting
		}
	}

	// 获取所有活跃服务器
	servers, err := models.GetAllServers(0)
	if err != nil {
		log.Printf("获取服务器列表失败: %v", err)
		return
	}

	// 获取所有启用的通知渠道
	channels, err := models.GetEnabledNotificationChannels()
	if err != nil {
		log.Printf("获取通知渠道失败: %v", err)
		return
	}

	// 如果没有通知渠道，跳过检查
	if len(channels) == 0 {
		return
	}

	for _, server := range servers {
		// 获取服务器特定的预警设置(如果有)
		serverSettings, err := models.GetServerAlertSettings(server.ID)
		if err != nil {
			log.Printf("获取服务器 %d 预警设置失败: %v", server.ID, err)
			continue
		}

		// 合并全局设置和服务器特定设置
		settings := s.mergeSettings(settingsMap, serverSettings)

		// 检查状态变化预警 (新增)
		if statusSetting, ok := settings["status"]; ok {
			s.checkServerStatus(server, statusSetting, channels)
		}
		
		// 只对在线服务器检查资源指标
		if !server.Online {
			continue
		}

		// 获取最新监控数据
		latestData, err := models.GetLatestMonitorData(server.ID, 1)
		if err != nil || len(latestData) == 0 {
			continue
		}

		// 检查CPU指标
		if cpuSetting, ok := settings["cpu"]; ok {
			s.checkMetric("cpu", server, latestData[0].CPUUsage, cpuSetting, channels)
		}

		// 检查内存指标
		if memorySetting, ok := settings["memory"]; ok {
			// 计算内存使用百分比
			var memoryUsage float64
			if latestData[0].MemoryTotal > 0 {
				memoryUsage = float64(latestData[0].MemoryUsed) / float64(latestData[0].MemoryTotal) * 100
			}
			s.checkMetric("memory", server, memoryUsage, memorySetting, channels)
		}

		// 检查网络指标 (网络流量 MB/s)
		if networkSetting, ok := settings["network"]; ok {
			// 计算网络流量 (MB/s)
			networkTotal := (latestData[0].NetworkIn + latestData[0].NetworkOut) / 1024 / 1024
			s.checkMetric("network", server, networkTotal, networkSetting, channels)
		}
	}
}

// checkMetric 检查单个指标并触发预警
func (s *AlertService) checkMetric(
	metricType string,
	server models.Server,
	value float64,
	setting models.AlertSetting,
	channels []models.NotificationChannel,
) {
	// 加锁保护并发访问
	s.mu.Lock()
	defer s.mu.Unlock()

	// 初始化状态映射
	if _, ok := s.metricStates[metricType]; !ok {
		s.metricStates[metricType] = make(map[uint]MetricState)
	}

	// 获取当前状态
	state, exists := s.metricStates[metricType][server.ID]
	now := time.Now()

	// 检查是否超过阈值
	if value >= setting.Threshold {
		// 如果第一次超过阈值或已重置
		if !exists || state.ExceedTime.IsZero() {
			state.ExceedTime = now
			state.Value = value
			state.Alerted = false
			s.metricStates[metricType][server.ID] = state
			log.Printf("服务器 %s(%d) 的 %s 指标开始超过阈值: 当前值=%.2f, 阈值=%.2f", 
				server.Name, server.ID, metricType, value, setting.Threshold)
			return
		}

		// 如果持续时间已达到要求且未发送过预警
		duration := now.Sub(state.ExceedTime).Seconds()
		if duration >= float64(setting.Duration) && !state.Alerted {
			// 触发预警
			s.triggerAlert(metricType, server, value, setting, channels)
			state.Alerted = true
			state.Value = value
			s.metricStates[metricType][server.ID] = state
		}
	} else {
		// 如果已发送预警但现在恢复了，则记录恢复事件
		if exists && state.Alerted {
			s.resolveAlert(metricType, server, value)
		}
		// 重置状态
		state.ExceedTime = time.Time{}
		state.Alerted = false
		s.metricStates[metricType][server.ID] = state
	}
}

// triggerAlert 触发预警通知
func (s *AlertService) triggerAlert(
	metricType string,
	server models.Server,
	value float64,
	setting models.AlertSetting,
	channels []models.NotificationChannel,
) {
	log.Printf("触发预警: 服务器 %s(%d), 类型 %s, 值 %.2f, 阈值 %.2f",
		server.Name, server.ID, metricType, value, setting.Threshold)

	// 创建预警记录
	record := models.AlertRecord{
		ServerID:    server.ID,
		ServerName:  server.Name,
		AlertType:   metricType,
		Value:       value,
		Threshold:   setting.Threshold,
		Resolved:    false,
		NotifiedAt:  time.Now(),
	}

	// 收集成功通知的渠道ID
	var channelIDs []string
	for _, channel := range channels {
		// 发送通知
		if s.sendNotification(channel, record) {
			channelIDs = append(channelIDs, strconv.FormatUint(uint64(channel.ID), 10))
		}
	}

	record.ChannelIDs = strings.Join(channelIDs, ",")
	if err := models.CreateAlertRecord(&record); err != nil {
		log.Printf("保存预警记录失败: %v", err)
	}
}

// resolveAlert 记录预警解决
func (s *AlertService) resolveAlert(metricType string, server models.Server, value float64) {
	log.Printf("预警解除: 服务器 %s(%d), 类型 %s, 当前值 %.2f",
		server.Name, server.ID, metricType, value)

	// 查找最近的未解决预警
	record, err := models.GetLatestUnresolvedAlert(server.ID, metricType)
	if err != nil {
		log.Printf("查找未解决预警失败: %v", err)
		return
	}

	// 更新为已解决
	record.Resolved = true
	record.ResolvedAt = time.Now()
	if err := models.UpdateAlertRecord(record); err != nil {
		log.Printf("更新预警记录失败: %v", err)
	}

	// 如果有通知过的渠道，则发送解决通知
	if record.ChannelIDs != "" {
		channelIDs := strings.Split(record.ChannelIDs, ",")
		for _, idStr := range channelIDs {
			id, _ := strconv.ParseUint(idStr, 10, 64)
			var channel models.NotificationChannel
			if err := models.GetNotificationChannelByID(uint(id), &channel); err != nil {
				continue
			}
			s.sendResolutionNotification(channel, *record, value)
		}
	}
}

// sendNotification 发送通知
func (s *AlertService) sendNotification(channel models.NotificationChannel, alert models.AlertRecord) bool {
	var config map[string]string
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		log.Printf("解析通知配置失败: %v", err)
		return false
	}

	var title, content string
	switch alert.AlertType {
	case "cpu":
		title = fmt.Sprintf("服务器 %s CPU使用率预警", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的CPU使用率达到 %.2f%%, 超过预设阈值 %.2f%%",
			alert.ServerName, alert.Value, alert.Threshold)
	case "memory":
		title = fmt.Sprintf("服务器 %s 内存使用率预警", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的内存使用率达到 %.2f%%, 超过预设阈值 %.2f%%",
			alert.ServerName, alert.Value, alert.Threshold)
	case "network":
		title = fmt.Sprintf("服务器 %s 网络流量预警", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的网络流量达到 %.2f MB/s, 超过预设阈值 %.2f MB/s",
			alert.ServerName, alert.Value, alert.Threshold)
	case "test":
		title = fmt.Sprintf("服务器监控系统测试通知")
		content = fmt.Sprintf("这是一条测试通知，请忽略。测试值: %.2f, 测试阈值: %.2f",
			alert.Value, alert.Threshold)
	default:
		title = fmt.Sprintf("服务器 %s 预警通知", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的 %s 指标达到 %.2f, 超过预设阈值 %.2f",
			alert.ServerName, alert.AlertType, alert.Value, alert.Threshold)
	}

	// 根据通知渠道类型选择不同的发送方式
	switch channel.Type {
	case "email":
		return s.sendEmailNotification(config, title, content)
	case "serverchan":
		return s.sendServerChanNotification(config, title, content)
	default:
		log.Printf("不支持的通知类型: %s", channel.Type)
		return false
	}
}

// sendEmailNotification 发送邮件通知
func (s *AlertService) sendEmailNotification(config map[string]string, title, content string) bool {
	emailConfig := utils.ParseEmailConfig(config)
	
	// 构建HTML内容
	htmlContent := fmt.Sprintf(`
		<html>
		<head>
			<meta charset="utf-8">
			<style>
				body { font-family: Arial, sans-serif; line-height: 1.6; }
				.container { max-width: 600px; margin: 0 auto; padding: 20px; }
				.header { background-color: #f44336; color: white; padding: 10px; text-align: center; }
				.content { padding: 20px; background-color: #f9f9f9; }
				.footer { padding: 10px; text-align: center; font-size: 12px; color: #666; }
			</style>
		</head>
		<body>
			<div class="container">
				<div class="header">
					<h2>服务器监控预警通知</h2>
				</div>
				<div class="content">
					<h3>%s</h3>
					<p>%s</p>
					<p>通知时间: %s</p>
				</div>
				<div class="footer">
					<p>此邮件由服务器监控系统自动发送，请勿直接回复</p>
				</div>
			</div>
		</body>
		</html>
	`, title, content, time.Now().Format("2006-01-02 15:04:05"))

	// 优先使用管理员（个人资料）邮箱作为收件人；若未设置则回退到通知渠道配置中的 to_email
	recipients := make([]string, 0, 4)
	if adminEmails, err := models.GetAdminEmails(); err == nil && len(adminEmails) > 0 {
		recipients = append(recipients, adminEmails...)
	} else if err != nil {
		log.Printf("获取管理员邮箱失败: %v", err)
	}

	// fallback: 使用配置中的 to_email（兼容历史配置）
	if len(recipients) == 0 {
		to := strings.TrimSpace(emailConfig.ToEmail)
		if to != "" {
			// 支持简单的逗号/分号分隔
			split := strings.FieldsFunc(to, func(r rune) bool {
				return r == ',' || r == ';'
			})
			for _, item := range split {
				item = strings.TrimSpace(item)
				if item != "" {
					recipients = append(recipients, item)
				}
			}
		}
	}

	// 去重
	seen := make(map[string]struct{}, len(recipients))
	uniqueRecipients := make([]string, 0, len(recipients))
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		uniqueRecipients = append(uniqueRecipients, r)
	}
	recipients = uniqueRecipients

	if len(recipients) == 0 {
		log.Printf("邮件通知发送失败：未找到收件人邮箱，请先在“个人资料”中设置管理员邮箱")
		return false
	}

	successCount := 0
	for _, recipient := range recipients {
		cfg := emailConfig
		cfg.ToEmail = recipient
		if err := utils.SendEmail(cfg, title, htmlContent); err != nil {
			log.Printf("发送邮件通知失败(收件人=%s): %v", recipient, err)
			continue
		}
		successCount++
	}

	if successCount == 0 {
		return false
	}

	log.Printf("邮件通知发送成功: %s (收件人数量=%d)", title, successCount)
	return true
}

// sendServerChanNotification 发送Server酱通知
func (s *AlertService) sendServerChanNotification(config map[string]string, title, content string) bool {
	sendkey, ok := config["sendkey"]
	if !ok {
		log.Printf("Server酱缺少sendkey配置")
		return false
	}

	resp, err := utils.ServerChanSend(sendkey, title, content)
	if err != nil {
		log.Printf("发送Server酱通知失败: %v", err)
		return false
	}

	log.Printf("Server酱通知发送成功: %v", resp)
	return true
}

// sendResolutionNotification 发送解决通知
func (s *AlertService) sendResolutionNotification(channel models.NotificationChannel, alert models.AlertRecord, currentValue float64) bool {
	var config map[string]string
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		log.Printf("解析通知配置失败: %v", err)
		return false
	}

	var title, content string
	switch alert.AlertType {
	case "cpu":
		title = fmt.Sprintf("服务器 %s CPU使用率已恢复", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的CPU使用率已恢复至 %.2f%%, 低于预设阈值 %.2f%%",
			alert.ServerName, currentValue, alert.Threshold)
	case "memory":
		title = fmt.Sprintf("服务器 %s 内存使用率已恢复", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的内存使用率已恢复至 %.2f%%, 低于预设阈值 %.2f%%",
			alert.ServerName, currentValue, alert.Threshold)
	case "network":
		title = fmt.Sprintf("服务器 %s 网络流量已恢复", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的网络流量已恢复至 %.2f MB/s, 低于预设阈值 %.2f MB/s",
			alert.ServerName, currentValue, alert.Threshold)
	default:
		title = fmt.Sprintf("服务器 %s 预警已解除", alert.ServerName)
		content = fmt.Sprintf("服务器 %s 的 %s 指标已恢复至 %.2f, 低于预设阈值 %.2f",
			alert.ServerName, alert.AlertType, currentValue, alert.Threshold)
	}

	switch channel.Type {
	case "email":
		return s.sendEmailNotification(config, title, content)
	case "serverchan":
		return s.sendServerChanNotification(config, title, content)
	default:
		return false
	}
}

// SendTestNotification 发送测试通知
func (s *AlertService) SendTestNotification(channel models.NotificationChannel, alert models.AlertRecord) bool {
	return s.sendNotification(channel, alert)
}

// mergeSettings 合并全局设置和服务器特定设置
func (s *AlertService) mergeSettings(global map[string]models.AlertSetting, serverSettings []models.AlertSetting) map[string]models.AlertSetting {
	result := make(map[string]models.AlertSetting)

	// 首先复制全局设置
	for k, v := range global {
		result[k] = v
	}

	// 用服务器特定设置覆盖全局设置
	for _, setting := range serverSettings {
		if setting.Enabled {
			result[setting.Type] = setting
		}
	}

	return result
} 

// 新增方法：检查服务器状态变化
func (s *AlertService) checkServerStatus(
	server models.Server,
	setting models.AlertSetting,
	channels []models.NotificationChannel,
) {
	// 加锁保护并发访问
	s.mu.Lock()
	defer s.mu.Unlock()

	// 初始化状态映射，如果不存在
	if _, ok := s.metricStates["status"]; !ok {
		s.metricStates["status"] = make(map[uint]MetricState)
	}

	// 获取旧状态
	oldState, exists := s.metricStates["status"][server.ID]
	
	// 首次检测到服务器，记录其状态
	if !exists {
		s.metricStates["status"][server.ID] = MetricState{
			Value: func() float64 {
				if server.Online {
					return 1.0
				}
				return 0.0
			}(),
			ExceedTime: time.Time{},
			Alerted: false,
		}
		return
	}
	
	// 检查状态是否变化
	currentStatus := func() float64 {
		if server.Online {
			return 1.0
		}
		return 0.0
	}()
	
	// 状态变化或者状态持续超过阈值时处理
	now := time.Now()
	
	if oldState.Value == 1.0 && currentStatus == 0.0 {
		// 状态从在线变为离线
		log.Printf("服务器 %s(ID:%d) 状态变化: 在线 -> 离线", server.Name, server.ID)
		// 记录开始离线的时间
		s.metricStates["status"][server.ID] = MetricState{
			Value: 0.0,
			ExceedTime: now, // 记录开始离线时间
			Alerted: false,
		}
	} else if oldState.Value == 0.0 && currentStatus == 1.0 {
		// 状态从离线变为在线
		log.Printf("服务器 %s(ID:%d) 状态变化: 离线 -> 在线", server.Name, server.ID)
		
		// 处理上线通知（如果设置了上线通知）
		if setting.Threshold == 1 || setting.Threshold == 3 {
			s.sendStatusAlert(server, setting, channels, true)
		}
		
		// 重置状态
		s.metricStates["status"][server.ID] = MetricState{
			Value: 1.0,
			ExceedTime: time.Time{},
			Alerted: false,
		}
	} else if currentStatus == 0.0 && !oldState.Alerted && !oldState.ExceedTime.IsZero() {
		// 服务器持续离线，检查是否超过持续时间阈值
		// 如果设置了离线报警（阈值为2或3）
		if setting.Threshold == 2 || setting.Threshold == 3 {
			duration := now.Sub(oldState.ExceedTime).Seconds()
			// 如果离线持续时间超过阈值，触发报警
			if duration >= float64(setting.Duration) {
				log.Printf("服务器 %s(ID:%d) 离线持续时间 %.2f 秒，超过阈值 %d 秒，触发报警",
					server.Name, server.ID, duration, setting.Duration)
				
				s.sendStatusAlert(server, setting, channels, false)
				
				// 更新状态，标记已经发送报警
				s.metricStates["status"][server.ID] = MetricState{
					Value: 0.0,
					ExceedTime: oldState.ExceedTime,
					Alerted: true,
				}
			}
		}
	}
}

// 发送状态报警通知
func (s *AlertService) sendStatusAlert(
	server models.Server,
	setting models.AlertSetting,
	channels []models.NotificationChannel,
	isOnline bool,
) {
	alertType := "status"
	alertValue := 0.0
	if isOnline {
		alertValue = 1.0
	}
	
	// 创建预警记录
	record := models.AlertRecord{
		ServerID:    server.ID,
		ServerName:  server.Name,
		AlertType:   alertType,
		Value:       alertValue,
		Threshold:   setting.Threshold,
		Resolved:    true, // 状态预警不需要解决
		ResolvedAt:  time.Now(),
		NotifiedAt:  time.Now(),
	}

	// 收集成功通知的渠道ID
	var channelIDs []string
	for _, channel := range channels {
		if s.sendStatusNotification(channel, record, isOnline) {
			channelIDs = append(channelIDs, strconv.FormatUint(uint64(channel.ID), 10))
		}
	}
	
	// 记录通知渠道
	record.ChannelIDs = strings.Join(channelIDs, ",")
	
	// 保存预警记录
	if err := models.CreateAlertRecord(&record); err != nil {
		log.Printf("保存状态预警记录失败: %v", err)
	}
}

// 新增方法：发送服务器状态变化通知
func (s *AlertService) sendStatusNotification(
	channel models.NotificationChannel, 
	alert models.AlertRecord,
	isOnline bool,
) bool {
	status := "上线"
	if !isOnline {
		status = "离线"
	}
	title := fmt.Sprintf("【服务器%s】%s", status, alert.ServerName)
	content := fmt.Sprintf("服务器 %s (ID: %d) 已%s，请关注。\n时间: %s", 
		alert.ServerName, 
		alert.ServerID, 
		status,
		time.Now().Format("2006-01-02 15:04:05"))
	
	// 解析配置
	config, err := channel.GetChannelConfig()
	if err != nil {
		log.Printf("解析通知渠道配置失败: %v", err)
		return false
	}
	
	// 根据渠道类型发送通知
	switch channel.Type {
	case "email":
		return s.sendEmailNotification(config, title, content)
	case "serverchan":
		return s.sendServerChanNotification(config, title, content)
	default:
		log.Printf("不支持的通知渠道类型: %s", channel.Type)
		return false
	}
} 
 
