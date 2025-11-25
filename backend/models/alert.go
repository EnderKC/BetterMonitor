package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// AlertSetting 预警设置模型
type AlertSetting struct {
	gorm.Model
	Type        string  `json:"type" gorm:"type:varchar(20);not null"`  // cpu, memory, network, status
	Threshold   float64 `json:"threshold" gorm:"not null"`              // 阈值百分比(0-100)或具体数值，对status类型：1表示上线报警，2表示离线报警，3表示上线和离线都报警
	Duration    int     `json:"duration" gorm:"not null"`               // 持续时间(秒)
	Enabled     bool    `json:"enabled" gorm:"default:true"`            // 是否启用
	ServerID    uint    `json:"server_id" gorm:"default:0"`             // 0表示全局设置，非0表示特定服务器
}

// NotificationChannel 通知渠道模型
type NotificationChannel struct {
	gorm.Model
	Type        string `json:"type" gorm:"type:varchar(20);not null"`  // email, serverchan
	Name        string `json:"name" gorm:"type:varchar(50);not null"`  // 渠道名称
	Config      string `json:"config" gorm:"type:text"`                // JSON格式配置，包含密钥等
	Enabled     bool   `json:"enabled" gorm:"default:true"`            // 是否启用
}

// AlertRecord 预警记录模型
type AlertRecord struct {
	gorm.Model
	ServerID     uint      `json:"server_id" gorm:"index"`
	ServerName   string    `json:"server_name"`
	AlertType    string    `json:"alert_type"`          // cpu, memory, network
	Value        float64   `json:"value"`               // 触发时的值
	Threshold    float64   `json:"threshold"`           // 阈值
	Resolved     bool      `json:"resolved"`            // 是否已解决
	ResolvedAt   time.Time `json:"resolved_at"`         // 解决时间
	NotifiedAt   time.Time `json:"notified_at"`         // 通知时间
	ChannelIDs   string    `json:"channel_ids"`         // 通知渠道ID列表，逗号分隔
}

// GetGlobalAlertSettings 获取全局预警设置
func GetGlobalAlertSettings() ([]AlertSetting, error) {
	var settings []AlertSetting
	result := DB.Where("server_id = 0").Find(&settings)
	return settings, result.Error
}

// GetServerAlertSettings 获取服务器特定的预警设置
func GetServerAlertSettings(serverID uint) ([]AlertSetting, error) {
	var settings []AlertSetting
	result := DB.Where("server_id = ?", serverID).Find(&settings)
	return settings, result.Error
}

// GetAlertSettingByID 通过ID获取预警设置
func GetAlertSettingByID(id uint, setting *AlertSetting) error {
	return DB.First(setting, id).Error
}

// CreateAlertSetting 创建预警设置
func CreateAlertSetting(setting *AlertSetting) error {
	return DB.Create(setting).Error
}

// UpdateAlertSetting 更新预警设置
func UpdateAlertSetting(setting *AlertSetting) error {
	return DB.Save(setting).Error
}

// DeleteAlertSetting 删除预警设置
func DeleteAlertSetting(id uint) error {
	return DB.Delete(&AlertSetting{}, id).Error
}

// GetAllNotificationChannels 获取所有通知渠道
func GetAllNotificationChannels() ([]NotificationChannel, error) {
	var channels []NotificationChannel
	result := DB.Find(&channels)
	return channels, result.Error
}

// GetEnabledNotificationChannels 获取所有启用的通知渠道
func GetEnabledNotificationChannels() ([]NotificationChannel, error) {
	var channels []NotificationChannel
	result := DB.Where("enabled = ?", true).Find(&channels)
	return channels, result.Error
}

// GetNotificationChannelByID 通过ID获取通知渠道
func GetNotificationChannelByID(id uint, channel *NotificationChannel) error {
	return DB.First(channel, id).Error
}

// GetChannelsByIDs 通过ID列表获取通知渠道
func GetChannelsByIDs(ids []string) ([]NotificationChannel, error) {
	if len(ids) == 0 {
		return []NotificationChannel{}, nil
	}

	var channels []NotificationChannel
	result := DB.Where("id IN (?)", ids).Find(&channels)
	return channels, result.Error
}

// CreateNotificationChannel 创建通知渠道
func CreateNotificationChannel(channel *NotificationChannel) error {
	return DB.Create(channel).Error
}

// UpdateNotificationChannel 更新通知渠道
func UpdateNotificationChannel(channel *NotificationChannel) error {
	return DB.Save(channel).Error
}

// DeleteNotificationChannel 删除通知渠道
func DeleteNotificationChannel(id uint) error {
	return DB.Delete(&NotificationChannel{}, id).Error
}

// GetAlertRecords 获取预警记录
func GetAlertRecords(serverID uint, alertType string, onlyUnresolved bool, page, limit int) ([]AlertRecord, int64, error) {
	var records []AlertRecord
	var total int64
	
	query := DB.Model(&AlertRecord{})
	
	if serverID > 0 {
		query = query.Where("server_id = ?", serverID)
	}
	
	if alertType != "" {
		query = query.Where("alert_type = ?", alertType)
	}
	
	if onlyUnresolved {
		query = query.Where("resolved = ?", false)
	}
	
	// 计算总数
	query.Count(&total)
	
	// 分页查询
	offset := (page - 1) * limit
	result := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&records)
	
	return records, total, result.Error
}

// GetLatestUnresolvedAlert 获取最新的未解决预警
func GetLatestUnresolvedAlert(serverID uint, alertType string) (*AlertRecord, error) {
	var record AlertRecord
	result := DB.Where("server_id = ? AND alert_type = ? AND resolved = ?", 
		serverID, alertType, false).Order("created_at DESC").First(&record)
	return &record, result.Error
}

// GetAlertRecordByID 通过ID获取预警记录
func GetAlertRecordByID(id uint, record *AlertRecord) error {
	return DB.First(record, id).Error
}

// CreateAlertRecord 创建预警记录
func CreateAlertRecord(record *AlertRecord) error {
	return DB.Create(record).Error
}

// UpdateAlertRecord 更新预警记录
func UpdateAlertRecord(record *AlertRecord) error {
	return DB.Save(record).Error
}

// GetChannelConfig 解析通知渠道配置
func (c *NotificationChannel) GetChannelConfig() (map[string]string, error) {
	var config map[string]string
	if err := json.Unmarshal([]byte(c.Config), &config); err != nil {
		return nil, err
	}
	return config, nil
}

// GetFormattedChannelIDs 返回格式化的通知渠道ID列表
func (r *AlertRecord) GetFormattedChannelIDs() []uint {
	if r.ChannelIDs == "" {
		return []uint{}
	}
	
	idStrs := strings.Split(r.ChannelIDs, ",")
	ids := make([]uint, 0, len(idStrs))
	
	for _, idStr := range idStrs {
		var id uint
		fmt.Sscanf(idStr, "%d", &id)
		if id > 0 {
			ids = append(ids, id)
		}
	}
	
	return ids
} 