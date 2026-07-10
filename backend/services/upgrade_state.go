package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

const DefaultAgentUpgradeTimeout = 20 * time.Minute

var (
	ErrActiveUpgradeExists       = errors.New("active agent upgrade already exists")
	ErrUpgradeTransitionInvalid  = errors.New("invalid agent upgrade transition")
	ErrUpgradeProgressRegressed  = errors.New("agent upgrade progress regressed")
	ErrUpgradeTransitionConflict = errors.New("agent upgrade transition conflict")

	upgradeCreateMu sync.Mutex
)

type CreateUpgradeJobInput struct {
	ServerID        uint
	Trigger         string
	TargetVersion   string
	TargetAgentType string
	Channel         string
	AssetName       string
	AssetSize       int64
	DownloadURL     string
	SHA256          string
	Now             time.Time
	DeadlineAt      time.Time
}

type UpgradeUpdate struct {
	Message         string
	ErrorCode       string
	BytesDownloaded int64
	At              time.Time
}

type AgentUpgradeStatusInput struct {
	ServerID        uint
	RequestID       string
	Status          models.AgentUpgradeStatus
	BytesDownloaded int64
	ErrorCode       string
	At              time.Time
}

type AgentUpgradeBootInput struct {
	ServerID  uint
	RequestID string
	Version   string
	AgentType string
	Outcome   string
	ErrorCode string
	At        time.Time
}

type AgentUpgradeBootResult struct {
	Found     bool
	Confirmed bool
}

var upgradeForwardTransitions = map[models.AgentUpgradeStatus]models.AgentUpgradeStatus{
	models.UpgradeQueued:      models.UpgradeDispatched,
	models.UpgradeDispatched:  models.UpgradeReceived,
	models.UpgradeReceived:    models.UpgradeDownloading,
	models.UpgradeDownloading: models.UpgradeVerifying,
	models.UpgradeVerifying:   models.UpgradeApplying,
	models.UpgradeApplying:    models.UpgradeRestarting,
	models.UpgradeRestarting:  models.UpgradeSucceeded,
}

var nonTerminalUpgradeStatuses = []models.AgentUpgradeStatus{
	models.UpgradeQueued,
	models.UpgradeDispatched,
	models.UpgradeReceived,
	models.UpgradeDownloading,
	models.UpgradeVerifying,
	models.UpgradeApplying,
	models.UpgradeRestarting,
}

