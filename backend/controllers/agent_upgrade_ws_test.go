package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
)

func createUpgradeJobForWebSocketTest(
	t *testing.T,
	server *models.Server,
	targetVersion, targetType string,
	status models.AgentUpgradeStatus,
) *models.AgentUpgradeJob {
	t.Helper()
	require.NoError(t, models.DB.Model(&models.Server{}).Where("id = ?", server.ID).Updates(map[string]interface{}{
		"agent_version":      "1.2.3",
		"agent_type":         "full",
		"desired_agent_type": "full",
	}).Error)
	server.AgentVersion = "1.2.3"
	server.AgentType = "full"
	server.DesiredAgentType = "full"

	job, err := services.CreateAgentUpgradeJob(models.DB, services.CreateUpgradeJobInput{
		ServerID:        server.ID,
		Trigger:         "manual",
		TargetVersion:   targetVersion,
		TargetAgentType: targetType,
		Channel:         "stable",
		AssetName:       "better-monitor-agent-" + targetVersion + "-linux-amd64",
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example/agent",
		SHA256:          strings.Repeat("a", 64),
		Now:             time.Now().UTC(),
	})
	require.NoError(t, err)

	sequence := []models.AgentUpgradeStatus{
		models.UpgradeDispatched,
		models.UpgradeReceived,
		models.UpgradeDownloading,
		models.UpgradeVerifying,
		models.UpgradeApplying,
		models.UpgradeRestarting,
	}
	for _, next := range sequence {
		if job.Status == status {
			break
		}
		require.NoError(t, services.TransitionUpgradeJob(models.DB, job.ID, next, services.UpgradeUpdate{
			At: time.Now().UTC(),
		}))
		job.Status = next
	}
	require.Equal(t, status, job.Status)
	return job
}

func dialAgentWebSocket(
	t *testing.T,
	fixture *websocketAuthFixture,
	version, agentType string,
	hello map[string]interface{},
) *websocket.Conn {
	t.Helper()
	headers := agentHeaders(fixture.server.SecretKey)
	headers.Set("X-Agent-Version", version)
	headers.Set("X-Agent-Type", agentType)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)
	conn, resp, err := websocket.DefaultDialer.Dial(url, headers)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.WriteJSON(hello))
	return conn
}

func upgradeHello(version, agentType string, report map[string]interface{}) map[string]interface{} {
	hello := map[string]interface{}{
		"type":                       "agent_hello",
		"version":                    version,
		"agent_type":                 agentType,
		"heartbeat_interval_seconds": 10,
	}
	if report != nil {
		hello["upgrade_report"] = report
	}
	return hello
}

func loadUpgradeJob(t *testing.T, id string) models.AgentUpgradeJob {
	t.Helper()
	var job models.AgentUpgradeJob
	require.NoError(t, models.DB.First(&job, "id = ?", id).Error)
	return job
}

func TestAgentUpgradeStatusPersistsOnlyValidCurrentJobTransitions(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	job := createUpgradeJobForWebSocketTest(t, fixture.server, "2.0.0", "full", models.UpgradeDispatched)
	conn := dialAgentWebSocket(t, fixture, "1.2.3", "full", upgradeHello("1.2.3", "full", nil))
	require.Eventually(t, func() bool {
		_, ok := ActiveAgentConnections.Current(fixture.server.ID)
		return ok
	}, time.Second, 10*time.Millisecond)

	sensitiveMessage := "downloaded https://secret.example/agent sha=" + strings.Repeat("b", 64)
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":       "agent_upgrade_status",
		"request_id": job.ID,
		"status":     "received",
		"message":    sensitiveMessage,
	}))
	require.Eventually(t, func() bool {
		return loadUpgradeJob(t, job.ID).Status == models.UpgradeReceived
	}, time.Second, 10*time.Millisecond)
	received := loadUpgradeJob(t, job.ID)
	assert.NotContains(t, received.LastMessage, "https://")
	assert.NotContains(t, received.LastMessage, strings.Repeat("b", 64))

	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":             "agent_upgrade_status",
		"request_id":       job.ID,
		"status":           "downloading",
		"bytes_downloaded": 100,
	}))
	require.Eventually(t, func() bool {
		stored := loadUpgradeJob(t, job.ID)
		return stored.Status == models.UpgradeDownloading && stored.BytesDownloaded == 100
	}, time.Second, 10*time.Millisecond)

	for _, invalid := range []map[string]interface{}{
		{
			"type":             "agent_upgrade_status",
			"request_id":       job.ID,
			"status":           "downloading",
			"bytes_downloaded": 50,
		},
		{
			"type":       "agent_upgrade_status",
			"request_id": job.ID,
			"status":     "applying",
		},
		{
			"type":       "agent_upgrade_status",
			"request_id": "stale-request-id",
			"status":     "verifying",
		},
		{
			"type":       "agent_upgrade_" + "response",
			"request_id": job.ID,
			"data": map[string]interface{}{
				"status": "verifying",
			},
		},
	} {
		require.NoError(t, conn.WriteJSON(invalid))
	}
	time.Sleep(50 * time.Millisecond)
	unchanged := loadUpgradeJob(t, job.ID)
	assert.Equal(t, models.UpgradeDownloading, unchanged.Status)
	assert.Equal(t, int64(100), unchanged.BytesDownloaded)

	other := &models.Server{Name: "other", SecretKey: "other-secret", AgentVersion: "1.2.3", AgentType: "full"}
	require.NoError(t, models.DB.Create(other).Error)
	otherJob := createUpgradeJobForWebSocketTest(t, other, "2.0.0", "full", models.UpgradeDispatched)
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":       "agent_upgrade_status",
		"request_id": otherJob.ID,
		"status":     "received",
	}))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, models.UpgradeDispatched, loadUpgradeJob(t, otherJob.ID).Status)

	require.NoError(t, services.TransitionUpgradeJob(models.DB, job.ID, models.UpgradeFailed, services.UpgradeUpdate{
		ErrorCode: "test_failed",
		At:        time.Now().UTC(),
	}))
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":       "agent_upgrade_status",
		"request_id": job.ID,
		"status":     "received",
	}))
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, models.UpgradeFailed, loadUpgradeJob(t, job.ID).Status)
}

