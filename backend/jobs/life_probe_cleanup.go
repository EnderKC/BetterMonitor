package jobs

import (
	"log"
	"time"

	"github.com/user/server-ops-backend/models"
)

// CleanupLifeProbeData 清理过期的生命探针数据
// 根据系统设置中的保留策略删除过期数据
func CleanupLifeProbeData() {
	log.Println("[定时任务] 开始执行生命探针数据清理...")

	settings, err := models.GetSettings()
	if err != nil {
		log.Printf("[定时任务] 获取系统设置失败: %v", err)
		return
	}

	config, err := settings.GetLifeProbeRetention()
	if err != nil {
		log.Printf("[定时任务] 获取保留配置失败: %v", err)
		return
	}

	now := time.Now()
	totalDeleted := 0

	// 清理心率数据
	if config.HeartRateDays > 0 {
		cutoff := now.AddDate(0, 0, -config.HeartRateDays)
		result := models.DB.Where("measure_time < ?", cutoff).Delete(&models.LifeHeartRate{})
		if result.Error != nil {
			log.Printf("[定时任务] 清理心率数据失败: %v", result.Error)
		} else {
			log.Printf("[定时任务] 清理心率数据: 删除 %d 条记录 (保留 %d 天)", result.RowsAffected, config.HeartRateDays)
			totalDeleted += int(result.RowsAffected)
		}
	} else {
		log.Printf("[定时任务] 心率数据永久保留 (配置: 0 天)")
	}

	// 清理步数详情数据
	if config.StepDetailDays > 0 {
		cutoff := now.AddDate(0, 0, -config.StepDetailDays)
		result := models.DB.Where("end_time < ?", cutoff).Delete(&models.LifeStepSample{})
		if result.Error != nil {
			log.Printf("[定时任务] 清理步数详情失败: %v", result.Error)
		} else {
			log.Printf("[定时任务] 清理步数详情: 删除 %d 条记录 (保留 %d 天)", result.RowsAffected, config.StepDetailDays)
			totalDeleted += int(result.RowsAffected)
		}
	} else {
		log.Printf("[定时任务] 步数详情永久保留 (配置: 0 天)")
	}

	// 清理睡眠详情数据
	if config.SleepDetailDays > 0 {
		cutoff := now.AddDate(0, 0, -config.SleepDetailDays)
		result := models.DB.Where("end_time < ?", cutoff).Delete(&models.LifeSleepSegment{})
		if result.Error != nil {
			log.Printf("[定时任务] 清理睡眠详情失败: %v", result.Error)
		} else {
			log.Printf("[定时任务] 清理睡眠详情: 删除 %d 条记录 (保留 %d 天)", result.RowsAffected, config.SleepDetailDays)
			totalDeleted += int(result.RowsAffected)
		}
	} else {
		log.Printf("[定时任务] 睡眠详情永久保留 (配置: 0 天)")
	}

	log.Printf("[定时任务] 生命探针数据清理完成，共删除 %d 条记录", totalDeleted)
}

// CleanupLifeLoggerEvents 清理过期的生命探针事件日志
// 保留最近30天的事件日志（用于审计）
func CleanupLifeLoggerEvents() {
	log.Println("[定时任务] 开始清理生命探针事件日志...")

	// 保留最近30天的事件日志
	cutoff := time.Now().AddDate(0, 0, -30)
	result := models.DB.Where("timestamp < ?", cutoff).Delete(&models.LifeLoggerEvent{})

	if result.Error != nil {
		log.Printf("[定时任务] 清理事件日志失败: %v", result.Error)
		return
	}

	log.Printf("[定时任务] 清理事件日志完成，删除 %d 条记录 (保留 30 天)", result.RowsAffected)
}

// CleanupStaleLifeProbes 清理长时间未同步的生命探针
// 可选功能：标记或通知长期未活跃的探针
func CleanupStaleLifeProbes() {
	log.Println("[定时任务] 检查长时间未同步的生命探针...")

	// 查询超过30天未同步的探针
	var staleProbes []models.LifeProbe
	cutoff := time.Now().AddDate(0, 0, -30)

	err := models.DB.Where("last_sync_at IS NOT NULL AND last_sync_at < ?", cutoff).Find(&staleProbes).Error
	if err != nil {
		log.Printf("[定时任务] 查询长时间未同步探针失败: %v", err)
		return
	}

	if len(staleProbes) > 0 {
		log.Printf("[定时任务] 发现 %d 个超过30天未同步的探针:", len(staleProbes))
		for _, probe := range staleProbes {
			log.Printf("  - [%d] %s (设备ID: %s, 最后同步: %s)",
				probe.ID, probe.Name, probe.DeviceID,
				probe.LastSyncAt.Format("2006-01-02 15:04:05"))
		}
	} else {
		log.Println("[定时任务] 所有探针同步状态正常")
	}
}
