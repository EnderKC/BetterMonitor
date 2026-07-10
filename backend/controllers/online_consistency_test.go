package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
)

func TestOnlineStatusConsumersAgreeOnUnifiedPolicy(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	now := time.Now()
	lastHeartbeat := now.Add(-59 * time.Second)
	require.NoError(t, models.DB.Model(&models.Server{}).
		Where("id = ?", fixture.public.ID).
		Updates(map[string]interface{}{
			"agent_heartbeat_seconds": 20,
			"last_heartbeat":          lastHeartbeat,
			"online":                  false,
			"status":                  "offline",
		}).Error)

	statusResponse, err := http.Get(fmt.Sprintf("%s/api/servers/%d/status", fixture.httpServer.URL, fixture.public.ID))
	require.NoError(t, err)
	defer statusResponse.Body.Close()
	require.Equal(t, http.StatusOK, statusResponse.StatusCode)
	var statusPayload struct {
		Online bool   `json:"online"`
		Status string `json:"status"`
	}
	require.NoError(t, json.NewDecoder(statusResponse.Body).Decode(&statusPayload))
	assert.True(t, statusPayload.Online)
	assert.Equal(t, "online", statusPayload.Status)

	versionsResponse, err := http.Get(fixture.httpServer.URL + "/api/servers/versions")
	require.NoError(t, err)
	defer versionsResponse.Body.Close()
	var versions []struct {
		ID     uint `json:"id"`
		Status int  `json:"status"`
	}
	require.NoError(t, json.NewDecoder(versionsResponse.Body).Decode(&versions))
	versionStatus := -1
	for _, item := range versions {
		if item.ID == fixture.public.ID {
			versionStatus = item.Status
		}
	}
	assert.Equal(t, 1, versionStatus)

	conn, _, err := websocket.DefaultDialer.Dial(fixture.websocketURL+"/api/servers/public/ws", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var listPayload struct {
		Servers []struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"servers"`
	}
	require.NoError(t, conn.ReadJSON(&listPayload))
	listStatus := "missing"
	for _, item := range listPayload.Servers {
		if item.ID == fixture.public.ID {
			listStatus = item.Status
		}
	}
	assert.Equal(t, "online", listStatus)

	var server models.Server
	require.NoError(t, models.DB.First(&server, fixture.public.ID).Error)
	assert.True(t, services.IsServerOnline(server, time.Now()))
	assert.True(t, services.OfflineSince(server).Equal(lastHeartbeat.Add(time.Minute)))
}

func TestAgentDisconnectWaitsForUnifiedTimeoutBeforeOffline(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)
	conn, _, err := websocket.DefaultDialer.Dial(url, agentHeaders(fixture.server.SecretKey))
	require.NoError(t, err)
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":                       "agent_hello",
		"version":                    "1.2.3",
		"agent_type":                 "full",
		"heartbeat_interval_seconds": 10,
	}))
	require.Eventually(t, func() bool {
		_, ok := ActiveAgentConnections.Current(fixture.server.ID)
		return ok
	}, time.Second, 10*time.Millisecond)

	_ = conn.Close()
	require.Eventually(t, func() bool {
		_, ok := ActiveAgentConnections.Current(fixture.server.ID)
		return !ok
	}, time.Second, 10*time.Millisecond)

	var server models.Server
	require.NoError(t, models.DB.First(&server, fixture.server.ID).Error)
	assert.True(t, server.Online)
	assert.Equal(t, "online", server.Status)
	assert.True(t, services.IsServerOnline(server, server.LastHeartbeat.Add(29*time.Second)))

	changed, err := services.ReconcileServerOnlineState(
		models.DB,
		&server,
		server.LastHeartbeat.Add(services.OnlineTimeout(10*time.Second)+time.Nanosecond),
	)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.False(t, server.Online)
	assert.Equal(t, "offline", server.Status)
}
