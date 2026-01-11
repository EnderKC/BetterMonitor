package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// LifeProbeRetentionConfig 生命探针数据保留配置
type LifeProbeRetentionConfig struct {
	HeartRateDays   int `json:"heart_rate_days"`   // 心率数据保留天数，0表示永久保留
	StepDetailDays  int `json:"step_detail_days"`  // 步数详情保留天数，0表示永久保留
	SleepDetailDays int `json:"sleep_detail_days"` // 睡眠详情保留天数，0表示永久保留
}

// SystemSettings 存储全局系统设置
type SystemSettings struct {
	gorm.Model
	// 监控设置 (Agent)
	MonitorInterval string `json:"monitor_interval" gorm:"default:'30s'"` // 监控数据上报间隔

	// 前端设置
	UIRefreshInterval string `json:"ui_refresh_interval" gorm:"default:'10s'"` // 探针页面数据刷新间隔
	ChartHistoryHours int    `json:"chart_history_hours" gorm:"default:24"`    // 图表显示的历史数据小时数

	// 监控数据保留策略
	DataRetentionDays int `json:"data_retention_days" gorm:"default:7"` // 服务器监控数据保留天数

	// 生命探针数据保留策略（JSON格式，支持更细粒度控制）
	LifeProbeRetentionJSON string `json:"life_probe_retention_json" gorm:"type:text"` // JSON格式存储

	// 生命探针公开访问设置
	AllowPublicLifeProbeAccess bool `json:"allow_public_life_probe_access" gorm:"default:true"` // 是否允许公开访问生命探针详情

	// Agent升级设置
	AgentReleaseRepo    string `json:"agent_release_repo" gorm:"default:'EnderKC/BetterMonitor'"` // GitHub仓库
	AgentReleaseChannel string `json:"agent_release_channel" gorm:"default:'stable'"`             // stable/nightly等
	AgentReleaseMirror  string `json:"agent_release_mirror" gorm:"default:''"`                    // 下载镜像（可选）
}

// GetLifeProbeRetention 获取生命探针保留配置
func (s *SystemSettings) GetLifeProbeRetention() (*LifeProbeRetentionConfig, error) {
	if s.LifeProbeRetentionJSON == "" {
		// 返回默认值
		return &LifeProbeRetentionConfig{
			HeartRateDays:   90,  // 默认90天
			StepDetailDays:  180, // 默认180天
			SleepDetailDays: 365, // 默认365天
		}, nil
	}

	var config LifeProbeRetentionConfig
	if err := json.Unmarshal([]byte(s.LifeProbeRetentionJSON), &config); err != nil {
		return nil, fmt.Errorf("解析生命探针保留配置失败: %w", err)
	}
	return &config, nil
}

// SetLifeProbeRetention 设置生命探针保留配置
func (s *SystemSettings) SetLifeProbeRetention(config *LifeProbeRetentionConfig) error {
	// 验证配置
	if config.HeartRateDays < 0 || config.StepDetailDays < 0 || config.SleepDetailDays < 0 {
		return errors.New("保留天数不能为负数")
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化生命探针保留配置失败: %w", err)
	}
	s.LifeProbeRetentionJSON = string(data)
	return nil
}

// 默认设置值
var defaultSettings = SystemSettings{
	MonitorInterval:   "30s",
	UIRefreshInterval: "10s",
	ChartHistoryHours: 24,
	DataRetentionDays: 7,
	LifeProbeRetentionJSON: `{
		"heart_rate_days": 90,
		"step_detail_days": 180,
		"sleep_detail_days": 365
	}`,
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
	_, err := time.ParseDuration(settings.MonitorInterval)
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
	// 注意：GORM 的 Updates(struct) 默认会忽略零值字段（false/0/""），
	// 会导致布尔开关无法从 true 更新为 false。
	// 通过 Select("*") 强制更新所有字段，同时 Omit 掉主键/时间戳等不可更新字段。
	return DB.Model(&existingSettings).
		Select("*").
		Omit("id", "created_at", "updated_at", "deleted_at").
		Updates(settings).Error
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
