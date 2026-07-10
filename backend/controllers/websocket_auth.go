package controllers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"gorm.io/gorm"
)

const agentHelloDeadline = 5 * time.Second

var errWSTicketSessionInvalid = errors.New("websocket ticket session invalid")

type agentHandshakeMetadata struct {
	Version                  string
	AgentType                string
	HeartbeatIntervalSeconds int
}

type agentHello struct {
	Type                     string                  `json:"type"`
	Version                  string                  `json:"version"`
	AgentType                string                  `json:"agent_type"`
	HeartbeatIntervalSeconds int                     `json:"heartbeat_interval_seconds"`
	UpgradeReport            *AgentUpgradeBootReport `json:"upgrade_report,omitempty"`
}

type AgentUpgradeBootReport struct {
	RequestID string `json:"request_id"`
	Outcome   string `json:"outcome"`
	ErrorCode string `json:"error_code,omitempty"`
}

type agentUpgradeAck struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Confirmed bool   `json:"confirmed"`
}

func AgentWebSocketHandler(c *gin.Context) {
	if rejectLegacyWebSocketAuth(c) {
		return
	}

	server, ok := loadWebSocketServer(c)
	if !ok {
		return
	}
	metadata, ok := authenticateAgentHandshake(c, server)
	if !ok {
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	hello, valid := readAndValidateAgentHello(conn, metadata)
	if !valid {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid agent hello"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}
	ack, err := persistAgentHello(server, hello, time.Now())
	if err != nil {
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "agent handshake persistence failed"),
			time.Now().Add(time.Second),
		)
		_ = conn.Close()
		return
	}
	if ack != nil {
		if err := conn.WriteJSON(ack); err != nil {
			_ = conn.Close()
			return
		}
	}

	heartbeat := time.Duration(hello.HeartbeatIntervalSeconds) * time.Second
	serveServerWebSocket(&SafeConn{Conn: conn}, server, "", true, false, heartbeat)
}

func BrowserWebSocketHandler(c *gin.Context) {
	if rejectLegacyWebSocketAuth(c) {
		return
	}

	serverID, ok := parseWebSocketServerID(c)
	if !ok {
		return
	}

	sessionID := strings.TrimSpace(c.Query("session"))
	isMonitor := strings.HasSuffix(c.Request.URL.Path, "/monitor-ws")
	purpose := services.WSTicketServer
	if isMonitor {
		purpose = services.WSTicketMonitor
	} else if sessionID != "" {
		purpose = services.WSTicketTerminal
	}

	expected := services.WSTicketClaims{
		Purpose:   purpose,
		ServerID:  serverID,
		SessionID: sessionID,
	}
	if _, err := consumeBrowserTicket(c, expected); err != nil {
		writeBrowserTicketError(c, err)
		return
	}
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server_not_found"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	serveServerWebSocket(&SafeConn{Conn: conn}, server, sessionID, false, isMonitor, 0)
}

func consumeBrowserTicket(c *gin.Context, expected services.WSTicketClaims) (*services.WSTicketClaims, error) {
	values, exists := c.Request.URL.Query()["ticket"]
	if !exists || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return nil, services.ErrWSTicketInvalid
	}

	claims, err := browserWSTicketStore.Consume(strings.TrimSpace(values[0]), expected)
	if err != nil {
		return nil, err
	}
	admin, err := models.GetAdminAccount()
	if err != nil || admin.ID != claims.AdminID || admin.SessionVersion != claims.SessionVersion {
		return nil, errWSTicketSessionInvalid
	}
	return &claims, nil
}

func authorizeOptionalBrowserTicket(c *gin.Context, expected services.WSTicketClaims) (bool, bool) {
	if rejectLegacyWebSocketAuth(c) {
		return false, false
	}
	if _, exists := c.Request.URL.Query()["ticket"]; !exists {
		return false, true
	}
	if _, err := consumeBrowserTicket(c, expected); err != nil {
		writeBrowserTicketError(c, err)
		return false, false
	}
	return true, true
}

