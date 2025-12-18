package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/controllers"
	"github.com/user/server-ops-backend/middleware"
)

// SetupRoutes 配置API路由
func SetupRoutes(r *gin.Engine) {
	// 静态文件服务
	r.Static("/static", "./static")

	// 前端文件服务（SPA模式）
	r.NoRoute(func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// 健康检查
	r.GET("/health", controllers.HealthCheck)
	r.HEAD("/health", controllers.HealthCheck)

	// 根路径健康检查（用于Agent延迟检测）
	r.GET("/", controllers.HealthCheck)
	r.HEAD("/", controllers.HealthCheck)

	// 添加不带前缀的WebSocket路由，便于客户端连接
	r.GET("/servers/:id/ws", controllers.WebSocketHandler)
	// 添加前端当前使用的WebSocket路由路径
	r.GET("/ws/:id/server", controllers.WebSocketHandler)

	// API路由组
	api := r.Group("/api")
	{
		// 不需要认证的路由
		// 登录
		api.POST("/login", controllers.Login)

		// 公开的服务器监控数据 (探针页面)
		api.GET("/servers/public/ws", controllers.PublicServersWebSocketHandler)

		// 公开的WebSocket接口 (探针页面，不需要鉴权)
		api.GET("/servers/public/:id/ws", controllers.PublicWebSocketHandler)

		// 公开的服务器状态API (前端检查状态)
		api.GET("/servers/:id/status", controllers.GetServerStatus)

		// 公开的服务器监控历史数据API (探针页面使用)
		api.GET("/servers/public/:id/monitor", controllers.GetPublicServerMonitor)

		// 公开的前端设置API (探针页面使用)
		api.GET("/public/settings", controllers.GetPublicSettings)

		// 生命探针公开接口
		api.GET("/life-probes/public", controllers.GetPublicLifeProbes)
		api.GET("/life-probes/public/:id/details", controllers.GetPublicLifeProbeDetails)
		api.GET("/life-probes/public/ws", controllers.LifeProbeListWebSocketHandler)
		api.GET("/life-probes/public/:id/ws", controllers.LifeProbeDetailWebSocketHandler)

		// 版本信息API (公开，无需认证)
		api.GET("/version", controllers.GetDashboardVersion)
		api.GET("/health", controllers.HealthCheck)

		// Agent 获取配置接口
		api.GET("/servers/:id/settings", controllers.GetAgentSettings)

		// WebSocket接口（支持Secret Key认证）
		api.GET("/servers/:id/ws", controllers.WebSocketHandler)
		api.GET("/servers/:id/monitor-ws", controllers.WebSocketHandler)
		api.GET("/ws/:id/server", controllers.WebSocketHandler)

		// LifeLogger数据采集接口
		api.POST("/life-logger/events", controllers.IngestLifeLoggerEvent)

		// 需要JWT认证的路由
		auth := api.Group("/")
		auth.Use(middleware.JWTAuthMiddleware())
		{
			// 用户相关
			auth.GET("/profile", controllers.GetProfile)
			auth.PUT("/profile", controllers.UpdateProfile)
			auth.POST("/change-password", controllers.ChangePassword)

			// 服务器管理
			auth.GET("/servers", controllers.GetAllServers)
			auth.GET("/servers/:id", controllers.GetServer)
			auth.POST("/servers", controllers.CreateServer)
			auth.PUT("/servers/:id/update", controllers.UpdateServer)
			auth.DELETE("/servers/:id", controllers.DeleteServer)
			auth.PUT("/servers/reorder", controllers.ReorderServers)

			// 监控数据
			auth.GET("/servers/:id/monitor", controllers.GetServerMonitor)

			// 生命探针管理
			auth.GET("/life-probes", controllers.ListLifeProbes)
			auth.GET("/life-probes/:id", controllers.GetLifeProbe)
			auth.POST("/life-probes", controllers.CreateLifeProbe)
			auth.PUT("/life-probes/:id", controllers.UpdateLifeProbe)
			auth.DELETE("/life-probes/:id", controllers.DeleteLifeProbe)
			auth.GET("/life-probes/:id/details", controllers.GetLifeProbeDetails)

			// 版本管理
			auth.GET("/system/info", controllers.GetSystemInfo)
			auth.GET("/servers/versions", controllers.GetServerVersions)

			// Agent升级管理
			auth.GET("/agents/releases/latest", controllers.GetLatestAgentRelease)
			auth.POST("/servers/upgrade", controllers.ForceAgentUpgrade)

			// 终端会话管理
			auth.GET("/servers/:id/terminal/sessions", controllers.GetTerminalSessions)
			auth.POST("/servers/:id/terminal/sessions", controllers.CreateTerminalSession)
			auth.DELETE("/servers/:id/terminal/sessions/:session_id", controllers.DeleteTerminalSession)
			auth.GET("/servers/:id/terminal/sessions/:session_id/cwd", controllers.GetTerminalWorkingDirectory)

			// 文件管理API
			auth.GET("/servers/:id/files", controllers.GetFileList)
			auth.GET("/servers/:id/files/tree", controllers.GetFileTree)
			auth.GET("/servers/:id/files/children", controllers.GetDirectoryChildren)
			auth.GET("/servers/:id/files/content", controllers.GetFileContent)
			auth.PUT("/servers/:id/files/content", controllers.SaveFileContent)
			auth.POST("/servers/:id/files/create", controllers.CreateFile)
			auth.POST("/servers/:id/files/mkdir", controllers.CreateDirectory)
			auth.POST("/servers/:id/files/upload", controllers.UploadFile)
			auth.GET("/servers/:id/files/download", controllers.DownloadFile)
			auth.POST("/servers/:id/files/delete", controllers.DeleteFiles)

			// 进程管理API
			auth.GET("/servers/:id/processes", controllers.GetProcesses)
			auth.DELETE("/servers/:id/processes/:pid", controllers.KillProcess)

			// Docker管理API
			auth.GET("/servers/:id/docker/containers", controllers.GetContainers)
			auth.GET("/servers/:id/docker/containers/:container_id/logs", controllers.GetContainerLogs)
			auth.POST("/servers/:id/docker/containers/:container_id/start", controllers.StartContainer)
			auth.POST("/servers/:id/docker/containers/:container_id/stop", controllers.StopContainer)
			auth.POST("/servers/:id/docker/containers/:container_id/restart", controllers.RestartContainer)
			auth.DELETE("/servers/:id/docker/containers/:container_id", controllers.RemoveContainer)
			auth.POST("/servers/:id/docker/containers", controllers.CreateContainer)

			// 容器文件管理
			auth.GET("/servers/:id/docker/containers/:container_id/files", controllers.GetContainerFileList)
			auth.GET("/servers/:id/docker/containers/:container_id/files/children", controllers.GetContainerDirectoryChildren)
			auth.GET("/servers/:id/docker/containers/:container_id/files/content", controllers.GetContainerFileContent)
			auth.PUT("/servers/:id/docker/containers/:container_id/files/content", controllers.SaveContainerFileContent)
			auth.POST("/servers/:id/docker/containers/:container_id/files/create", controllers.CreateContainerFile)
			auth.POST("/servers/:id/docker/containers/:container_id/files/mkdir", controllers.CreateContainerDirectory)
			auth.POST("/servers/:id/docker/containers/:container_id/files/upload", controllers.UploadContainerFile)
			auth.GET("/servers/:id/docker/containers/:container_id/files/download", controllers.DownloadContainerFile)
			auth.POST("/servers/:id/docker/containers/:container_id/files/delete", controllers.DeleteContainerFiles)

			auth.GET("/servers/:id/docker/images", controllers.GetImages)
			auth.POST("/servers/:id/docker/images/pull", controllers.PullImage)
			auth.DELETE("/servers/:id/docker/images/:id", controllers.RemoveImage)

			auth.GET("/servers/:id/docker/composes", controllers.GetComposes)
			auth.GET("/servers/:id/docker/composes/:name/config", controllers.GetComposeConfig)
			auth.POST("/servers/:id/docker/composes/:name/up", controllers.ComposeUp)
			auth.POST("/servers/:id/docker/composes/:name/down", controllers.ComposeDown)
			auth.DELETE("/servers/:id/docker/composes/:name", controllers.RemoveCompose)
			auth.POST("/servers/:id/docker/composes", controllers.CreateCompose)

			// Nginx管理API
			auth.GET("/servers/:id/nginx/configs", controllers.NginxConfigsList)
			auth.GET("/servers/:id/nginx/configs/:config_id/content", controllers.NginxConfigContent)
			auth.PUT("/servers/:id/nginx/configs/:config_id", controllers.SaveNginxConfig)
			auth.POST("/servers/:id/nginx/configs", controllers.CreateNginxConfig)
			auth.DELETE("/servers/:id/nginx/configs/:config_id", controllers.DeleteNginxConfig)
			auth.GET("/servers/:id/nginx/logs", controllers.NginxLogsList)
			auth.GET("/servers/:id/nginx/logs/:log_id/content", controllers.NginxLogContent)
			auth.GET("/servers/:id/nginx/logs/:log_id/download", controllers.DownloadNginxLog)
			auth.POST("/servers/:id/nginx/restart", controllers.RestartNginx)
			auth.POST("/servers/:id/nginx/stop", controllers.StopNginx)
			auth.POST("/servers/:id/nginx/start", controllers.StartNginx)
			auth.GET("/servers/:id/nginx/test", controllers.TestNginxConfig)
			auth.GET("/servers/:id/nginx/processes", controllers.GetNginxProcesses)
			auth.GET("/servers/:id/nginx/ports", controllers.GetNginxPorts)
			auth.GET("/servers/:id/websites", controllers.ListWebsites)
			auth.GET("/servers/:id/nginx/openresty/status", controllers.OpenRestyStatus)
			auth.POST("/servers/:id/nginx/openresty/install", controllers.InstallOpenResty)
			auth.POST("/servers/:id/websites", controllers.ApplyWebsiteConfig)
			auth.POST("/servers/:id/websites/ssl", controllers.IssueWebsiteCertificate)
			auth.POST("/servers/:id/nginx/declarative/apply", controllers.ApplyWebsiteConfig)
			auth.POST("/servers/:id/nginx/declarative/ssl", controllers.IssueWebsiteCertificate)
			auth.GET("/servers/:id/cert/accounts", controllers.ListCertificateAccounts)
			auth.POST("/servers/:id/cert/accounts", controllers.CreateCertificateAccount)
			auth.DELETE("/servers/:id/cert/accounts/:account_id", controllers.DeleteCertificateAccount)
			auth.GET("/servers/:id/certificates", controllers.ListManagedCertificates)
			auth.GET("/servers/:id/certificates/:cert_id/content", controllers.GetCertificateContent)
			auth.DELETE("/servers/:id/certificates/:cert_id", controllers.DeleteManagedCertificate)

			// 需要管理员权限的路由
			admin := auth.Group("/admin")
			admin.Use(middleware.AdminAuthMiddleware())
			{
				// 用户管理
				admin.POST("/users", controllers.Register)

				// 系统设置管理
				admin.GET("/settings", controllers.GetSystemSettings)
				admin.PUT("/settings", controllers.UpdateSystemSettings)

				// 数据库统计信息
				admin.GET("/database/stats", controllers.GetDatabaseStats)

				// 其他管理员功能
			}

			// 预警通知相关API
			alerts := auth.Group("/alerts")
			{
				// 预警设置
				alerts.GET("/settings", controllers.GetAlertSettings)
				alerts.POST("/settings", controllers.CreateAlertSetting)
				alerts.PUT("/settings/:id", controllers.UpdateAlertSetting)
				alerts.DELETE("/settings/:id", controllers.DeleteAlertSetting)

				// 通知渠道
				alerts.GET("/channels", controllers.GetNotificationChannels)
				alerts.POST("/channels", controllers.CreateNotificationChannel)
				alerts.PUT("/channels/:id", controllers.UpdateNotificationChannel)
				alerts.DELETE("/channels/:id", controllers.DeleteNotificationChannel)
				alerts.POST("/channels/:id/test", controllers.TestNotificationChannel)

				// 预警记录
				alerts.GET("/records", controllers.GetAlertRecords)
				alerts.PUT("/records/:id/resolve", controllers.ResolveAlertRecord)
			}
		}
	}
}
