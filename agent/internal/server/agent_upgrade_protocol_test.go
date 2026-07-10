package server

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-agent/config"
	"github.com/user/server-ops-agent/internal/upgrader"
	"github.com/user/server-ops-agent/pkg/logger"
)

func TestDecodeAgentUpgradeInstructionRequiresTypedPayloadAndBackendRequestID(t *testing.T) {
	assetName := "better-monitor-agent-monitor-1.4.0-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	message, err := json.Marshal(map[string]interface{}{
		"type":       "agent_upgrade",
		"request_id": "job-request-id",
		"payload": map[string]interface{}{
			"target_version":    "1.4.0",
			"target_agent_type": "monitor",
			"asset_name":        assetName,
			"asset_size":        1024,
			"download_url":      "https://downloads.example/" + assetName,
			"sha256":            strings.Repeat("a", 64),
		},
	})
	require.NoError(t, err)

	instruction, err := decodeAgentUpgradeInstruction(message)
	require.NoError(t, err)
	assert.Equal(t, "job-request-id", instruction.RequestID)
	assert.Equal(t, "monitor", instruction.TargetAgentType)

	withoutRequestID := strings.Replace(string(message), `"request_id":"job-request-id",`, "", 1)
	_, err = decodeAgentUpgradeInstruction([]byte(withoutRequestID))
	assert.Error(t, err)

	legacyData := strings.Replace(string(message), `"payload":`, `"data":`, 1)
	_, err = decodeAgentUpgradeInstruction([]byte(legacyData))
	assert.Error(t, err)
}

func TestAgentUpgradeFailureCodePreservesStableUpgraderCode(t *testing.T) {
	assert.Equal(t, "download_sha_mismatch", agentUpgradeFailureCode(&upgrader.UpgradeError{Code: "download_sha_mismatch"}))
	assert.Equal(t, "upgrade_failed", agentUpgradeFailureCode(errors.New("unexpected failure")))
}

func TestAgentUpgradeStatusUsesFlatSafeProjection(t *testing.T) {
	message := buildAgentUpgradeStatus("job-request-id", "downloading", 1024, "", "safe progress")
	raw, err := json.Marshal(message)
	require.NoError(t, err)

	assert.Contains(t, string(raw), `"type":"agent_upgrade_status"`)
	assert.Contains(t, string(raw), `"bytes_downloaded":1024`)
	assert.NotContains(t, string(raw), `"payload"`)
	assert.NotContains(t, string(raw), "download_url")
	assert.NotContains(t, string(raw), "sha256")
	assert.NotContains(t, string(raw), "secret")
}

func TestAgentHelloUpgradeReportRepeatsUntilExactConfirmedAck(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "upgrade-marker.json")
	marker := upgrader.UpgradeMarker{
		RequestID:       "job-request-id",
		TargetVersion:   "1.4.0",
		TargetAgentType: "monitor",
		Outcome:         "applied",
	}
	require.NoError(t, upgrader.WriteUpgradeMarker(markerPath, marker))
	log, err := logger.New("", "error")
	require.NoError(t, err)
	client := New(&config.Config{HeartbeatInterval: 10}, log)
	client.upgradeMarkerPath = markerPath

	first := client.agentHello()
	require.NotNil(t, first.UpgradeReport)
	assert.Equal(t, marker.RequestID, first.UpgradeReport.RequestID)

	client.handleAgentUpgradeAck([]byte(`{"type":"agent_upgrade_ack","request_id":"wrong","confirmed":true}`))
	second := client.agentHello()
	require.NotNil(t, second.UpgradeReport)

	client.handleAgentUpgradeAck([]byte(`{"type":"agent_upgrade_ack","request_id":"job-request-id","confirmed":false}`))
	third := client.agentHello()
	require.NotNil(t, third.UpgradeReport)

	client.handleAgentUpgradeAck([]byte(`{"type":"agent_upgrade_ack","request_id":"job-request-id","confirmed":true}`))
	fourth := client.agentHello()
	assert.Nil(t, fourth.UpgradeReport)
}
