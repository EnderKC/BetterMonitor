package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
)

type websocketAuthFixture struct {
	router       *gin.Engine
	server       *models.Server
	public       *models.Server
	admin        *models.AdminAccount
	httpServer   *httptest.Server
	websocketURL string
}

func setupWebSocketAuthFixture(t *testing.T) *websocketAuthFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB := models.DB
	previousStore := browserWSTicketStore
	t.Cleanup(func() {
		models.DB = previousDB
		browserWSTicketStore = previousStore
		ActiveAgentConnections.ClearForTest()
		ActiveTerminalConnections.ClearForTest()
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&models.AdminAccount{},
		&models.Server{},
		&models.ServerMonitor{},
		&models.AgentUpgradeJob{},
	))
	models.DB = db

	admin := &models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       "admin",
		Password:       "hash",
		SessionVersion: 3,
	}
	require.NoError(t, db.Create(admin).Error)

	privateServer := &models.Server{
		Name:            "private-server",
		IP:              "10.0.0.10",
		SecretKey:       "server-secret",
		Status:          "online",
		Online:          true,
		LastHeartbeat:   time.Now(),
		AllowPublicView: false,
	}
	publicServer := &models.Server{
		Name:            "public-server",
		IP:              "192.0.2.10",
		SecretKey:       "public-secret",
		Status:          "online",
		Online:          true,
		LastHeartbeat:   time.Now(),
		AllowPublicView: true,
	}
	require.NoError(t, db.Create(privateServer).Error)
	require.NoError(t, db.Create(publicServer).Error)

	browserWSTicketStore = services.NewWSTicketStore(services.DefaultWSTicketTTL, time.Now)
	ActiveAgentConnections.ClearForTest()
	ActiveTerminalConnections.ClearForTest()

	router := gin.New()
	var handlers sync.WaitGroup
	tracked := func(handler gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			handlers.Add(1)
			defer handlers.Done()
			handler(c)
		}
	}
	router.GET("/api/servers/:id/agent-ws", tracked(AgentWebSocketHandler))
	router.GET("/api/servers/:id/ws", tracked(BrowserWebSocketHandler))
	router.GET("/api/servers/:id/monitor-ws", tracked(BrowserWebSocketHandler))
	router.GET("/api/servers/public/ws", tracked(PublicServersWebSocketHandler))
	router.GET("/api/servers/:id/status", tracked(GetServerStatus))
	router.GET("/api/servers/versions", tracked(GetServerVersions))

	httpServer := httptest.NewServer(router)
	t.Cleanup(func() {
		httpServer.Close()
		handlers.Wait()
	})

	return &websocketAuthFixture{
		router:       router,
		server:       privateServer,
		public:       publicServer,
		admin:        admin,
		httpServer:   httpServer,
		websocketURL: "ws" + strings.TrimPrefix(httpServer.URL, "http"),
	}
}

func agentHeaders(secret string) http.Header {
	headers := make(http.Header)
	headers.Set("X-Secret-Key", secret)
	headers.Set("X-Agent-Version", "1.2.3")
	headers.Set("X-Agent-Type", "full")
	headers.Set("X-Agent-Heartbeat-Seconds", "10")
	return headers
}

func closeRejectedResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func TestLegacyWebSocketAuthRejectsAgentQuerySecret(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws?token=%s", fixture.websocketURL, fixture.server.ID, fixture.server.SecretKey)

	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAgentWebSocketAuthRejectsMissingAndWrongHeader(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)

	for _, headers := range []http.Header{nil, agentHeaders("wrong-secret")} {
		conn, resp, err := websocket.DefaultDialer.Dial(url, headers)
		if conn != nil {
			_ = conn.Close()
		}
		closeRejectedResponse(resp)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestAgentWebSocketAuthRegistersOnlyAfterMatchingHello(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)

	conn, resp, err := websocket.DefaultDialer.Dial(url, agentHeaders(fixture.server.SecretKey))
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	t.Cleanup(func() { _ = conn.Close() })

	_, ok := ActiveAgentConnections.Current(fixture.server.ID)
	assert.False(t, ok)
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
}

func TestAgentHelloAndHeartbeatPersistUnifiedLivenessMetadata(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)

	conn, _, err := websocket.DefaultDialer.Dial(url, agentHeaders(fixture.server.SecretKey))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":                       "agent_hello",
		"version":                    "1.2.3",
		"agent_type":                 "full",
		"heartbeat_interval_seconds": 10,
	}))

	var afterHello models.Server
	require.Eventually(t, func() bool {
		if err := models.DB.First(&afterHello, fixture.server.ID).Error; err != nil {
			return false
		}
		return afterHello.AgentVersion == "1.2.3" &&
			afterHello.AgentType == "full" &&
			afterHello.AgentHeartbeatSeconds == 10 &&
			afterHello.Online && afterHello.Status == "online" &&
			!afterHello.LastHeartbeat.IsZero()
	}, time.Second, 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":      "agent_heartbeat",
		"timestamp": time.Now().Unix(),
	}))

	require.Eventually(t, func() bool {
		var refreshed models.Server
		if err := models.DB.First(&refreshed, fixture.server.ID).Error; err != nil {
			return false
		}
		return refreshed.LastHeartbeat.After(afterHello.LastHeartbeat)
	}, time.Second, 10*time.Millisecond)
}

