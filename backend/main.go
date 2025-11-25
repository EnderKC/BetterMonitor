package main

import (
	"log"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/config"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/routes"
	"github.com/user/server-ops-backend/services"
)

// 定期检查服务器状态
func startServerStatusChecker() {
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		for range ticker.C {
			servers, err := models.GetAllServers(0)
			if err != nil {
				log.Printf("获取服务器列表失败: %v", err)
				continue
			}

			for i := range servers {
				models.CheckServerStatus(&servers[i])
			}
			log.Println("已完成服务器状态检查")
		}
	}()
}

// 启动预警服务
func startAlertService() *services.AlertService {
	alertService := services.GetAlertService()
	go alertService.Start()
	return alertService
}

// 启动数据清理服务
func startDataCleanupService() {
	// 每天凌晨3点执行数据清理
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	go func() {
		defer ticker.Stop()

		log.Println("数据清理服务已启动")

		// 启动时立即执行一次清理
		cleanupOldData()

		for range ticker.C {
			now := time.Now()
			// 只在凌晨3点执行清理（避免频繁执行）
			if now.Hour() == 3 && now.Minute() < 5 {
				cleanupOldData()
			}
		}
	}()
}

// 清理过期监控数据
func cleanupOldData() {
	log.Println("开始清理过期监控数据...")

	// 获取系统设置
	settings, err := models.GetSettings()
	if err != nil {
		log.Printf("获取系统设置失败，使用默认保留天数7天: %v", err)
		settings = &models.SystemSettings{DataRetentionDays: 7}
	}

	retention := settings.DataRetentionDays
	if retention <= 0 {
		retention = 7 // 默认保留7天
	}

	// 计算截止时间
	cutoff := time.Now().AddDate(0, 0, -retention)

	log.Printf("清理 %s 之前的监控数据（保留%d天）", cutoff.Format("2006-01-02 15:04:05"), retention)

	// 执行清理
	if err := models.DeleteServerMonitorDataBefore(cutoff); err != nil {
		log.Printf("清理过期监控数据失败: %v", err)
	} else {
		log.Printf("成功清理 %s 之前的过期监控数据", cutoff.Format("2006-01-02 15:04:05"))
	}
}

func main() {
	// 初始化配置
	cfg := config.LoadConfig()

	// 初始化数据库
	if err := models.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 启动服务器状态检查器
	startServerStatusChecker()

	// 启动预警服务
	alertService := startAlertService()
	defer alertService.Stop()

	// 启动数据清理服务
	startDataCleanupService()

	// 创建Gin引擎
	r := gin.Default()

	// 配置跨域
	r.Use(config.CorsMiddleware())
	// 启用Gzip压缩
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// 设置路由
	routes.SetupRoutes(r)

	// 启动服务器
	log.Printf("服务器启动在端口 %s...\n", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
