package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
)

func TestOnlineTimeoutClamps(t *testing.T) {
	assert.Equal(t, 30*time.Second, OnlineTimeout(0))
	assert.Equal(t, 30*time.Second, OnlineTimeout(5*time.Second))
	assert.Equal(t, 30*time.Second, OnlineTimeout(10*time.Second))
	assert.Equal(t, 15*time.Minute, OnlineTimeout(10*time.Minute))
}

func TestIsServerOnlineUsesPersistedHeartbeatInterval(t *testing.T) {
	now := time.Date(2026, 7, 10, 18, 0, 0, 0, time.UTC)
	server := models.Server{
		AgentHeartbeatSeconds: 20,
		LastHeartbeat:         now.Add(-60 * time.Second),
		Online:                false,
		Status:                "offline",
	}
	assert.True(t, IsServerOnline(server, now), "timeout boundary is inclusive")

	server.LastHeartbeat = now.Add(-60*time.Second - time.Nanosecond)
	assert.False(t, IsServerOnline(server, now))

	server.AgentHeartbeatSeconds = 0
	server.LastHeartbeat = now.Add(-30 * time.Second)
	assert.True(t, IsServerOnline(server, now), "zero interval uses the ten-second default")

	server.LastHeartbeat = time.Time{}
	assert.False(t, IsServerOnline(server, now))
}

func TestOfflineSinceUsesSameTimeoutPolicy(t *testing.T) {
	lastHeartbeat := time.Date(2026, 7, 10, 18, 0, 0, 0, time.UTC)
	server := models.Server{
		AgentHeartbeatSeconds: 20,
		LastHeartbeat:         lastHeartbeat,
	}
	assert.Equal(t, lastHeartbeat.Add(time.Minute), OfflineSince(server))

	server.LastHeartbeat = time.Time{}
	assert.True(t, OfflineSince(server).IsZero())
}

func TestReconcileServerOnlineStateUpdatesOnlyAfterTimeout(t *testing.T) {
	now := time.Date(2026, 7, 10, 18, 0, 0, 0, time.UTC)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}))

	server := models.Server{
		Name:                  "server",
		AgentHeartbeatSeconds: 10,
		LastHeartbeat:         now.Add(-29 * time.Second),
		Online:                true,
		Status:                "online",
	}
	require.NoError(t, db.Create(&server).Error)

	changed, err := ReconcileServerOnlineState(db, &server, now)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.True(t, server.Online)

	changed, err = ReconcileServerOnlineState(db, &server, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.True(t, changed)
	assert.False(t, server.Online)
	assert.Equal(t, "offline", server.Status)

	var persisted models.Server
	require.NoError(t, db.First(&persisted, server.ID).Error)
	assert.False(t, persisted.Online)
	assert.Equal(t, "offline", persisted.Status)

	changed, err = ReconcileServerOnlineState(db, &server, now.Add(3*time.Second))
	require.NoError(t, err)
	assert.False(t, changed, "already reconciled state is not rebroadcast")
}
