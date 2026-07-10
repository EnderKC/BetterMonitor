package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

type UpgradeTargetRequest struct {
	ServerIDs       []uint `json:"server_ids"`
	TargetVersion   string `json:"target_version"`
	Channel         string `json:"channel"`
	TargetAgentType string `json:"target_agent_type"`
}

type AgentUpgradeCommand struct {
	Type      string                     `json:"type"`
	RequestID string                     `json:"request_id"`
	Payload   AgentUpgradeCommandPayload `json:"payload"`
}

type AgentUpgradeCommandPayload struct {
	TargetVersion   string `json:"target_version"`
	TargetAgentType string `json:"target_agent_type"`
	AssetName       string `json:"asset_name"`
	AssetSize       int64  `json:"asset_size"`
	DownloadURL     string `json:"download_url"`
	SHA256          string `json:"sha256"`
}

type UpgradeCommandTransport interface {
	IsConnected(serverID uint) bool
	Send(serverID uint, command AgentUpgradeCommand) error
}

type UpgradeAssetResolver func(
	ctx context.Context,
	settings *models.SystemSettings,
	request ResolveUpgradeRequest,
) (AgentUpgradeAsset, error)

type UpgradeOrchestrator struct {
	DB           *gorm.DB
	Transport    UpgradeCommandTransport
	ResolveAsset UpgradeAssetResolver
	Now          func() time.Time
}

