package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigDefaultsHeartbeatIntervalToTenSeconds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server_url: monitor.example.com\n"), 0o600))

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.HeartbeatInterval)
}

func TestSaveConfigPersistsHeartbeatInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	cfg := &Config{
		ServerURL:         "monitor.example.com",
		ServerID:          7,
		SecretKey:         "secret",
		AgentType:         "full",
		HeartbeatInterval: 12 * time.Second,
		MonitorInterval:   30 * time.Second,
	}

	require.NoError(t, SaveConfig(cfg, path))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "heartbeat_interval: 12s")
}
