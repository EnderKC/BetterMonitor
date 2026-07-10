package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-backend/models"
	"gorm.io/gorm"
)

func TestDispatchAgentUpgradesHandlesNoopCrossTypeAndPerServerFailures(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SystemSettings{
		AgentReleaseRepo:    "demo/better-monitor",
		AgentReleaseChannel: "stable",
	}).Error)

	noop := createOrchestratorServer(t, db, now, "1.4.0", "full", "linux", "amd64", true)
	crossType := createOrchestratorServer(t, db, now, "1.4.0", "full", "linux", "amd64", true)
	valid := createOrchestratorServer(t, db, now, "1.3.0", "full", "linux", "amd64", true)
	offline := createOrchestratorServer(t, db, now.Add(-time.Hour), "1.3.0", "full", "linux", "amd64", false)
	disconnected := createOrchestratorServer(t, db, now, "1.3.0", "full", "linux", "amd64", true)
	missingAsset := createOrchestratorServer(t, db, now, "1.3.0", "full", "darwin", "amd64", true)
	sendError := createOrchestratorServer(t, db, now, "1.3.0", "full", "linux", "amd64", true)
	active := createOrchestratorServer(t, db, now, "1.3.0", "full", "linux", "amd64", true)
	_, err := CreateAgentUpgradeJob(db, CreateUpgradeJobInput{
		ServerID:        active.ID,
		Trigger:         "manual",
		TargetVersion:   "1.4.0",
		TargetAgentType: "monitor",
		Channel:         "stable",
		AssetName:       "better-monitor-agent-monitor-1.4.0-linux-amd64",
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example.test/agent",
		SHA256:          "1111111111111111111111111111111111111111111111111111111111111111",
		Now:             now,
	})
	require.NoError(t, err)

	transport := &fakeUpgradeTransport{
		connected: map[uint]bool{
			crossType.ID:    true,
			valid.ID:        true,
			missingAsset.ID: true,
			sendError.ID:    true,
			active.ID:       true,
		},
		sendErrors: map[uint]error{sendError.ID: fmt.Errorf("send failed")},
	}
	var resolverCalls int
	orchestrator := UpgradeOrchestrator{
		DB:        db,
		Transport: transport,
		Now:       func() time.Time { return now },
		ResolveAsset: func(
			ctx context.Context,
			settings *models.SystemSettings,
			request ResolveUpgradeRequest,
		) (AgentUpgradeAsset, error) {
			resolverCalls++
			if request.OS == "darwin" {
				return AgentUpgradeAsset{}, contractError("release_asset_missing", fmt.Errorf("missing asset"))
			}
			version := request.TargetVersion
			if version == "" {
				version = "1.4.0"
			}
			name, err := CanonicalAgentAssetName(version, request.OS, request.Arch, request.AgentType)
			if err != nil {
				return AgentUpgradeAsset{}, err
			}
			return AgentUpgradeAsset{
				Version:     version,
				Channel:     request.Channel,
				AgentType:   request.AgentType,
				OS:          request.OS,
				Arch:        NormalizeArch(request.Arch),
				Name:        name,
				DownloadURL: "https://downloads.example.test/" + name,
				Size:        1024,
				SHA256:      "1111111111111111111111111111111111111111111111111111111111111111",
			}, nil
		},
	}

	result, err := orchestrator.Dispatch(context.Background(), UpgradeTargetRequest{
		ServerIDs: []uint{
			noop.ID,
			crossType.ID,
			valid.ID,
			offline.ID,
			disconnected.ID,
			missingAsset.ID,
			sendError.ID,
			active.ID,
			999999,
		},
		TargetVersion:   "1.4.0",
		Channel:         "stable",
		TargetAgentType: "monitor",
	})
	require.NoError(t, err)

	assert.Len(t, result.Jobs, 3)
	assert.Len(t, result.Noop, 0, "cross-type requests are executable even at the same version")
	assertRejectionCode(t, result.Rejected, offline.ID, "agent_offline")
	assertRejectionCode(t, result.Rejected, disconnected.ID, "agent_offline")
	assertRejectionCode(t, result.Rejected, missingAsset.ID, "release_asset_missing")
	assertRejectionCode(t, result.Rejected, active.ID, "upgrade_already_active")
	assertRejectionCode(t, result.Rejected, 999999, "server_not_found")
	assert.Equal(t, 4, resolverCalls, "active/offline/missing/noop decisions must avoid unnecessary resolver calls")

	jobByServer := make(map[uint]UpgradeJobView)
	for _, job := range result.Jobs {
		jobByServer[job.ServerID] = job
	}
	assert.Equal(t, models.UpgradeDispatched, jobByServer[crossType.ID].Status)
	assert.Equal(t, models.UpgradeDispatched, jobByServer[valid.ID].Status)
	assert.Equal(t, models.UpgradeFailed, jobByServer[sendError.ID].Status)
	assert.Equal(t, "dispatch_failed", jobByServer[sendError.ID].ErrorCode)
	assert.Len(t, transport.commands(), 3)

	var crossTypeServer models.Server
	require.NoError(t, db.First(&crossTypeServer, crossType.ID).Error)
	assert.Equal(t, "full", crossTypeServer.AgentType)
	assert.Equal(t, "monitor", crossTypeServer.DesiredAgentType)

	var failedServer models.Server
	require.NoError(t, db.First(&failedServer, sendError.ID).Error)
	assert.Equal(t, "full", failedServer.AgentType)
	assert.Equal(t, "full", failedServer.DesiredAgentType)

	noopOnly, err := orchestrator.Dispatch(context.Background(), UpgradeTargetRequest{
		ServerIDs:       []uint{noop.ID},
		TargetVersion:   "1.4.0",
		Channel:         "stable",
		TargetAgentType: "full",
	})
	require.NoError(t, err)
	require.Len(t, noopOnly.Noop, 1)
	assert.Equal(t, noop.ID, noopOnly.Noop[0].ServerID)
	assert.Empty(t, noopOnly.Jobs)
}