func TestAgentWebSocketAuthClosesMetadataMismatch(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/agent-ws", fixture.websocketURL, fixture.server.ID)

	conn, _, err := websocket.DefaultDialer.Dial(url, agentHeaders(fixture.server.SecretKey))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":                       "agent_hello",
		"version":                    "different-version",
		"agent_type":                 "full",
		"heartbeat_interval_seconds": 10,
	}))

	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, websocket.ClosePolicyViolation, closeErr.Code)
	_, ok := ActiveAgentConnections.Current(fixture.server.ID)
	assert.False(t, ok)
}

func TestLegacyWebSocketAuthRejectsBrowserJWTQuery(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	url := fmt.Sprintf("%s/api/servers/%d/ws?token=legacy-jwt", fixture.websocketURL, fixture.server.ID)

	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBrowserWebSocketTicketRequiresValidSingleUseScope(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	baseURL := fmt.Sprintf("%s/api/servers/%d/ws", fixture.websocketURL, fixture.server.ID)

	conn, resp, err := websocket.DefaultDialer.Dial(baseURL, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	wrongScopeTicket, _, err := browserWSTicketStore.Issue(services.WSTicketClaims{
		AdminID:        fixture.admin.ID,
		SessionVersion: fixture.admin.SessionVersion,
		Purpose:        services.WSTicketMonitor,
		ServerID:       fixture.server.ID,
	})
	require.NoError(t, err)
	conn, resp, err = websocket.DefaultDialer.Dial(baseURL+"?ticket="+wrongScopeTicket, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	ticket, _, err := browserWSTicketStore.Issue(services.WSTicketClaims{
		AdminID:        fixture.admin.ID,
		SessionVersion: fixture.admin.SessionVersion,
		Purpose:        services.WSTicketServer,
		ServerID:       fixture.server.ID,
	})
	require.NoError(t, err)
	conn, resp, err = websocket.DefaultDialer.Dial(baseURL+"?ticket="+ticket, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	var welcome map[string]interface{}
	require.NoError(t, conn.ReadJSON(&welcome))
	assert.Equal(t, "welcome", welcome["type"])
	_ = conn.Close()

	conn, resp, err = websocket.DefaultDialer.Dial(baseURL+"?ticket="+ticket, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBrowserWebSocketTicketRejectsInvalidatedSession(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	ticket, _, err := browserWSTicketStore.Issue(services.WSTicketClaims{
		AdminID:        fixture.admin.ID,
		SessionVersion: fixture.admin.SessionVersion,
		Purpose:        services.WSTicketServer,
		ServerID:       fixture.server.ID,
	})
	require.NoError(t, err)
	require.NoError(t, models.DB.Model(&models.AdminAccount{}).
		Where("id = ?", fixture.admin.ID).
		Update("session_version", fixture.admin.SessionVersion+1).Error)

	url := fmt.Sprintf("%s/api/servers/%d/ws?ticket=%s", fixture.websocketURL, fixture.server.ID, ticket)
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if conn != nil {
		_ = conn.Close()
	}
	closeRejectedResponse(resp)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBrowserWebSocketAnonymousPublicListExcludesPrivateServers(t *testing.T) {
	fixture := setupWebSocketAuthFixture(t)
	conn, resp, err := websocket.DefaultDialer.Dial(fixture.websocketURL+"/api/servers/public/ws", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	t.Cleanup(func() { _ = conn.Close() })

	var payload struct {
		Type    string `json:"type"`
		Servers []struct {
			ID uint `json:"id"`
		} `json:"servers"`
	}
	require.NoError(t, conn.ReadJSON(&payload))
	assert.Equal(t, "server_list", payload.Type)
	require.Len(t, payload.Servers, 1)
	assert.Equal(t, fixture.public.ID, payload.Servers[0].ID)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), fmt.Sprint(fixture.server.ID))
}