func authenticateAgentHandshake(c *gin.Context, server *models.Server) (agentHandshakeMetadata, bool) {
	secret := c.GetHeader("X-Secret-Key")
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(server.SecretKey)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent_ws_unauthorized"})
		return agentHandshakeMetadata{}, false
	}

	metadata := agentHandshakeMetadata{
		Version:   strings.TrimSpace(c.GetHeader("X-Agent-Version")),
		AgentType: strings.ToLower(strings.TrimSpace(c.GetHeader("X-Agent-Type"))),
	}
	heartbeat, err := strconv.ParseUint(strings.TrimSpace(c.GetHeader("X-Agent-Heartbeat-Seconds")), 10, 31)
	if err != nil || heartbeat == 0 || metadata.Version == "" ||
		(metadata.AgentType != "full" && metadata.AgentType != "monitor") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_ws_metadata_invalid"})
		return agentHandshakeMetadata{}, false
	}
	metadata.HeartbeatIntervalSeconds = int(heartbeat)
	return metadata, true
}

func readAndValidateAgentHello(conn *websocket.Conn, expected agentHandshakeMetadata) (agentHello, bool) {
	if err := conn.SetReadDeadline(time.Now().Add(agentHelloDeadline)); err != nil {
		return agentHello{}, false
	}
	var hello agentHello
	if err := conn.ReadJSON(&hello); err != nil {
		return agentHello{}, false
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return agentHello{}, false
	}
	valid := hello.Type == "agent_hello" &&
		hello.Version == expected.Version &&
		strings.ToLower(strings.TrimSpace(hello.AgentType)) == expected.AgentType &&
		hello.HeartbeatIntervalSeconds == expected.HeartbeatIntervalSeconds
	return hello, valid
}

func persistAgentHello(server *models.Server, hello agentHello, now time.Time) (*agentUpgradeAck, error) {
	helloType := strings.ToLower(strings.TrimSpace(hello.AgentType))
	var ack *agentUpgradeAck
	syncDesired := false
	err := models.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"agent_version":           hello.Version,
			"agent_type":              helloType,
			"agent_heartbeat_seconds": hello.HeartbeatIntervalSeconds,
			"last_heartbeat":          now,
			"online":                  true,
			"status":                  "online",
		}
		if err := tx.Model(&models.Server{}).Where("id = ?", server.ID).Updates(updates).Error; err != nil {
			return err
		}

		if hello.UpgradeReport != nil {
			result, err := services.ReconcileAgentUpgradeBoot(tx, services.AgentUpgradeBootInput{
				ServerID:  server.ID,
				RequestID: hello.UpgradeReport.RequestID,
				Version:   hello.Version,
				AgentType: helloType,
				Outcome:   hello.UpgradeReport.Outcome,
				ErrorCode: hello.UpgradeReport.ErrorCode,
				At:        now,
			})
			if err != nil {
				return err
			}
			ack = &agentUpgradeAck{
				Type:      "agent_upgrade_ack",
				RequestID: strings.TrimSpace(hello.UpgradeReport.RequestID),
				Confirmed: result.Confirmed,
			}
		}

		var activeJobs int64
		if err := tx.Model(&models.AgentUpgradeJob{}).
			Where("server_id = ? AND active_key IS NOT NULL", server.ID).
			Count(&activeJobs).Error; err != nil {
			return err
		}
		if activeJobs == 0 {
			if err := tx.Model(&models.Server{}).Where("id = ?", server.ID).
				Update("desired_agent_type", helloType).Error; err != nil {
				return err
			}
			syncDesired = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	server.AgentVersion = hello.Version
	server.AgentType = helloType
	if syncDesired {
		server.DesiredAgentType = helloType
	}
	server.AgentHeartbeatSeconds = hello.HeartbeatIntervalSeconds
	server.LastHeartbeat = now
	server.Online = true
	server.Status = "online"
	return ack, nil
}

func loadWebSocketServer(c *gin.Context) (*models.Server, bool) {
	id, ok := parseWebSocketServerID(c)
	if !ok {
		return nil, false
	}
	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server_not_found"})
		return nil, false
	}
	return server, true
}

func parseWebSocketServerID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_server_id"})
		return 0, false
	}
	return uint(id), true
}

func rejectLegacyWebSocketAuth(c *gin.Context) bool {
	for key := range c.Request.URL.Query() {
		switch strings.ToLower(key) {
		case "token", "secret_key", "jwt":
			c.JSON(http.StatusUnauthorized, gin.H{"error": "legacy_ws_auth_rejected"})
			return true
		}
	}
	return false
}

func writeBrowserTicketError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrWSTicketScopeMismatch):
		c.JSON(http.StatusForbidden, gin.H{"error": "ws_ticket_scope_mismatch"})
	case errors.Is(err, errWSTicketSessionInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ws_ticket_session_invalid"})
	default:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ws_ticket_invalid"})
	}
}
