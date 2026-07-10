package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-backend/middleware"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"github.com/user/server-ops-backend/utils"
)

func setupWSTicketControllerTest(t *testing.T) (http.Handler, *models.AdminAccount, time.Time) {
	t.Helper()
	router, admin := setupAdminAuthTest(t, "admin", "correct-password")
	now := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	previousStore := browserWSTicketStore
	t.Cleanup(func() { browserWSTicketStore = previousStore })
	browserWSTicketStore = services.NewWSTicketStore(services.DefaultWSTicketTTL, func() time.Time { return now })
	router.POST("/ws-tickets", middleware.JWTAuthMiddleware(), IssueWSTicket)
	return router, admin, now
}

func TestIssueWSTicketReturnsScopedTerminalTicket(t *testing.T) {
	router, admin, now := setupWSTicketControllerTest(t)
	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)

	w := performJSONRequest(t, router, http.MethodPost, "/ws-tickets", map[string]interface{}{
		"purpose":    "terminal",
		"server_id":  7,
		"session_id": "session-1",
	}, token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response struct {
		Ticket    string    `json:"ticket"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.NotEmpty(t, response.Ticket)
	assert.Equal(t, now.Add(services.DefaultWSTicketTTL), response.ExpiresAt)

	claims, err := browserWSTicketStore.Consume(response.Ticket, services.WSTicketClaims{
		Purpose:   services.WSTicketTerminal,
		ServerID:  7,
		SessionID: "session-1",
	})
	require.NoError(t, err)
	assert.Equal(t, admin.ID, claims.AdminID)
	assert.Equal(t, admin.SessionVersion, claims.SessionVersion)
}

func TestIssueWSTicketRejectsInvalidPurposeScope(t *testing.T) {
	router, admin, _ := setupWSTicketControllerTest(t)
	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)

	tests := []struct {
		name    string
		payload map[string]interface{}
	}{
		{name: "unknown purpose", payload: map[string]interface{}{"purpose": "unknown"}},
		{name: "terminal missing server", payload: map[string]interface{}{"purpose": "terminal", "session_id": "session-1"}},
		{name: "terminal missing session", payload: map[string]interface{}{"purpose": "terminal", "server_id": 7}},
		{name: "terminal extra life probe", payload: map[string]interface{}{"purpose": "terminal", "server_id": 7, "session_id": "session-1", "life_probe_id": 9}},
		{name: "server extra session", payload: map[string]interface{}{"purpose": "server", "server_id": 7, "session_id": "session-1"}},
		{name: "list extra server", payload: map[string]interface{}{"purpose": "server_list", "server_id": 7}},
		{name: "life detail missing probe", payload: map[string]interface{}{"purpose": "life_probe_detail"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performJSONRequest(t, router, http.MethodPost, "/ws-tickets", tt.payload, token)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestIssueWSTicketRejectsStaleJWT(t *testing.T) {
	router, admin, _ := setupWSTicketControllerTest(t)
	staleToken, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)
	require.NoError(t, models.DB.Model(&models.AdminAccount{}).
		Where("id = ?", admin.ID).
		Update("session_version", admin.SessionVersion+1).Error)

	w := performJSONRequest(t, router, http.MethodPost, "/ws-tickets", map[string]interface{}{
		"purpose":   "server",
		"server_id": 7,
	}, staleToken)

	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}
