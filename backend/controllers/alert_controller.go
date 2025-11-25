package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
)

// GetAlertSettings 获取预警设置
func GetAlertSettings(c *gin.Context) {
	serverID, _ := strconv.ParseUint(c.DefaultQuery("server_id", "0"), 10, 64)

	var settings []models.AlertSetting
	var err error

	if serverID > 0 {
		// 获取特定服务器的设置
		settings, err = models.GetServerAlertSettings(uint(serverID))
	} else {
		// 获取全局设置
		settings, err = models.GetGlobalAlertSettings()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取预警设置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"settings": settings})
}

// CreateAlertSetting 创建预警设置
func CreateAlertSetting(c *gin.Context) {
	var setting models.AlertSetting
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证字段
	if setting.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预警类型不能为空"})
		return
	}

	if setting.Type != "cpu" && setting.Type != "memory" && setting.Type != "network" && setting.Type != "status" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预警类型必须是cpu、memory、network或status"})
		return
	}

	if setting.Threshold <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "阈值必须大于0"})
		return
	}

	if setting.Duration <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "持续时间必须大于0秒"})
		return
	}

	if err := models.CreateAlertSetting(&setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建预警设置失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "预警设置创建成功",
		"setting": setting,
	})
}

// UpdateAlertSetting 更新预警设置
func UpdateAlertSetting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的设置ID"})
		return
	}

	var setting models.AlertSetting
	if err := models.GetAlertSettingByID(uint(id), &setting); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预警设置不存在"})
		return
	}

	// 保存旧值
	oldType := setting.Type
	oldServerID := setting.ServerID

	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证字段
	if setting.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预警类型不能为空"})
		return
	}

	if setting.Type != "cpu" && setting.Type != "memory" && setting.Type != "network" && setting.Type != "status" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预警类型必须是cpu、memory、network或status"})
		return
	}

	if setting.Threshold <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "阈值必须大于0"})
		return
	}

	if setting.Duration <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "持续时间必须大于0秒"})
		return
	}

	// 防止修改不应该修改的字段
	setting.ID = uint(id)
	setting.Type = oldType         // 不允许修改预警类型
	setting.ServerID = oldServerID // 不允许修改服务器ID

	if err := models.UpdateAlertSetting(&setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新预警设置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "预警设置更新成功",
		"setting": setting,
	})
}

// DeleteAlertSetting 删除预警设置
func DeleteAlertSetting(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的设置ID"})
		return
	}

	if err := models.DeleteAlertSetting(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除预警设置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "预警设置删除成功"})
}

// GetNotificationChannels 获取通知渠道
func GetNotificationChannels(c *gin.Context) {
	onlyEnabled := c.DefaultQuery("enabled", "false") == "true"

	var channels []models.NotificationChannel
	var err error

	if onlyEnabled {
		channels, err = models.GetEnabledNotificationChannels()
	} else {
		channels, err = models.GetAllNotificationChannels()
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取通知渠道失败"})
		return
	}

	// 清理敏感信息
	for i := range channels {
		// 检查配置是否为空
		if channels[i].Config != "" {
			var configMap map[string]string
			if err := json.Unmarshal([]byte(channels[i].Config), &configMap); err == nil {
				// 移除密码等敏感信息
				if _, ok := configMap["password"]; ok {
					configMap["password"] = "******"
				}
				if _, ok := configMap["sendkey"]; ok {
					configMap["sendkey"] = "******"
				}
				// 重新序列化
				if newConfig, err := json.Marshal(configMap); err == nil {
					channels[i].Config = string(newConfig)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"channels": channels})
}

// CreateNotificationChannel 创建通知渠道
func CreateNotificationChannel(c *gin.Context) {
	var channel models.NotificationChannel
	if err := c.ShouldBindJSON(&channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证字段
	if channel.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道名称不能为空"})
		return
	}

	if channel.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道类型不能为空"})
		return
	}

	if channel.Type != "email" && channel.Type != "serverchan" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "渠道类型必须是email或serverchan"})
		return
	}

	// 验证配置
	var configMap map[string]string
	if err := json.Unmarshal([]byte(channel.Config), &configMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "配置格式无效，必须是JSON对象"})
		return
	}

	// 根据类型验证必要的配置项
	switch channel.Type {
	case "email":
		requiredFields := []string{"smtp_host", "username", "password", "from_email", "to_email"}
		for _, field := range requiredFields {
			if _, ok := configMap[field]; !ok || configMap[field] == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "邮件配置缺少必要字段: " + field})
				return
			}
		}
	case "serverchan":
		if _, ok := configMap["sendkey"]; !ok || configMap["sendkey"] == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Server酱配置缺少sendkey"})
			return
		}
	}

	if err := models.CreateNotificationChannel(&channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建通知渠道失败"})
		return
	}

	// 返回数据前清理敏感信息
	var returnConfig map[string]string
	json.Unmarshal([]byte(channel.Config), &returnConfig)

	if _, ok := returnConfig["password"]; ok {
		returnConfig["password"] = "******"
	}
	if _, ok := returnConfig["sendkey"]; ok {
		returnConfig["sendkey"] = "******"
	}

	if cleanConfig, err := json.Marshal(returnConfig); err == nil {
		channel.Config = string(cleanConfig)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "通知渠道创建成功",
		"channel": channel,
	})
}

