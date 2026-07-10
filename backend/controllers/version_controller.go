package controllers

import (
	"fmt"
	"net/http"
	"runtime"
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
	servers, err := models.GetAllServers()
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
		if services.IsServerOnline(server, time.Now()) {
			status = 1
		} else {
			status = 0
		}

		serverVersions = append(serverVersions, gin.H{
			"id":            server.ID,
			"name":          server.Name,
			"host":          server.IP,
			"agentVersion":  server.AgentVersion,
			"agentType":     server.AgentType,
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
	type safeReleaseAsset struct {
		Name string `json:"name"`
		OS   string `json:"os,omitempty"`
		Arch string `json:"arch,omitempty"`
		Size int64  `json:"size"`
	}
	assets := make([]safeReleaseAsset, 0, len(info.Assets))
	for _, asset := range info.Assets {
		assets = append(assets, safeReleaseAsset{
			Name: asset.Name,
			OS:   asset.OS,
			Arch: asset.Arch,
			Size: asset.Size,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"version":      info.Version,
		"name":         info.Name,
		"notes":        info.Notes,
		"publishedAt":  info.PublishedAt,
		"assets":       assets,
		"release_repo": settings.AgentReleaseRepo,
		"channel":      info.Channel,
	})
}
