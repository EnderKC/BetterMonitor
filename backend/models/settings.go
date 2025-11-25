package models

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// SystemSettings 存储全局系统设置
type SystemSettings struct {
	gorm.Model
	// 心跳和监控设置 (Agent)
	HeartbeatInterval string `json:"heartbeat_interval" gorm:"default:'10s'"` // 心跳上报间隔
	MonitorInterval   string `json:"monitor_interval" gorm:"default:'30s'"`   // 监控数据上报间隔

	// 前端设置
	UIRefreshInterval string `json:"ui_refresh_interval" gorm:"default:'10s'"` // 探针页面数据刷新间隔
	ChartHistoryHours int    `json:"chart_history_hours" gorm:"default:24"`    // 图表显示的历史数据小时数

	// 监控数据保留策略
	DataRetentionDays int `json:"data_retention_days" gorm:"default:7"` // 监控数据保留天数

	// Agent升级设置
	AgentReleaseRepo    string `json:"agent_release_repo" gorm:"default:'EnderKC/BetterMonitor'"` // GitHub仓库
	AgentReleaseChannel string `json:"agent_release_channel" gorm:"default:'stable'"`             // stable/nightly等
	AgentReleaseMirror  string `json:"agent_release_mirror" gorm:"default:''"`                    // 下载镜像（可选）
}

// 默认设置值
var defaultSettings = SystemSettings{
	HeartbeatInterval:   "10s",
	MonitorInterval:     "30s",
	UIRefreshInterval:   "10s",
	ChartHistoryHours:   24,
	DataRetentionDays:   7,
	AgentReleaseRepo:    "EnderKC/BetterMonitor",
	AgentReleaseChannel: "stable",
	AgentReleaseMirror:  "",
}

// GetSettings 获取系统设置
func GetSettings() (*SystemSettings, error) {
	var settings SystemSettings

	// 检索第一条记录
	err := DB.First(&settings).Error

	// 如果没有记录，创建默认设置
	if errors.Is(err, gorm.ErrRecordNotFound) {
		settings = defaultSettings
		err = DB.Create(&settings).Error
	}

	return &settings, err
}

// ParseDuration 将字符串解析为时间Duration
func ParseDuration(duration string) (time.Duration, error) {
	return time.ParseDuration(duration)
}

// SaveSettings 保存系统设置
func SaveSettings(settings *SystemSettings) error {
	// 验证Duration格式
	_, err := time.ParseDuration(settings.HeartbeatInterval)
	if err != nil {
		return errors.New("无效的心跳间隔格式: " + err.Error())
	}

	_, err = time.ParseDuration(settings.MonitorInterval)
	if err != nil {
		return errors.New("无效的监控间隔格式: " + err.Error())
	}

	_, err = time.ParseDuration(settings.UIRefreshInterval)
	if err != nil {
		return errors.New("无效的UI刷新间隔格式: " + err.Error())
	}

	var existingSettings SystemSettings
	result := DB.First(&existingSettings)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 如果没有设置，创建新的
			return DB.Create(settings).Error
		}
		return result.Error
	}

	// 更新现有设置
	return DB.Model(&existingSettings).Updates(settings).Error
}

// GetSettingsAsJSON 获取设置为JSON格式
func GetSettingsAsJSON() (string, error) {
	settings, err := GetSettings()
	if err != nil {
		return "", err
	}

	jsonData, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}
