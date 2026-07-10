package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateAgentUpgradesMapsResultStatusAndUsesSnakeCaseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := dispatchAgentUpgradesForController
	defer func() { dispatchAgentUpgradesForController = original }()

	tests := []struct {
		name       string
		body       string
		result     services.UpgradeDispatchResult
		wantStatus int
	}{
		{name: "invalid request", body: `{}`, wantStatus: http.StatusBadRequest},
		{
			name: "accepted jobs",
			body: `{"server_ids":[7],"target_version":"1.4.0","channel":"stable","target_agent_type":"monitor"}`,
			result: services.UpgradeDispatchResult{Jobs: []services.UpgradeJobView{{
				ID: "job-1", ServerID: 7, TargetVersion: "1.4.0", TargetAgentType: "monitor", Status: models.UpgradeDispatched,
			}}},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "all noop",
			body: `{"server_ids":[7],"target_version":"1.4.0","channel":"stable","target_agent_type":"full"}`,
			result: services.UpgradeDispatchResult{Noop: []services.UpgradeNoop{{
				ServerID: 7, Version: "1.4.0", AgentType: "full", Reason: "already_at_target",
			}}},
			wantStatus: http.StatusOK,
		},
		{
			name: "all rejected",
			body: `{"server_ids":[7],"target_version":"1.4.0","channel":"stable"}`,
			result: services.UpgradeDispatchResult{Rejected: []services.UpgradeRejection{{
				ServerID: 7, Code: "agent_offline", Message: "agent is offline or disconnected",
			}}},
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured services.UpgradeTargetRequest
			dispatchAgentUpgradesForController = func(
				ctx context.Context,
				request services.UpgradeTargetRequest,
			) (services.UpgradeDispatchResult, error) {
				captured = request
				return tt.result, nil
			}

			req := httptest.NewRequest(http.MethodPost, "/api/agent-upgrades", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			CreateAgentUpgrades(c)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusBadRequest {
				return
			}
			assert.Equal(t, []uint{7}, captured.ServerIDs)
			assert.NotContains(t, w.Body.String(), "download_url")
			assert.NotContains(t, w.Body.String(), "sha256")
		})
	}
}

func TestListAndGetAgentUpgradeJobsReturnSafeFilteredViews(t *testing.T) {
	db := setupAgentUpgradeControllerDB(t)
	models.DB = db
	now := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	server := models.Server{
		Name:             "controller-server",
		AgentVersion:     "1.3.0",
		AgentType:        "full",
		DesiredAgentType: "full",
	}
	require.NoError(t, db.Create(&server).Error)
	job, err := services.CreateAgentUpgradeJob(db, services.CreateUpgradeJobInput{
		ServerID:        server.ID,
		Trigger:         "manual",
		TargetVersion:   "1.4.0",
		TargetAgentType: "monitor",
		Channel:         "stable",
		AssetName:       "better-monitor-agent-monitor-1.4.0-linux-amd64",
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example.test/private-agent-url",
		SHA256:          "1111111111111111111111111111111111111111111111111111111111111111",
		Now:             now,
	})
	require.NoError(t, err)

	t.Run("list active by server", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/agent-upgrades?server_id=1&active=true&limit=20", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		ListAgentUpgradeJobs(c)

		require.Equal(t, http.StatusOK, w.Code)
		var response struct {
			Jobs []services.UpgradeJobView `json:"jobs"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response.Jobs, 1)
		assert.Equal(t, job.ID, response.Jobs[0].ID)
		assert.NotContains(t, w.Body.String(), "private-agent-url")
		assert.NotContains(t, w.Body.String(), job.SHA256)
	})

	t.Run("get exact job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/agent-upgrades/"+job.ID, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: job.ID}}
		c.Request = req

		GetAgentUpgradeJob(c)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), job.ID)
		assert.NotContains(t, w.Body.String(), "download_url")
		assert.NotContains(t, w.Body.String(), "sha256")
	})

	t.Run("get missing job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/agent-upgrades/missing", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "missing"}}
		c.Request = req

		GetAgentUpgradeJob(c)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupAgentUpgradeControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.AgentUpgradeJob{}, &models.SystemSettings{}))
	return db
}