type UpgradeJobView struct {
	ID              string                    `json:"id"`
	ServerID        uint                      `json:"server_id"`
	Trigger         string                    `json:"trigger"`
	FromVersion     string                    `json:"from_version"`
	TargetVersion   string                    `json:"target_version"`
	FromAgentType   string                    `json:"from_agent_type"`
	TargetAgentType string                    `json:"target_agent_type"`
	Channel         string                    `json:"channel"`
	AssetName       string                    `json:"asset_name"`
	AssetSize       int64                     `json:"asset_size"`
	Status          models.AgentUpgradeStatus `json:"status"`
	LastMessage     string                    `json:"last_message"`
	ErrorCode       string                    `json:"error_code"`
	BytesDownloaded int64                     `json:"bytes_downloaded"`
	DeadlineAt      time.Time                 `json:"deadline_at"`
	DispatchedAt    *time.Time                `json:"dispatched_at"`
	CompletedAt     *time.Time                `json:"completed_at"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

type UpgradeNoop struct {
	ServerID  uint   `json:"server_id"`
	Version   string `json:"version"`
	AgentType string `json:"agent_type"`
	Reason    string `json:"reason"`
}

type UpgradeRejection struct {
	ServerID uint   `json:"server_id"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type UpgradeDispatchResult struct {
	Jobs     []UpgradeJobView   `json:"jobs"`
	Noop     []UpgradeNoop      `json:"noop"`
	Rejected []UpgradeRejection `json:"rejected"`
}

func (o UpgradeOrchestrator) Dispatch(ctx context.Context, request UpgradeTargetRequest) (UpgradeDispatchResult, error) {
	result := UpgradeDispatchResult{
		Jobs:     []UpgradeJobView{},
		Noop:     []UpgradeNoop{},
		Rejected: []UpgradeRejection{},
	}
	if o.DB == nil {
		return result, fmt.Errorf("dispatch agent upgrades: database is nil")
	}
	if o.Transport == nil {
		return result, fmt.Errorf("dispatch agent upgrades: transport is nil")
	}
	if len(request.ServerIDs) == 0 {
		return result, fmt.Errorf("server_ids are required")
	}

	var settings models.SystemSettings
	if err := o.DB.Order("id ASC").First(&settings).Error; err != nil {
		return result, fmt.Errorf("load agent release settings: %w", err)
	}
	channelInput := strings.TrimSpace(request.Channel)
	if channelInput == "" {
		channelInput = strings.TrimSpace(settings.AgentReleaseChannel)
	}
	if channelInput == "" {
		channelInput = "stable"
	}
	channel, err := NormalizeUpgradeChannel(channelInput)
	if err != nil {
		return result, err
	}

	targetVersion := strings.TrimSpace(request.TargetVersion)
	if targetVersion != "" {
		targetVersion, err = normalizeSemVersionText(targetVersion)
		if err != nil {
			return result, contractError("release_version_invalid", err)
		}
	}
	targetTypeInput := strings.ToLower(strings.TrimSpace(request.TargetAgentType))
	if targetTypeInput != "" && targetTypeInput != "full" && targetTypeInput != "monitor" {
		return result, contractError("release_agent_type_invalid", fmt.Errorf("unsupported agent type %q", request.TargetAgentType))
	}

	resolver := o.ResolveAsset
	if resolver == nil {
		resolver = ResolveAgentUpgradeAsset
	}
	now := time.Now().UTC()
	if o.Now != nil {
		now = o.Now().UTC()
	}

	seen := make(map[uint]struct{}, len(request.ServerIDs))
	for _, serverID := range request.ServerIDs {
		if serverID == 0 {
			result.Rejected = append(result.Rejected, UpgradeRejection{
				ServerID: serverID,
				Code:     "server_not_found",
				Message:  "server does not exist",
			})
			continue
		}
		if _, exists := seen[serverID]; exists {
			continue
		}
		seen[serverID] = struct{}{}

		var server models.Server
		if err := o.DB.First(&server, serverID).Error; err != nil {
			code := "server_load_failed"
			message := "failed to load server"
			if errors.Is(err, gorm.ErrRecordNotFound) {
				code = "server_not_found"
				message = "server does not exist"
			}
			result.Rejected = append(result.Rejected, UpgradeRejection{ServerID: serverID, Code: code, Message: message})
			continue
		}

		if hasActiveUpgradeJob(o.DB, server.ID) {
			result.Rejected = append(result.Rejected, UpgradeRejection{
				ServerID: server.ID,
				Code:     "upgrade_already_active",
				Message:  "server already has an active agent upgrade",
			})
			continue
		}

		currentType := strings.ToLower(strings.TrimSpace(server.AgentType))
		if currentType == "" {
			currentType = "full"
		}
		targetType := targetTypeInput
		if targetType == "" {
			targetType = currentType
		}
		currentVersion := strings.TrimSpace(server.AgentVersion)
		if targetVersion != "" && targetVersion == currentVersion && targetType == currentType {
			result.Noop = append(result.Noop, UpgradeNoop{
				ServerID:  server.ID,
				Version:   currentVersion,
				AgentType: currentType,
				Reason:    "already_at_target",
			})
			continue
		}

		if !IsServerOnline(server, now) || !o.Transport.IsConnected(server.ID) {
			result.Rejected = append(result.Rejected, UpgradeRejection{
				ServerID: server.ID,
				Code:     "agent_offline",
				Message:  "agent is offline or disconnected",
			})
			continue
		}

		asset, err := resolver(ctx, &settings, ResolveUpgradeRequest{
			TargetVersion: targetVersion,
			Channel:       channel,
			OS:            server.OS,
			Arch:          server.Arch,
			AgentType:     targetType,
		})
		if err != nil {
			code := UpgradeContractErrorCode(err)
			if code == "" {
				code = "release_preflight_failed"
			}
			result.Rejected = append(result.Rejected, UpgradeRejection{
				ServerID: server.ID,
				Code:     code,
				Message:  "agent release preflight failed",
			})
			continue
		}
		if asset.Version == currentVersion && targetType == currentType {
			result.Noop = append(result.Noop, UpgradeNoop{
				ServerID:  server.ID,
				Version:   currentVersion,
				AgentType: currentType,
				Reason:    "already_at_target",
			})
			continue
		}

		trigger := "manual"
		if targetType != currentType {
			trigger = "type_switch"
		}
		job, err := CreateAgentUpgradeJob(o.DB, CreateUpgradeJobInput{
			ServerID:        server.ID,
			Trigger:         trigger,
			TargetVersion:   asset.Version,
			TargetAgentType: targetType,
			Channel:         asset.Channel,
			AssetName:       asset.Name,
			AssetSize:       asset.Size,
			DownloadURL:     asset.DownloadURL,
			SHA256:          asset.SHA256,
			Now:             now,
		})
		if err != nil {
			code := "upgrade_job_create_failed"
			message := "failed to create agent upgrade job"
			if errors.Is(err, ErrActiveUpgradeExists) {
				code = "upgrade_already_active"
				message = "server already has an active agent upgrade"
			}
			result.Rejected = append(result.Rejected, UpgradeRejection{ServerID: server.ID, Code: code, Message: message})
			continue
		}

		command := AgentUpgradeCommand{
			Type:      "agent_upgrade",
			RequestID: job.ID,
			Payload: AgentUpgradeCommandPayload{
				TargetVersion:   job.TargetVersion,
				TargetAgentType: job.TargetAgentType,
				AssetName:       job.AssetName,
				AssetSize:       job.AssetSize,
				DownloadURL:     job.DownloadURL,
				SHA256:          job.SHA256,
			},
		}
		if err := o.Transport.Send(server.ID, command); err != nil {
			_ = TransitionUpgradeJob(o.DB, job.ID, models.UpgradeFailed, UpgradeUpdate{
				Message:   "failed to dispatch agent upgrade",
				ErrorCode: "dispatch_failed",
				At:        now,
			})
			result.Jobs = append(result.Jobs, loadUpgradeJobView(o.DB, job.ID))
			continue
		}
		if err := TransitionUpgradeJob(o.DB, job.ID, models.UpgradeDispatched, UpgradeUpdate{
			Message: "agent upgrade dispatched",
			At:      now,
		}); err != nil {
			_ = TransitionUpgradeJob(o.DB, job.ID, models.UpgradeFailed, UpgradeUpdate{
				Message:   "failed to persist dispatch state",
				ErrorCode: "dispatch_state_failed",
				At:        now,
			})
		}
		result.Jobs = append(result.Jobs, loadUpgradeJobView(o.DB, job.ID))
	}
	return result, nil
}

func AgentUpgradeJobView(job models.AgentUpgradeJob) UpgradeJobView {
	return UpgradeJobView{
		ID:              job.ID,
		ServerID:        job.ServerID,
		Trigger:         job.Trigger,
		FromVersion:     job.FromVersion,
		TargetVersion:   job.TargetVersion,
		FromAgentType:   job.FromAgentType,
		TargetAgentType: job.TargetAgentType,
		Channel:         job.Channel,
		AssetName:       job.AssetName,
		AssetSize:       job.AssetSize,
		Status:          job.Status,
		LastMessage:     job.LastMessage,
		ErrorCode:       job.ErrorCode,
		BytesDownloaded: job.BytesDownloaded,
		DeadlineAt:      job.DeadlineAt,
		DispatchedAt:    job.DispatchedAt,
		CompletedAt:     job.CompletedAt,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
	}
}

func hasActiveUpgradeJob(db *gorm.DB, serverID uint) bool {
	var count int64
	return db.Model(&models.AgentUpgradeJob{}).
		Where("server_id = ? AND active_key IS NOT NULL", serverID).
		Count(&count).Error == nil && count > 0
}

func loadUpgradeJobView(db *gorm.DB, jobID string) UpgradeJobView {
	var job models.AgentUpgradeJob
	if err := db.First(&job, "id = ?", jobID).Error; err != nil {
		return UpgradeJobView{ID: jobID}
	}
	return AgentUpgradeJobView(job)
}
