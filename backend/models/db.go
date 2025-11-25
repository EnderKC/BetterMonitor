package models

import (
	"log"
	"os"
	"time"

	"github.com/user/server-ops-backend/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB() error {
	cfg := config.LoadConfig()

	// 创建数据目录（如果不存在）
	dir := "./data"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// 配置GORM日志
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Info,
			Colorful:      true,
		},
	)

	// 连接SQLite数据库
	db, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return err
	}

	DB = db

	// 自动迁移数据库结构
	if err := DB.AutoMigrate(
		&User{},
		&Server{},
		&ServerMonitor{},
		&SystemSettings{},
		&AlertSetting{},
		&NotificationChannel{},
		&AlertRecord{},
		&CertificateAccount{},
		&ManagedCertificate{},
	); err != nil {
		return err
	}

	// 检查是否需要创建管理员账户
	var count int64
	DB.Model(&User{}).Count(&count)
	if count == 0 {
		// 创建默认管理员用户
		adminUser := User{
			Username: "admin",
			Password: HashPassword("admin123"), // 默认密码，建议首次登录后修改
			Role:     "admin",
		}
		if err := DB.Create(&adminUser).Error; err != nil {
			log.Printf("创建默认管理员失败: %v", err)
		} else {
			log.Println("已创建默认管理员账户，用户名: admin, 密码: admin123")
		}
	}

	// 检查是否需要创建默认系统设置
	var settingsCount int64
	DB.Model(&SystemSettings{}).Count(&settingsCount)
	if settingsCount == 0 {
		// 创建默认系统设置
		settings := SystemSettings{
			HeartbeatInterval: "10s",
			MonitorInterval:   "30s",
			UIRefreshInterval: "10s",
			DataRetentionDays: 7,
		}
		if err := DB.Create(&settings).Error; err != nil {
			log.Printf("创建默认系统设置失败: %v", err)
		} else {
			log.Println("已创建默认系统设置")
		}
	}

	return nil
}
