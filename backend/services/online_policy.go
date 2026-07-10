package services

import (
	"time"

	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

const (
	DefaultAgentHeartbeatInterval = 10 * time.Second
	MinimumOnlineTimeout          = 30 * time.Second
	MaximumOnlineTimeout          = 15 * time.Minute
)

func OnlineTimeout(heartbeat time.Duration) time.Duration {
	if heartbeat <= 0 {
		heartbeat = DefaultAgentHeartbeatInterval
	}
	if heartbeat >= MaximumOnlineTimeout/3 {
		return MaximumOnlineTimeout
	}
	timeout := 3 * heartbeat
	if timeout < MinimumOnlineTimeout {
		return MinimumOnlineTimeout
	}
	if timeout > MaximumOnlineTimeout {
		return MaximumOnlineTimeout
	}
	return timeout
}

func IsServerOnline(server models.Server, now time.Time) bool {
	if server.LastHeartbeat.IsZero() {
		return false
	}
	heartbeat := persistedHeartbeatInterval(server.AgentHeartbeatSeconds)
	return now.Sub(server.LastHeartbeat) <= OnlineTimeout(heartbeat)
}

func OfflineSince(server models.Server) time.Time {
	if server.LastHeartbeat.IsZero() {
		return time.Time{}
	}
	heartbeat := persistedHeartbeatInterval(server.AgentHeartbeatSeconds)
	return server.LastHeartbeat.Add(OnlineTimeout(heartbeat))
}

func persistedHeartbeatInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultAgentHeartbeatInterval
	}
	if seconds >= int(MaximumOnlineTimeout/time.Second) {
		return MaximumOnlineTimeout
	}
	return time.Duration(seconds) * time.Second
}

func ReconcileServerOnlineState(db *gorm.DB, server *models.Server, now time.Time) (bool, error) {
	online := IsServerOnline(*server, now)
	status := "offline"
	if online {
		status = "online"
	}
	if server.Online == online && server.Status == status {
		return false, nil
	}

	if err := db.Model(&models.Server{}).
		Where("id = ?", server.ID).
		Updates(map[string]interface{}{
			"online": online,
			"status": status,
		}).Error; err != nil {
		return false, err
	}
	server.Online = online
	server.Status = status
	return true, nil
}
