package controllers

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/pkg/version"
	"github.com/user/server-ops-backend/services"
)

// HealthCheck 健康检查端点
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"uptime":    time.Since(startTime).String(),
	})
}

// 启动时间
var startTime = time.Now()

var agentUpgradeSender = func(conn *SafeConn, payload map[string]interface{}) error {
	if conn == nil {
		return fmt.Errorf("连接不存在")
	}
	return conn.WriteJSON(payload)
}

// GetDashboardVersion 获取Dashboard版本信息
func GetDashboardVersion(c *gin.Context) {
	versionInfo := version.GetVersion()
	c.JSON(http.StatusOK, versionInfo)
}

// GetSystemInfo 获取系统信息（包含详细的系统信息）
func GetSystemInfo(c *gin.Context) {
	// 获取Dashboard版本信息
	dashboardVersion := version.GetVersion()

	// 获取内存信息
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 格式化内存大小
	formatMemory := func(bytes uint64) string {
		const unit = 1024
		if bytes < unit {
			return fmt.Sprintf("%d B", bytes)
		}
		div, exp := uint64(unit), 0
		for n := bytes / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
	}

	systemInfo := gin.H{
		"version":     dashboardVersion.Version,
		"buildTime":   dashboardVersion.BuildDate,
		"goVersion":   dashboardVersion.GoVersion,
		"startTime":   startTime.Format(time.RFC3339),
		"uptime":      time.Since(startTime).String(),
		"osInfo":      fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH),
		"arch":        runtime.GOARCH,
		"cpuCount":    runtime.NumCPU(),
		"memoryTotal": formatMemory(m.Sys),
	}

	c.JSON(http.StatusOK, systemInfo)
}

// GetServerVersions 获取指定服务器的版本信息
func GetServerVersions(c *gin.Context) {
	servers, err := models.GetAllServers(0) // 传入0表示获取所有服务器
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取服务器列表失败",
		})
		return
	}

	var serverVersions []gin.H
	for _, server := range servers {
		var status int
		if server.Online {
			status = 1
		} else {
			status = 0
		}

		serverVersions = append(serverVersions, gin.H{
			"id":            server.ID,
			"name":          server.Name,
			"host":          server.IP,
			"agentVersion":  server.AgentVersion,
			"status":        status,
			"lastHeartbeat": server.LastHeartbeat,
		})
	}

	c.JSON(http.StatusOK, serverVersions)
}

// GetLatestAgentRelease 获取最新的Agent发布信息
func GetLatestAgentRelease(c *gin.Context) {
	settings, err := models.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取系统设置失败: %v", err),
		})
		return
	}

	info, err := services.FetchLatestAgentRelease(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取最新版本失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"version":      info.Version,
		"name":         info.Name,
		"notes":        info.Notes,
		"publishedAt":  info.PublishedAt,
		"assets":       info.Assets,
		"release_repo": settings.AgentReleaseRepo,
	})
}

// ForceAgentUpgrade 强制升级多个Agent
func ForceAgentUpgrade(c *gin.Context) {
	var req struct {
		ServerIDs     []uint64 `json:"serverIds" binding:"required"`
		TargetVersion string   `json:"targetVersion"`
		Channel       string   `json:"channel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || len(req.ServerIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误",
		})
		return
	}

	settings, err := models.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取系统设置失败: %v", err),
		})
		return
	}

	upgradeChannel := strings.TrimSpace(req.Channel)
	if upgradeChannel == "" {
		upgradeChannel = settings.AgentReleaseChannel
	}

	targetVersion := strings.TrimSpace(req.TargetVersion)
	if targetVersion == "" {
		if releaseInfo, err := services.FetchLatestAgentRelease(settings); err == nil && releaseInfo != nil {
			targetVersion = releaseInfo.Version
		}
	}
	if targetVersion == "" {
		targetVersion = version.GetVersion().Version
	}

	result := struct {
		Success []uint64 `json:"success"`
		Failure []uint64 `json:"failure"`
		Offline []uint64 `json:"offline"`
		Missing []uint64 `json:"missing"`
	}{
		Success: []uint64{},
		Failure: []uint64{},
		Offline: []uint64{},
		Missing: []uint64{},
	}

	for _, id := range req.ServerIDs {
		server, err := models.GetServerByID(uint(id))
		if err != nil {
			result.Missing = append(result.Missing, id)
			continue
		}
		if !server.Online {
			result.Offline = append(result.Offline, id)
			continue
		}

		connVal, ok := ActiveAgentConnections.Load(server.ID)
		if !ok {
			result.Offline = append(result.Offline, id)
			continue
		}

		conn, ok := connVal.(*SafeConn)
		if !ok {
			result.Failure = append(result.Failure, id)
			continue
		}

		requestID := fmt.Sprintf("upgrade-%d-%d", server.ID, time.Now().UnixNano())
		command := map[string]interface{}{
			"type":       "agent_upgrade",
			"request_id": requestID,
			"payload": map[string]interface{}{
				"action":         "upgrade",
				"target_version": targetVersion,
				"channel":        upgradeChannel,
				"server_id":      server.ID,
			},
		}

		if err := conn.WriteJSON(command); err != nil {
			result.Failure = append(result.Failure, id)
		} else {
			result.Success = append(result.Success, id)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       fmt.Sprintf("升级指令已触发，目标版本: %s", targetVersion),
		"targetVersion": targetVersion,
		"channel":       upgradeChannel,
		"result":        result,
	})
}
