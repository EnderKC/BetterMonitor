package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AgentHello struct {
	Type                     string `json:"type"`
	Version                  string `json:"version"`
	AgentType                string `json:"agent_type"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	UpgradeRequestID         string `json:"upgrade_request_id"`
}

type AgentHeartbeat struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
}

func BuildAgentWebSocketURL(serverURL string, serverID uint) (string, error) {
	if serverID == 0 {
		return "", fmt.Errorf("server id is required")
	}
	rawURL := strings.TrimSpace(serverURL)
	if rawURL == "" {
		return "", fmt.Errorf("server url is required")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid server url")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "ws":
		parsed.Scheme = "ws"
	case "https", "wss":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported server url scheme")
	}

	parsed.Path = fmt.Sprintf("/api/servers/%d/agent-ws", serverID)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func BuildAgentWebSocketHeaders(secret string, hello AgentHello) http.Header {
	headers := make(http.Header)
	headers.Set("X-Secret-Key", secret)
	headers.Set("X-Agent-Version", hello.Version)
	headers.Set("X-Agent-Type", hello.AgentType)
	headers.Set("X-Agent-Heartbeat-Seconds", strconv.Itoa(hello.HeartbeatIntervalSeconds))
	return headers
}

func runAgentHeartbeat(
	ctx context.Context,
	interval time.Duration,
	now func() time.Time,
	send func(AgentHeartbeat) error,
) error {
	if ctx == nil || interval <= 0 || now == nil || send == nil {
		return fmt.Errorf("invalid agent heartbeat configuration")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := send(AgentHeartbeat{
				Type:      "agent_heartbeat",
				Timestamp: now().Unix(),
			}); err != nil {
				return err
			}
		}
	}
}
