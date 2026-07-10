package services

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-backend/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateAgentUpgradeJobUsesRandomIDAndOneActiveJobPerServer(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	server := createUpgradeStateServer(t, db, "1.3.0", "full")
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	var success atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var ids sync.Map
	var wg sync.WaitGroup
	for index := 0; index < 20; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := CreateAgentUpgradeJob(db, CreateUpgradeJobInput{
				ServerID:        server.ID,
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
			switch {
			case err == nil:
				success.Add(1)
				ids.Store(job.ID, struct{}{})
			case errors.Is(err, ErrActiveUpgradeExists):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), success.Load())
	assert.Equal(t, int32(19), conflicts.Load())
	assert.Zero(t, unexpected.Load())
	var idCount int
	ids.Range(func(key, value interface{}) bool {
		idCount++
		assert.GreaterOrEqual(t, len(key.(string)), 22)
		return true
	})
	assert.Equal(t, 1, idCount)

	var stored models.AgentUpgradeJob
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, models.UpgradeQueued, stored.Status)
	assert.NotNil(t, stored.ActiveKey)
	assert.Equal(t, "active", *stored.ActiveKey)
	assert.Equal(t, now.Add(DefaultAgentUpgradeTimeout), stored.DeadlineAt)

	var updatedServer models.Server
	require.NoError(t, db.First(&updatedServer, server.ID).Error)
	assert.Equal(t, "full", updatedServer.AgentType)
	assert.Equal(t, "monitor", updatedServer.DesiredAgentType)
}

func TestTransitionUpgradeJobEnforcesStateMachineAndMonotonicProgress(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	server := createUpgradeStateServer(t, db, "1.3.0", "full")
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	job := createUpgradeStateJob(t, db, server, now, "monitor")

	err := TransitionUpgradeJob(db, job.ID, models.UpgradeReceived, UpgradeUpdate{At: now.Add(time.Second)})
	require.ErrorIs(t, err, ErrUpgradeTransitionInvalid)
	assertUpgradeJobStatus(t, db, job.ID, models.UpgradeQueued, 0)

	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeDispatched, UpgradeUpdate{
		Message: "sent",
		At:      now.Add(2 * time.Second),
	}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeDispatched, UpgradeUpdate{
		Message: "duplicate acknowledgement",
		At:      now.Add(3 * time.Second),
	}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeReceived, UpgradeUpdate{At: now.Add(4 * time.Second)}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeDownloading, UpgradeUpdate{
		BytesDownloaded: 100,
		At:              now.Add(5 * time.Second),
	}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeDownloading, UpgradeUpdate{
		BytesDownloaded: 150,
		At:              now.Add(6 * time.Second),
	}))

	err = TransitionUpgradeJob(db, job.ID, models.UpgradeDownloading, UpgradeUpdate{
		BytesDownloaded: 149,
		At:              now.Add(7 * time.Second),
	})
	require.ErrorIs(t, err, ErrUpgradeProgressRegressed)
	assertUpgradeJobStatus(t, db, job.ID, models.UpgradeDownloading, 150)

	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeVerifying, UpgradeUpdate{At: now.Add(8 * time.Second)}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeApplying, UpgradeUpdate{At: now.Add(9 * time.Second)}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeRestarting, UpgradeUpdate{At: now.Add(10 * time.Second)}))
	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeSucceeded, UpgradeUpdate{At: now.Add(11 * time.Second)}))

	var stored models.AgentUpgradeJob
	require.NoError(t, db.First(&stored, "id = ?", job.ID).Error)
	assert.Equal(t, models.UpgradeSucceeded, stored.Status)
	assert.Nil(t, stored.ActiveKey)
	require.NotNil(t, stored.CompletedAt)
	assert.Equal(t, now.Add(11*time.Second), *stored.CompletedAt)

	err = TransitionUpgradeJob(db, job.ID, models.UpgradeFailed, UpgradeUpdate{ErrorCode: "late_failure"})
	require.ErrorIs(t, err, ErrUpgradeTransitionInvalid)
	assertUpgradeJobStatus(t, db, job.ID, models.UpgradeSucceeded, 150)
}

