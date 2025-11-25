package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/pkg/version"
	"github.com/user/server-ops-backend/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	if err := db.AutoMigrate(&models.Server{}, &models.SystemSettings{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}

	models.DB = db
	return db
}

func clearActiveConnections() {
	ActiveAgentConnections.Range(func(key, value interface{}) bool {
		ActiveAgentConnections.Delete(key)
		return true
	})
}

func TestHealthCheck(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	HealthCheck(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "healthy", resp["status"])
	assert.NotEmpty(t, resp["uptime"])
}

func TestGetDashboardVersion(t *testing.T) {
	version.GetVersion = func() version.VersionInfo {
		return version.VersionInfo{
			Version:   "1.0.0",
			BuildDate: "2024-01-01",
			GoVersion: "go1.22",
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetDashboardVersion(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp version.VersionInfo
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "1.0.0", resp.Version)
}

func TestGetSystemInfo(t *testing.T) {
	version.GetVersion = func() version.VersionInfo {
		return version.VersionInfo{
			Version:   "1.0.0",
			BuildDate: "2024-01-01",
			GoVersion: "go1.22",
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/system/info", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetSystemInfo(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "1.0.0", resp["version"])
	assert.NotEmpty(t, resp["memoryTotal"])
}

func TestGetServerVersions(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	server1 := models.Server{
		Name:          "Server 1",
		IP:            "192.168.1.1",
		Online:        true,
		AgentVersion:  "1.0.0",
		LastHeartbeat: now,
	}
	server2 := models.Server{
		Name:          "Server 2",
		IP:            "192.168.1.2",
		Online:        false,
		AgentVersion:  "1.0.1",
		LastHeartbeat: now.Add(-5 * time.Minute),
	}
	assert.NoError(t, db.Create(&server1).Error)
	assert.NoError(t, db.Create(&server2).Error)

	req := httptest.NewRequest(http.MethodGet, "/servers/versions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetServerVersions(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp []map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
	assert.Equal(t, "Server 1", resp[0]["name"])
	assert.Equal(t, float64(1), resp[0]["status"])
}

func TestGetLatestAgentRelease(t *testing.T) {
	db := setupTestDB(t)
	assert.NoError(t, db.Create(&models.SystemSettings{
		AgentReleaseRepo:    "demo/repo",
		AgentReleaseChannel: "stable",
	}).Error)

	services.ClearReleaseCache()
	defer services.ClearReleaseCache()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"tag_name": "v1.2.3",
				"name": "Agent v1.2.3",
				"body": "notes",
				"published_at": "2024-01-01T00:00:00Z",
				"assets": [{
					"name": "agent-linux-amd64.tar.gz",
					"browser_download_url": "https://github.com/demo/repo/releases/download/v1.2.3/agent-linux-amd64.tar.gz",
					"size": 1234
				}]
			}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	services.SetReleaseAPIBaseURL(ts.URL)
	defer services.ResetReleaseAPIBaseURL()
	services.SetReleaseHTTPClient(ts.Client())
	defer services.ResetReleaseHTTPClient()

	req := httptest.NewRequest(http.MethodGet, "/agents/releases/latest", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetLatestAgentRelease(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
	assert.Equal(t, "1.2.3", resp["version"])
	assets := resp["assets"].([]interface{})
	assert.Len(t, assets, 1)
}

func TestForceAgentUpgrade(t *testing.T) {
	db := setupTestDB(t)
	assert.NoError(t, db.Create(&models.SystemSettings{
		AgentReleaseRepo:    "demo/repo",
		AgentReleaseChannel: "stable",
	}).Error)

	serverOnline := models.Server{
		Name:   "Online",
		IP:     "10.0.0.1",
		Online: true,
	}
	serverOffline := models.Server{
		Name:   "Offline",
		IP:     "10.0.0.2",
		Online: false,
	}
	serverSendError := models.Server{
		Name:   "SendError",
		IP:     "10.0.0.3",
		Online: true,
	}
	assert.NoError(t, db.Create(&serverOnline).Error)
	assert.NoError(t, db.Create(&serverOffline).Error)
	assert.NoError(t, db.Create(&serverSendError).Error)

	clearActiveConnections()
	defer clearActiveConnections()

	ActiveAgentConnections.Store(serverOnline.ID, &SafeConn{})
	ActiveAgentConnections.Store(serverSendError.ID, &SafeConn{})

	sentCommands := make([]map[string]interface{}, 0)
	origSender := agentUpgradeSender
	defer func() { agentUpgradeSender = origSender }()
	agentUpgradeSender = func(conn *SafeConn, payload map[string]interface{}) error {
		sentCommands = append(sentCommands, payload)
		if inner, ok := payload["payload"].(map[string]interface{}); ok {
			switch id := inner["server_id"].(type) {
			case uint:
				if id == serverSendError.ID {
					return fmt.Errorf("send error")
				}
			case uint64:
				if uint(id) == serverSendError.ID {
					return fmt.Errorf("send error")
				}
			case float64:
				if uint(id) == serverSendError.ID {
					return fmt.Errorf("send error")
				}
			}
		}
		return nil
	}

	body := map[string]interface{}{
		"serverIds":     []uint64{uint64(serverOnline.ID), uint64(serverOffline.ID), uint64(serverSendError.ID), 9999},
		"targetVersion": "2.0.0",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/servers/upgrade", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	ForceAgentUpgrade(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])

	result := resp["result"].(map[string]interface{})
	assert.Contains(t, result["success"], float64(serverOnline.ID))
	assert.Contains(t, result["offline"], float64(serverOffline.ID))
	assert.Contains(t, result["failure"], float64(serverSendError.ID))
	assert.Contains(t, result["missing"], float64(9999))
	assert.Len(t, sentCommands, 2)
}
