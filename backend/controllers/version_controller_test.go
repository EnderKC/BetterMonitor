package controllers

import (
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

	dsn := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
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
	ActiveAgentConnections.ClearForTest()
}

func TestHealthCheckWithVersion(t *testing.T) {
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
	// 使用真实的版本函数，不需要 mock
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetDashboardVersion(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp version.Info
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Version)
}

func TestGetSystemInfo(t *testing.T) {
	// 使用真实的版本函数，不需要 mock
	req := httptest.NewRequest(http.MethodGet, "/system/info", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	GetSystemInfo(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["version"])
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
		AgentType:     "full",
		LastHeartbeat: now,
	}
	server2 := models.Server{
		Name:          "Server 2",
		IP:            "192.168.1.2",
		Online:        false,
		AgentVersion:  "1.0.1",
		AgentType:     "monitor",
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
	assert.Equal(t, "full", resp[0]["agentType"])
	assert.Equal(t, "monitor", resp[1]["agentType"])
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
		if strings.Contains(r.URL.Path, "/releases") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{
				"tag_name": "v1.2.3",
				"name": "Agent v1.2.3",
				"body": "notes",
				"prerelease": false,
				"published_at": "2024-01-01T00:00:00Z",
				"assets": [{
					"name": "agent-linux-amd64.tar.gz",
					"browser_download_url": "https://github.com/demo/repo/releases/download/v1.2.3/agent-linux-amd64.tar.gz",
					"size": 1234
				}]
			}]`)
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
	assert.Equal(t, "stable", resp["channel"])
	assert.NotContains(t, w.Body.String(), "download_url")
	assert.NotContains(t, w.Body.String(), "browser_download_url")
}

func TestGetLatestAgentReleaseSelectsLatestReleaseWithinConfiguredChannel(t *testing.T) {
	db := setupTestDB(t)
	assert.NoError(t, db.Create(&models.SystemSettings{
		AgentReleaseRepo:    "demo/repo",
		AgentReleaseChannel: "prerelease",
	}).Error)

	services.ClearReleaseCache()
	defer services.ClearReleaseCache()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[
				{
					"tag_name": "v2.0.0-nightly.9",
					"name": "Nightly",
					"prerelease": true,
					"published_at": "2026-07-10T00:00:00Z",
					"assets": []
				},
				{
					"tag_name": "v1.9.0-beta.2",
					"name": "Beta",
					"prerelease": true,
					"published_at": "2026-07-09T00:00:00Z",
					"assets": []
				},
				{
					"tag_name": "v1.8.0",
					"name": "Stable",
					"prerelease": false,
					"published_at": "2026-07-08T00:00:00Z",
					"assets": []
				}
			]`)
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
	assert.Equal(t, "1.9.0-beta.2", resp["version"])
	assert.Equal(t, "prerelease", resp["channel"])
}
