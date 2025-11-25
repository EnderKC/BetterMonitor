package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
