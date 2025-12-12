package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Migration: 删除废弃的生命探针相关表
// 日期: 2024-12-12
// 描述: 移除 energy_detailed, focus_status, screen_event 相关的表和数据
//
// 废弃表:
// - life_focus_events (专注状态记录)
// - life_screen_events (屏幕使用记录)
// - life_energy_samples (能量值详情)
// - life_energy_daily_totals (每日能量汇总)
//
// 注意: 此迁移不可逆，请确保已备份数据库

func main() {
	// 从环境变量或参数获取数据库路径
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/monitor.db"
	}
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	log.Printf("开始数据库迁移: %s", dbPath)

	// 检查数据库文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("数据库文件不存在: %s", dbPath)
	}

	// 连接数据库
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 获取底层 SQL 连接
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接失败: %v", err)
	}
	defer sqlDB.Close()

	// 开始迁移
	log.Println("=" + string(make([]byte, 60)) + "=")
	log.Println("开始执行迁移: 删除废弃的生命探针表")
	log.Println("=" + string(make([]byte, 60)) + "=")

	// 废弃的表名列表
	deprecatedTables := []string{
		"life_focus_events",
		"life_screen_events",
		"life_energy_samples",
		"life_energy_daily_totals",
	}

	// 统计每个表的记录数和删除结果
	for _, tableName := range deprecatedTables {
		// 检查表是否存在
		if !db.Migrator().HasTable(tableName) {
			log.Printf("✓ 表 %s 不存在，跳过", tableName)
			continue
		}

		// 统计记录数
		var count int64
		if err := db.Table(tableName).Count(&count).Error; err != nil {
			log.Printf("⚠ 警告: 无法统计表 %s 的记录数: %v", tableName, err)
		} else {
			log.Printf("→ 表 %s 包含 %d 条记录", tableName, count)
		}

		// 删除表
		if err := db.Migrator().DropTable(tableName); err != nil {
			log.Printf("✗ 删除表 %s 失败: %v", tableName, err)
		} else {
			log.Printf("✓ 成功删除表 %s", tableName)
		}
	}

	log.Println("=" + string(make([]byte, 60)) + "=")
	log.Println("迁移完成")
	log.Println("=" + string(make([]byte, 60)) + "=")

	// 显示迁移后的数据库统计信息
	showDatabaseStats(db)
}

// showDatabaseStats 显示数据库统计信息
func showDatabaseStats(db *gorm.DB) {
	log.Println("\n数据库统计信息:")
	log.Println("-" + string(make([]byte, 60)) + "-")

	// 获取所有生命探针相关的表
	lifeTables := []string{
		"life_probes",
		"life_logger_events",
		"life_heart_rates",
		"life_step_samples",
		"life_step_daily_totals",
		"life_sleep_segments",
	}

	totalRecords := int64(0)
	for _, tableName := range lifeTables {
		if db.Migrator().HasTable(tableName) {
			var count int64
			if err := db.Table(tableName).Count(&count).Error; err != nil {
				log.Printf("  %s: 统计失败 (%v)", tableName, err)
			} else {
				log.Printf("  %-25s: %d 条记录", tableName, count)
				totalRecords += count
			}
		} else {
			log.Printf("  %-25s: 表不存在", tableName)
		}
	}

	log.Println("-" + string(make([]byte, 60)) + "-")
	log.Printf("生命探针数据总计: %d 条记录\n", totalRecords)

	// 显示数据库文件大小
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/monitor.db"
	}
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	if fileInfo, err := os.Stat(dbPath); err == nil {
		sizeMB := float64(fileInfo.Size()) / 1024 / 1024
		log.Printf("数据库文件大小: %.2f MB\n", sizeMB)
	}
}

// init 函数用于初始化
func init() {
	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetPrefix("[Migration] ")

	fmt.Println(`
╔═══════════════════════════════════════════════════════════════╗
║           生命探针数据库迁移 - 删除废弃表                     ║
║                                                               ║
║  此脚本将删除以下废弃的表:                                    ║
║  • life_focus_events       (专注状态记录)                    ║
║  • life_screen_events      (屏幕使用记录)                    ║
║  • life_energy_samples     (能量值详情)                      ║
║  • life_energy_daily_totals (每日能量汇总)                   ║
║                                                               ║
║  ⚠️  警告: 此操作不可逆，请确保已备份数据库                   ║
╚═══════════════════════════════════════════════════════════════╝
`)

	// 等待用户确认 (如果不是自动化运行)
	if os.Getenv("AUTO_MIGRATE") != "true" {
		fmt.Print("是否继续执行迁移？(yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" && response != "y" && response != "YES" && response != "Y" {
			log.Println("用户取消迁移")
			os.Exit(0)
		}
	}
}