func TestAgentSystemInfoCannotOverrideHelloVersionOrType(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	conn := dialAgentWebSocket(t, fixture, "1.2.3", "full", upgradeHello("1.2.3", "full", nil))
	require.Eventually(t, func() bool {
		_, ok := ActiveAgentConnections.Current(fixture.server.ID)
		return ok
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type": "system_info",
		"payload": map[string]interface{}{
			"agent_version": "9.9.9",
			"agent_type":    "monitor",
			"os":            "linux",
			"kernel_arch":   "x86_64",
		},
	}))
	time.Sleep(50 * time.Millisecond)

	var stored models.Server
	require.NoError(t, models.DB.First(&stored, fixture.server.ID).Error)
	assert.Equal(t, "1.2.3", stored.AgentVersion)
	assert.Equal(t, "full", stored.AgentType)
}

func TestAgentUpgradeReconnectConfirmsBootReportAgainstJobIdentity(t *testing.T) {
	tests := []struct {
		name          string
		reportOutcome string
		reportID      string
		helloVersion  string
		helloType     string
		terminal      models.AgentUpgradeStatus
		wantStatus    models.AgentUpgradeStatus
		wantConfirmed bool
	}{
		{
			name:          "matching applied report succeeds",
			reportOutcome: "applied",
			helloVersion:  "2.0.0",
			helloType:     "monitor",
			wantStatus:    models.UpgradeSucceeded,
			wantConfirmed: true,
		},
		{
			name:          "applied identity mismatch fails",
			reportOutcome: "applied",
			helloVersion:  "2.0.1",
			helloType:     "full",
			wantStatus:    models.UpgradeFailed,
			wantConfirmed: true,
		},
		{
			name:          "rolled back report fails",
			reportOutcome: "rolled_back",
			helloVersion:  "1.2.3",
			helloType:     "full",
			wantStatus:    models.UpgradeFailed,
			wantConfirmed: true,
		},
		{
			name:          "unknown request is not confirmed",
			reportOutcome: "applied",
			reportID:      "unknown-request-id",
			helloVersion:  "2.0.0",
			helloType:     "monitor",
			wantStatus:    models.UpgradeRestarting,
			wantConfirmed: false,
		},
		{
			name:          "late report leaves timed out job terminal",
			reportOutcome: "applied",
			helloVersion:  "2.0.0",
			helloType:     "monitor",
			terminal:      models.UpgradeTimedOut,
			wantStatus:    models.UpgradeTimedOut,
			wantConfirmed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := setupWebSocketAuthFixture(t)
			job := createUpgradeJobForWebSocketTest(t, fixture.server, "2.0.0", "monitor", models.UpgradeRestarting)
			if tt.terminal != "" {
				require.NoError(t, services.TransitionUpgradeJob(models.DB, job.ID, tt.terminal, services.UpgradeUpdate{
					ErrorCode: "upgrade_timed_out",
					At:        time.Now().UTC(),
				}))
			}
			reportID := tt.reportID
			if reportID == "" {
				reportID = job.ID
			}
			conn := dialAgentWebSocket(t, fixture, tt.helloVersion, tt.helloType, upgradeHello(
				tt.helloVersion,
				tt.helloType,
				map[string]interface{}{
					"request_id": reportID,
					"outcome":    tt.reportOutcome,
					"error_code": "rollback_verified",
				},
			))
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			var ack struct {
				Type      string `json:"type"`
				RequestID string `json:"request_id"`
				Confirmed bool   `json:"confirmed"`
			}
			require.NoError(t, conn.ReadJSON(&ack))
			assert.Equal(t, "agent_upgrade_ack", ack.Type)
			assert.Equal(t, reportID, ack.RequestID)
			assert.Equal(t, tt.wantConfirmed, ack.Confirmed)

			require.Eventually(t, func() bool {
				return loadUpgradeJob(t, job.ID).Status == tt.wantStatus
			}, time.Second, 10*time.Millisecond)

			var storedServer models.Server
			require.NoError(t, models.DB.First(&storedServer, fixture.server.ID).Error)
			assert.Equal(t, tt.helloVersion, storedServer.AgentVersion)
			assert.Equal(t, tt.helloType, storedServer.AgentType)
			if tt.wantStatus == models.UpgradeSucceeded || tt.wantStatus == models.UpgradeFailed {
				assert.Equal(t, storedServer.AgentType, storedServer.DesiredAgentType)
			}
		})
	}
}