func CreateAgentUpgradeJob(db *gorm.DB, input CreateUpgradeJobInput) (*models.AgentUpgradeJob, error) {
	if db == nil {
		return nil, fmt.Errorf("create agent upgrade job: database is nil")
	}
	upgradeCreateMu.Lock()
	defer upgradeCreateMu.Unlock()

	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	deadline := input.DeadlineAt
	if deadline.IsZero() {
		deadline = now.Add(DefaultAgentUpgradeTimeout)
	}

	var created models.AgentUpgradeJob
	err := db.Transaction(func(tx *gorm.DB) error {
		var server models.Server
		if err := tx.First(&server, input.ServerID).Error; err != nil {
			return fmt.Errorf("load upgrade server: %w", err)
		}

		fromType := strings.ToLower(strings.TrimSpace(server.AgentType))
		if fromType == "" {
			fromType = "full"
		}
		targetType := strings.ToLower(strings.TrimSpace(input.TargetAgentType))
		if targetType == "" {
			targetType = fromType
		}
		trigger := strings.TrimSpace(input.Trigger)
		if trigger == "" {
			trigger = "manual"
		}

		jobID, err := generateUpgradeJobID()
		if err != nil {
			return err
		}
		activeKey := "active"
		created = models.AgentUpgradeJob{
			ID:              jobID,
			ServerID:        server.ID,
			Trigger:         trigger,
			FromVersion:     strings.TrimSpace(server.AgentVersion),
			TargetVersion:   strings.TrimSpace(input.TargetVersion),
			FromAgentType:   fromType,
			TargetAgentType: targetType,
			Channel:         strings.ToLower(strings.TrimSpace(input.Channel)),
			AssetName:       strings.TrimSpace(input.AssetName),
			AssetSize:       input.AssetSize,
			DownloadURL:     strings.TrimSpace(input.DownloadURL),
			SHA256:          strings.ToLower(strings.TrimSpace(input.SHA256)),
			Status:          models.UpgradeQueued,
			ActiveKey:       &activeKey,
			DeadlineAt:      deadline.UTC(),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUpgradeUniqueConstraintError(err) {
				return ErrActiveUpgradeExists
			}
			return fmt.Errorf("create agent upgrade job: %w", err)
		}
		if err := tx.Model(&models.Server{}).
			Where("id = ?", server.ID).
			Update("desired_agent_type", targetType).Error; err != nil {
			return fmt.Errorf("set desired agent type: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func TransitionUpgradeJob(
	db *gorm.DB,
	jobID string,
	next models.AgentUpgradeStatus,
	update UpgradeUpdate,
) error {
	if db == nil {
		return fmt.Errorf("transition agent upgrade job: database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var job models.AgentUpgradeJob
		if err := tx.First(&job, "id = ?", jobID).Error; err != nil {
			return fmt.Errorf("load agent upgrade job: %w", err)
		}
		if isTerminalUpgradeStatus(job.Status) {
			return ErrUpgradeTransitionInvalid
		}

		sameState := next == job.Status
		if !sameState && next != models.UpgradeFailed && next != models.UpgradeTimedOut {
			if upgradeForwardTransitions[job.Status] != next {
				return ErrUpgradeTransitionInvalid
			}
		}
		if update.BytesDownloaded > 0 && update.BytesDownloaded < job.BytesDownloaded {
			return ErrUpgradeProgressRegressed
		}

		at := update.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		updates := map[string]interface{}{
			"status":     next,
			"updated_at": at,
		}
		if strings.TrimSpace(update.Message) != "" {
			updates["last_message"] = strings.TrimSpace(update.Message)
		}
		if strings.TrimSpace(update.ErrorCode) != "" {
			updates["error_code"] = strings.TrimSpace(update.ErrorCode)
		}
		if update.BytesDownloaded > 0 {
			updates["bytes_downloaded"] = update.BytesDownloaded
		}
		if next == models.UpgradeDispatched && job.DispatchedAt == nil {
			updates["dispatched_at"] = at
		}
		if isTerminalUpgradeStatus(next) {
			updates["active_key"] = nil
			updates["completed_at"] = at
		}

		query := tx.Model(&models.AgentUpgradeJob{}).
			Where("id = ? AND status = ?", job.ID, job.Status)
		if update.BytesDownloaded > 0 {
			query = query.Where("bytes_downloaded <= ?", update.BytesDownloaded)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update agent upgrade job: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrUpgradeTransitionConflict
		}

		if next == models.UpgradeFailed || next == models.UpgradeTimedOut {
			if err := tx.Model(&models.Server{}).
				Where("id = ?", job.ServerID).
				Update("desired_agent_type", gorm.Expr("agent_type")).Error; err != nil {
				return fmt.Errorf("restore desired agent type: %w", err)
			}
		} else if next == models.UpgradeSucceeded {
			if err := tx.Model(&models.Server{}).
				Where("id = ?", job.ServerID).
				Update("desired_agent_type", job.TargetAgentType).Error; err != nil {
				return fmt.Errorf("confirm desired agent type: %w", err)
			}
		}
		return nil
	})
}

func RecordAgentUpgradeStatus(db *gorm.DB, input AgentUpgradeStatusInput) error {
	if db == nil {
		return fmt.Errorf("record agent upgrade status: database is nil")
	}
	requestID := strings.TrimSpace(input.RequestID)
	if input.ServerID == 0 || requestID == "" {
		return fmt.Errorf("record agent upgrade status: server and request id are required")
	}
	if input.BytesDownloaded < 0 {
		return fmt.Errorf("record agent upgrade status: bytes downloaded cannot be negative")
	}
	if !isAgentReportedUpgradeStatus(input.Status) {
		return fmt.Errorf("record agent upgrade status: unsupported status %q", input.Status)
	}

	var job models.AgentUpgradeJob
	if err := db.Where("id = ? AND server_id = ?", requestID, input.ServerID).First(&job).Error; err != nil {
		return fmt.Errorf("record agent upgrade status: %w", err)
	}

	errorCode := safeAgentUpgradeErrorCode(input.ErrorCode)
	if input.Status == models.UpgradeFailed && errorCode == "" {
		errorCode = "agent_upgrade_failed"
	}
	message := "agent reported " + string(input.Status)
	if input.Status == models.UpgradeFailed {
		message = "agent reported failure: " + errorCode
	}
	return TransitionUpgradeJob(db, job.ID, input.Status, UpgradeUpdate{
		Message:         message,
		ErrorCode:       errorCode,
		BytesDownloaded: input.BytesDownloaded,
		At:              input.At,
	})
}

func ReconcileAgentUpgradeBoot(db *gorm.DB, input AgentUpgradeBootInput) (AgentUpgradeBootResult, error) {
	if db == nil {
		return AgentUpgradeBootResult{}, fmt.Errorf("reconcile agent upgrade boot: database is nil")
	}
	requestID := strings.TrimSpace(input.RequestID)
	if input.ServerID == 0 || requestID == "" {
		return AgentUpgradeBootResult{}, fmt.Errorf("reconcile agent upgrade boot: server and request id are required")
	}

	var job models.AgentUpgradeJob
	err := db.Where("id = ? AND server_id = ?", requestID, input.ServerID).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AgentUpgradeBootResult{}, nil
	}
	if err != nil {
		return AgentUpgradeBootResult{}, fmt.Errorf("reconcile agent upgrade boot: %w", err)
	}
	result := AgentUpgradeBootResult{Found: true, Confirmed: true}
	if isTerminalUpgradeStatus(job.Status) {
		return result, nil
	}

	at := input.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	outcome := strings.ToLower(strings.TrimSpace(input.Outcome))
	var next models.AgentUpgradeStatus
	var update UpgradeUpdate
	update.At = at
	switch outcome {
	case "applied":
		versionMatches := strings.TrimSpace(input.Version) == job.TargetVersion
		typeMatches := strings.EqualFold(strings.TrimSpace(input.AgentType), job.TargetAgentType)
		if !versionMatches || !typeMatches {
			next = models.UpgradeFailed
			update.ErrorCode = "upgrade_identity_mismatch"
			update.Message = "agent restarted with unexpected upgrade identity"
		} else if job.Status != models.UpgradeRestarting {
			next = models.UpgradeFailed
			update.ErrorCode = "upgrade_state_mismatch"
			update.Message = "agent restarted before upgrade reached restarting"
		} else {
			next = models.UpgradeSucceeded
			update.Message = "agent upgrade confirmed by reconnect"
		}
	case "rolled_back":
		next = models.UpgradeFailed
		update.ErrorCode = safeAgentUpgradeErrorCode(input.ErrorCode)
		if update.ErrorCode == "" {
			update.ErrorCode = "upgrade_rolled_back"
		}
		update.Message = "agent reported verified rollback"
	case "failed":
		next = models.UpgradeFailed
		update.ErrorCode = safeAgentUpgradeErrorCode(input.ErrorCode)
		if update.ErrorCode == "" {
			update.ErrorCode = "agent_upgrade_failed"
		}
		update.Message = "agent reported upgrade failure"
	default:
		return AgentUpgradeBootResult{Found: true}, fmt.Errorf("reconcile agent upgrade boot: unsupported outcome %q", input.Outcome)
	}

	if err := TransitionUpgradeJob(db, job.ID, next, update); err != nil {
		return AgentUpgradeBootResult{Found: true}, err
	}
	return result, nil
}

func ReconcileTimedOutUpgrades(db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("reconcile timed out upgrades: database is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var jobs []models.AgentUpgradeJob
	if err := db.Where("status IN ? AND deadline_at <= ?", nonTerminalUpgradeStatuses, now.UTC()).
		Find(&jobs).Error; err != nil {
		return 0, fmt.Errorf("list timed out upgrades: %w", err)
	}

	var transitioned int64
	for _, job := range jobs {
		err := TransitionUpgradeJob(db, job.ID, models.UpgradeTimedOut, UpgradeUpdate{
			Message:   "agent upgrade timed out",
			ErrorCode: "upgrade_timed_out",
			At:        now,
		})
		if err == nil {
			transitioned++
			continue
		}
		if errors.Is(err, ErrUpgradeTransitionInvalid) || errors.Is(err, ErrUpgradeTransitionConflict) {
			continue
		}
		return transitioned, err
	}
	return transitioned, nil
}

func isTerminalUpgradeStatus(status models.AgentUpgradeStatus) bool {
	return status == models.UpgradeSucceeded || status == models.UpgradeFailed || status == models.UpgradeTimedOut
}

func isAgentReportedUpgradeStatus(status models.AgentUpgradeStatus) bool {
	switch status {
	case models.UpgradeReceived,
		models.UpgradeDownloading,
		models.UpgradeVerifying,
		models.UpgradeApplying,
		models.UpgradeRestarting,
		models.UpgradeFailed:
		return true
	default:
		return false
	}
}

func safeAgentUpgradeErrorCode(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || len(raw) > 64 {
		return ""
	}
	for _, char := range raw {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return ""
	}
	return raw
}

func generateUpgradeJobID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate agent upgrade job id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func isUpgradeUniqueConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}