// UpdateNotificationChannel 更新通知渠道
func UpdateNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	var channel models.NotificationChannel
	if err := models.GetNotificationChannelByID(uint(id), &channel); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "通知渠道不存在"})
		return
	}

	// 保存原配置，以备需要合并
	var originalConfig map[string]string
	json.Unmarshal([]byte(channel.Config), &originalConfig)

	// 读取请求体
	var updateData struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Config  string `json:"config"`
		Enabled bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 更新字段
	if updateData.Name != "" {
		channel.Name = updateData.Name
	}

	// 类型不能更改
	if updateData.Type != "" && updateData.Type != channel.Type {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能更改通知渠道类型"})
		return
	}

	// 更新启用状态
	channel.Enabled = updateData.Enabled

	// 处理配置更新
	if updateData.Config != "" && updateData.Config != "[UNCHANGED]" {
		var newConfig map[string]string
		if err := json.Unmarshal([]byte(updateData.Config), &newConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "配置格式无效"})
			return
		}

		// 合并配置 - 保留未修改的字段
		for k, v := range newConfig {
			originalConfig[k] = v
		}

		// 根据类型验证必要的配置项
		switch channel.Type {
		case "email":
			requiredFields := []string{"smtp_host", "username", "from_email", "to_email"}
			for _, field := range requiredFields {
				if _, ok := originalConfig[field]; !ok || originalConfig[field] == "" {
					c.JSON(http.StatusBadRequest, gin.H{"error": "邮件配置缺少必要字段: " + field})
					return
				}
			}
			// 如果密码字段为空或占位符，恢复原密码
			if pass, ok := originalConfig["password"]; !ok || pass == "" || pass == "******" {
				var oldConfig map[string]string
				json.Unmarshal([]byte(channel.Config), &oldConfig)
				if oldPass, ok := oldConfig["password"]; ok {
					originalConfig["password"] = oldPass
				}
			}
		case "serverchan":
			// 如果sendkey字段为空或占位符，恢复原sendkey
			if key, ok := originalConfig["sendkey"]; !ok || key == "" || key == "******" {
				var oldConfig map[string]string
				json.Unmarshal([]byte(channel.Config), &oldConfig)
				if oldKey, ok := oldConfig["sendkey"]; ok {
					originalConfig["sendkey"] = oldKey
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Server酱配置缺少sendkey"})
					return
				}
			}
		}

		// 更新配置
		configJSON, _ := json.Marshal(originalConfig)
		channel.Config = string(configJSON)
	}

	if err := models.UpdateNotificationChannel(&channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新通知渠道失败"})
		return
	}

	// 返回数据前清理敏感信息
	var returnConfig map[string]string
	json.Unmarshal([]byte(channel.Config), &returnConfig)

	if _, ok := returnConfig["password"]; ok {
		returnConfig["password"] = "******"
	}
	if _, ok := returnConfig["sendkey"]; ok {
		returnConfig["sendkey"] = "******"
	}

	if cleanConfig, err := json.Marshal(returnConfig); err == nil {
		channel.Config = string(cleanConfig)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "通知渠道更新成功",
		"channel": channel,
	})
}

// DeleteNotificationChannel 删除通知渠道
func DeleteNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	if err := models.DeleteNotificationChannel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "通知渠道删除成功"})
}

// TestNotificationChannel 测试通知渠道
func TestNotificationChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	var channel models.NotificationChannel
	if err := models.GetNotificationChannelByID(uint(id), &channel); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "通知渠道不存在"})
		return
	}

	// 创建测试记录
	testRecord := models.AlertRecord{
		ServerID:   0,
		ServerName: "测试服务器",
		AlertType:  "test",
		Value:      95,
		Threshold:  80,
		Resolved:   false,
		NotifiedAt: time.Now(),
	}

	// 发送测试通知
	alertService := services.GetAlertService()
	success := alertService.SendTestNotification(channel, testRecord)

	if success {
		c.JSON(http.StatusOK, gin.H{"message": "测试通知发送成功"})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试通知发送失败"})
	}
}

// GetAlertRecords 获取预警记录
func GetAlertRecords(c *gin.Context) {
	serverID, _ := strconv.ParseUint(c.DefaultQuery("server_id", "0"), 10, 64)
	alertType := c.DefaultQuery("type", "")
	onlyUnresolved := c.DefaultQuery("unresolved", "false") == "true"

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	records, total, err := models.GetAlertRecords(uint(serverID), alertType, onlyUnresolved, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取预警记录失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// ResolveAlertRecord 手动解决预警记录
func ResolveAlertRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的记录ID"})
		return
	}

	var record models.AlertRecord
	if err := models.GetAlertRecordByID(uint(id), &record); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预警记录不存在"})
		return
	}

	if record.Resolved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预警记录已经解决"})
		return
	}

	record.Resolved = true
	record.ResolvedAt = time.Now()

	if err := models.UpdateAlertRecord(&record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新预警记录失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "预警记录已标记为已解决",
		"record":  record,
	})
}
 