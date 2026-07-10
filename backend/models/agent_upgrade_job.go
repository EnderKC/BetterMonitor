package models

import "time"

type AgentUpgradeStatus string

const (
	UpgradeQueued      AgentUpgradeStatus = "queued"
	UpgradeDispatched  AgentUpgradeStatus = "dispatched"
	UpgradeReceived    AgentUpgradeStatus = "received"
	UpgradeDownloading AgentUpgradeStatus = "downloading"
	UpgradeVerifying   AgentUpgradeStatus = "verifying"
	UpgradeApplying    AgentUpgradeStatus = "applying"
	UpgradeRestarting  AgentUpgradeStatus = "restarting"
	UpgradeSucceeded   AgentUpgradeStatus = "succeeded"
	UpgradeFailed      AgentUpgradeStatus = "failed"
	UpgradeTimedOut    AgentUpgradeStatus = "timed_out"
)

type AgentUpgradeJob struct {
	ID              string             `json:"id" gorm:"primaryKey;size:64"`
	ServerID        uint               `json:"server_id" gorm:"not null;index;uniqueIndex:idx_agent_upgrade_active,priority:1"`
	Trigger         string             `json:"trigger" gorm:"size:32;not null"`
	FromVersion     string             `json:"from_version" gorm:"size:64"`
	TargetVersion   string             `json:"target_version" gorm:"size:64;not null"`
	FromAgentType   string             `json:"from_agent_type" gorm:"size:20"`
	TargetAgentType string             `json:"target_agent_type" gorm:"size:20;not null"`
	Channel         string             `json:"channel" gorm:"size:20;not null"`
	AssetName       string             `json:"asset_name" gorm:"size:255;not null"`
	AssetSize       int64              `json:"asset_size" gorm:"not null"`
	DownloadURL     string             `json:"-" gorm:"type:text;not null"`
	SHA256          string             `json:"-" gorm:"size:64;not null"`
	Status          AgentUpgradeStatus `json:"status" gorm:"size:20;not null;index"`
	LastMessage     string             `json:"last_message" gorm:"type:text"`
	ErrorCode       string             `json:"error_code" gorm:"size:64"`
	BytesDownloaded int64              `json:"bytes_downloaded" gorm:"not null;default:0"`
	ActiveKey       *string            `json:"-" gorm:"size:16;uniqueIndex:idx_agent_upgrade_active,priority:2"`
	DeadlineAt      time.Time          `json:"deadline_at" gorm:"not null;index"`
	DispatchedAt    *time.Time         `json:"dispatched_at"`
	CompletedAt     *time.Time         `json:"completed_at"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}
