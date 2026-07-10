package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"gorm.io/gorm"
)

var dispatchAgentUpgradesForController = func(
	ctx context.Context,
	request services.UpgradeTargetRequest,
) (services.UpgradeDispatchResult, error) {
	orchestrator := services.UpgradeOrchestrator{
		DB:        models.DB,
		Transport: activeAgentUpgradeTransport{},
	}
	return orchestrator.Dispatch(ctx, request)
}

type activeAgentUpgradeTransport struct{}

func (activeAgentUpgradeTransport) IsConnected(serverID uint) bool {
	conn, ok := ActiveAgentConnections.Current(serverID)
	return ok && conn != nil && conn.Conn != nil
}

func (activeAgentUpgradeTransport) Send(serverID uint, command services.AgentUpgradeCommand) error {
	conn, ok := ActiveAgentConnections.Current(serverID)
	if !ok || conn == nil || conn.Conn == nil {
		return fmt.Errorf("agent connection is unavailable")
	}
	return conn.WriteJSON(command)
}

func CreateAgentUpgrades(c *gin.Context) {
	var request services.UpgradeTargetRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.ServerIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_agent_upgrade_request"})
		return
	}

	result, err := dispatchAgentUpgradesForController(c.Request.Context(), request)
	if err != nil {
		status := http.StatusInternalServerError
		if services.UpgradeContractErrorCode(err) != "" || strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": "agent_upgrade_dispatch_failed"})
		return
	}

	status := http.StatusUnprocessableEntity
	if len(result.Jobs) > 0 {
		status = http.StatusAccepted
	} else if len(result.Noop) > 0 {
		status = http.StatusOK
	}
	c.JSON(status, result)
}

func ListAgentUpgradeJobs(c *gin.Context) {
	query := models.DB.Model(&models.AgentUpgradeJob{})
	if serverIDText := strings.TrimSpace(c.Query("server_id")); serverIDText != "" {
		serverID, err := strconv.ParseUint(serverIDText, 10, 32)
		if err != nil || serverID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_server_id"})
			return
		}
		query = query.Where("server_id = ?", uint(serverID))
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		if !validAgentUpgradeStatus(models.AgentUpgradeStatus(status)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_agent_upgrade_status"})
			return
		}
		query = query.Where("status = ?", status)
	}
	if activeText, exists := c.GetQuery("active"); exists {
		active, err := strconv.ParseBool(activeText)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_active_filter"})
			return
		}
		if active {
			query = query.Where("active_key IS NOT NULL")
		} else {
			query = query.Where("active_key IS NULL")
		}
	}

	limit := 100
	if limitText := strings.TrimSpace(c.Query("limit")); limitText != "" {
		parsed, err := strconv.Atoi(limitText)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_limit"})
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = parsed
	}

	var jobs []models.AgentUpgradeJob
	if err := query.Order("created_at DESC").Limit(limit).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent_upgrade_jobs_query_failed"})
		return
	}
	views := make([]services.UpgradeJobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, services.AgentUpgradeJobView(job))
	}
	c.JSON(http.StatusOK, gin.H{"jobs": views})
}

func GetAgentUpgradeJob(c *gin.Context) {
	jobID := strings.TrimSpace(c.Param("id"))
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_agent_upgrade_job_id"})
		return
	}
	var job models.AgentUpgradeJob
	if err := models.DB.First(&job, "id = ?", jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent_upgrade_job_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "agent_upgrade_job_query_failed"})
		return
	}
	c.JSON(http.StatusOK, services.AgentUpgradeJobView(job))
}

func validAgentUpgradeStatus(status models.AgentUpgradeStatus) bool {
	switch status {
	case models.UpgradeQueued,
		models.UpgradeDispatched,
		models.UpgradeReceived,
		models.UpgradeDownloading,
		models.UpgradeVerifying,
		models.UpgradeApplying,
		models.UpgradeRestarting,
		models.UpgradeSucceeded,
		models.UpgradeFailed,
		models.UpgradeTimedOut:
		return true
	default:
		return false
	}
}