func TestTransitionUpgradeJobFailureReleasesActiveJobAndRestoresDesiredType(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	server := createUpgradeStateServer(t, db, "1.3.0", "full")
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	job := createUpgradeStateJob(t, db, server, now, "monitor")

	require.NoError(t, TransitionUpgradeJob(db, job.ID, models.UpgradeFailed, UpgradeUpdate{
		ErrorCode: "dispatch_failed",
		Message:   "dispatch failed",
		At:        now.Add(time.Second),
	}))

	var failed models.AgentUpgradeJob
	require.NoError(t, db.First(&failed, "id = ?", job.ID).Error)
	assert.Equal(t, models.UpgradeFailed, failed.Status)
	assert.Nil(t, failed.ActiveKey)
	assert.Equal(t, "dispatch_failed", failed.ErrorCode)

	var updatedServer models.Server
	require.NoError(t, db.First(&updatedServer, server.ID).Error)
	assert.Equal(t, "full", updatedServer.AgentType)
	assert.Equal(t, "full", updatedServer.DesiredAgentType)

	second, err := CreateAgentUpgradeJob(db, CreateUpgradeJobInput{
		ServerID:        server.ID,
		Trigger:         "retry",
		TargetVersion:   "1.4.0",
		TargetAgentType: "monitor",
		Channel:         "stable",
		AssetName:       "better-monitor-agent-monitor-1.4.0-linux-amd64",
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example.test/agent",
		SHA256:          "1111111111111111111111111111111111111111111111111111111111111111",
		Now:             now.Add(2 * time.Second),
	})
	require.NoError(t, err)
	assert.NotEqual(t, job.ID, second.ID)
}

func TestReconcileTimedOutUpgradesOnlyExpiresPastDeadline(t *testing.T) {
	db := setupUpgradeStateTestDB(t)
	now := time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)
	expiredServer := createUpgradeStateServer(t, db, "1.3.0", "full")
	expired := createUpgradeStateJob(t, db, expiredServer, now.Add(-DefaultAgentUpgradeTimeout-time.Second), "monitor")

	freshServer := createUpgradeStateServer(t, db, "1.3.0", "full")
	fresh := createUpgradeStateJob(t, db, freshServer, now, "monitor")

	count, err := ReconcileTimedOutUpgrades(db, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assertUpgradeJobStatus(t, db, expired.ID, models.UpgradeTimedOut, 0)
	assertUpgradeJobStatus(t, db, fresh.ID, models.UpgradeQueued, 0)

	var expiredServerAfter models.Server
	require.NoError(t, db.First(&expiredServerAfter, expiredServer.ID).Error)
	assert.Equal(t, "full", expiredServerAfter.DesiredAgentType)
}

func setupUpgradeStateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_busy_timeout=5000", stringsForSQLiteName(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.AgentUpgradeJob{}, &models.SystemSettings{}))
	return db
}

func createUpgradeStateServer(t *testing.T, db *gorm.DB, version, agentType string) models.Server {
	t.Helper()
	server := models.Server{
		Name:             "upgrade-state-server",
		AgentVersion:     version,
		AgentType:        agentType,
		DesiredAgentType: agentType,
	}
	require.NoError(t, db.Create(&server).Error)
	return server
}

func createUpgradeStateJob(
	t *testing.T,
	db *gorm.DB,
	server models.Server,
	now time.Time,
	targetType string,
) *models.AgentUpgradeJob {
	t.Helper()
	job, err := CreateAgentUpgradeJob(db, CreateUpgradeJobInput{
		ServerID:        server.ID,
		Trigger:         "manual",
		TargetVersion:   "1.4.0",
		TargetAgentType: targetType,
		Channel:         "stable",
		AssetName:       "better-monitor-agent-monitor-1.4.0-linux-amd64",
		AssetSize:       1024,
		DownloadURL:     "https://downloads.example.test/agent",
		SHA256:          "1111111111111111111111111111111111111111111111111111111111111111",
		Now:             now,
	})
	require.NoError(t, err)
	return job
}

func assertUpgradeJobStatus(
	t *testing.T,
	db *gorm.DB,
	jobID string,
	status models.AgentUpgradeStatus,
	bytesDownloaded int64,
) {
	t.Helper()
	var job models.AgentUpgradeJob
	require.NoError(t, db.First(&job, "id = ?", jobID).Error)
	assert.Equal(t, status, job.Status)
	assert.Equal(t, bytesDownloaded, job.BytesDownloaded)
}

func stringsForSQLiteName(value string) string {
	output := make([]rune, 0, len(value))
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			output = append(output, char)
		} else {
			output = append(output, '_')
		}
	}
	return string(output)
}
