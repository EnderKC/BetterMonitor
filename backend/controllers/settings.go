package controllers

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/config"
	"github.com/user/server-ops-backend/models"
)

// GetSystemSettings 获取系统设置
func GetSystemSettings(c *gin.Context) {
	settings, err := models.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取系统设置失败: " + err.Error(),
		})
		return
	}

	// 直接返回设置对象，前端期望直接获取到settings字段
	c.JSON(http.StatusOK, settings)
}

// UpdateSystemSettings 更新系统设置
func UpdateSystemSettings(c *gin.Context) {
	var settings models.SystemSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的请求数据: " + err.Error(),
		})
		return
	}

	// 验证并保存设置
	if err := models.SaveSettings(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "保存设置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "系统设置已更新",
		"data":    settings,
	})
}

// GetAgentSettings 获取Agent设置接口 (供Agent使用)
func GetAgentSettings(c *gin.Context) {
	serverId := c.Param("id")

	// 验证服务器是否存在
	server, err := models.GetServer(serverId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "服务器不存在",
		})
		return
	}

	// 获取系统设置
	settings, err := models.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取系统设置失败",
		})
		return
	}

	// 返回Agent相关设置
	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"server_id":             server.ID,
		"heartbeat_interval":    settings.HeartbeatInterval,
		"monitor_interval":      settings.MonitorInterval,
		"agent_release_repo":    settings.AgentReleaseRepo,
		"agent_release_channel": settings.AgentReleaseChannel,
		"agent_release_mirror":  settings.AgentReleaseMirror,
	})
}

// GetPublicSettings 获取前端公共设置接口 (无需验证)
func GetPublicSettings(c *gin.Context) {
	// 获取系统设置
	settings, err := models.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取系统设置失败",
		})
		return
	}

	// 直接返回前端需要的设置，不包装在response对象中
	c.JSON(http.StatusOK, gin.H{
		"ui_refresh_interval":   settings.UIRefreshInterval,
		"chart_history_hours":   settings.ChartHistoryHours,
		"agent_release_repo":    settings.AgentReleaseRepo,
		"agent_release_channel": settings.AgentReleaseChannel,
		"agent_release_mirror":  settings.AgentReleaseMirror,
	})
}

// TableStats 表统计信息
type TableStats struct {
	Name         string `json:"name"`
	RecordCount  int64  `json:"record_count"`
	DisplayName  string `json:"display_name"`
	Category     string `json:"category"`
	SizeEstimate string `json:"size_estimate,omitempty"` // 估算大小（仅供参考）
}

// DatabaseStats 数据库统计信息
type DatabaseStats struct {
	FilePath      string       `json:"file_path"`
	FileSizeBytes int64        `json:"file_size_bytes"`
	FileSizeMB    float64      `json:"file_size_mb"`
	TotalRecords  int64        `json:"total_records"`
	Tables        []TableStats `json:"tables"`
}

// GetDatabaseStats 获取数据库统计信息
func GetDatabaseStats(c *gin.Context) {
	cfg := config.LoadConfig()
	dbPath := cfg.DBPath

	// 获取数据库文件信息
	fileInfo, err := os.Stat(dbPath)
	if err != nil {
		log.Printf("获取数据库文件信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取数据库信息失败: " + err.Error(),
		})
		return
	}

	fileSize := fileInfo.Size()
	fileSizeMB := float64(fileSize) / 1024 / 1024

	// 定义需要统计的表及其元数据
	tableConfigs := []struct {
		tableName   string
		displayName string
		category    string
	}{
		// 服务器监控相关
		{"servers", "服务器列表", "服务器监控"},
		{"server_monitors", "服务器监控数据", "服务器监控"},
		{"alert_records", "告警记录", "服务器监控"},

		// 生命探针相关
		{"life_probes", "生命探针", "生命探针"},
		{"life_logger_events", "探针事件日志", "生命探针"},
		{"life_heart_rates", "心率记录", "生命探针"},
		{"life_step_samples", "步数详情", "生命探针"},
		{"life_step_daily_totals", "每日步数汇总", "生命探针"},
		{"life_sleep_segments", "睡眠片段", "生命探针"},

		// 系统配置相关
		{"users", "用户", "系统配置"},
		{"system_settings", "系统设置", "系统配置"},
		{"alert_settings", "告警配置", "系统配置"},
		{"notification_channels", "通知渠道", "系统配置"},

		// 证书管理相关
		{"certificate_accounts", "证书账户", "证书管理"},
		{"managed_certificates", "托管证书", "证书管理"},
	}

	var tables []TableStats
	var totalRecords int64

	for _, config := range tableConfigs {
		// 检查表是否存在
		if !models.DB.Migrator().HasTable(config.tableName) {
			continue
		}

		var count int64
		if err := models.DB.Table(config.tableName).Count(&count).Error; err != nil {
			log.Printf("统计表 %s 失败: %v", config.tableName, err)
			continue
		}

		tables = append(tables, TableStats{
			Name:        config.tableName,
			DisplayName: config.displayName,
			Category:    config.category,
			RecordCount: count,
		})

		totalRecords += count
	}

	stats := DatabaseStats{
		FilePath:      dbPath,
		FileSizeBytes: fileSize,
		FileSizeMB:    fileSizeMB,
		TotalRecords:  totalRecords,
		Tables:        tables,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