func TestDispatchAgentUpgradesDoesNotCreateJobBeforeCompletePreflight(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SystemSettings{
		AgentReleaseRepo:    "demo/better-monitor",
		AgentReleaseChannel: "stable",
	}).Error)
	server := createOrchestratorServer(t, db, now, "1.3.0", "full", "linux", "amd64", true)

	orchestrator := UpgradeOrchestrator{
		DB:        db,
		Transport: &fakeUpgradeTransport{connected: map[uint]bool{server.ID: true}},
		Now:       func() time.Time { return now },
		ResolveAsset: func(context.Context, *models.SystemSettings, ResolveUpgradeRequest) (AgentUpgradeAsset, error) {
			return AgentUpgradeAsset{}, contractError("release_checksum_missing", fmt.Errorf("missing checksum"))
		},
	}
	result, err := orchestrator.Dispatch(context.Background(), UpgradeTargetRequest{
		ServerIDs:     []uint{server.ID},
		TargetVersion: "1.4.0",
		Channel:       "stable",
	})
	require.NoError(t, err)
	assert.Empty(t, result.Jobs)
	assertRejectionCode(t, result.Rejected, server.ID, "release_checksum_missing")

	var count int64
	require.NoError(t, db.Model(&models.AgentUpgradeJob{}).Count(&count).Error)
	assert.Zero(t, count)
}

type fakeUpgradeTransport struct {
	mu         sync.Mutex
	connected  map[uint]bool
	sendErrors map[uint]error
	sent       []AgentUpgradeCommand
}

func (t *fakeUpgradeTransport) IsConnected(serverID uint) bool {
	return t != nil && t.connected[serverID]
}

func (t *fakeUpgradeTransport) Send(serverID uint, command AgentUpgradeCommand) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append(t.sent, command)
	return t.sendErrors[serverID]
}

func (t *fakeUpgradeTransport) commands() []AgentUpgradeCommand {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]AgentUpgradeCommand(nil), t.sent...)
}

func createOrchestratorServer(
	t *testing.T,
	db *gorm.DB,
	heartbeat time.Time,
	version, agentType, goos, arch string,
	online bool,
) models.Server {
	t.Helper()
	server := models.Server{
		Name:                  "orchestrator-server",
		OS:                    goos,
		Arch:                  arch,
		AgentVersion:          version,
		AgentType:             agentType,
		DesiredAgentType:      agentType,
		AgentHeartbeatSeconds: 10,
		LastHeartbeat:         heartbeat,
		Online:                online,
		Status:                map[bool]string{true: "online", false: "offline"}[online],
	}
	require.NoError(t, db.Create(&server).Error)
	return server
}

func assertRejectionCode(t *testing.T, rejections []UpgradeRejection, serverID uint, code string) {
	t.Helper()
	for _, rejection := range rejections {
		if rejection.ServerID == serverID {
			assert.Equal(t, code, rejection.Code)
			return
		}
	}
	t.Fatalf("missing rejection for server %d: %#v", serverID, rejections)
}
