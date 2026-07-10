package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
)

var browserWSTicketStore = services.NewWSTicketStore(services.DefaultWSTicketTTL, nil)

type issueWSTicketRequest struct {
	Purpose     services.WSTicketPurpose `json:"purpose"`
	ServerID    uint                     `json:"server_id"`
	LifeProbeID uint                     `json:"life_probe_id"`
	SessionID   string                   `json:"session_id"`
}

func IssueWSTicket(c *gin.Context) {
	admin, ok := currentAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_administrator_session"})
		return
	}

	currentAdmin, err := models.GetAdminAccount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "administrator_unavailable"})
		return
	}
	if currentAdmin.ID != admin.ID ||
		currentAdmin.Username != admin.Username ||
		currentAdmin.SessionVersion != admin.SessionVersion {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_administrator_session"})
		return
	}

	var req issueWSTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ws_ticket_request"})
		return
	}

	claims, ok := buildWSTicketClaims(currentAdmin, req)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_ws_ticket_scope"})
		return
	}

	ticket, expiresAt, err := browserWSTicketStore.Issue(claims)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ws_ticket_issue_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ticket":     ticket,
		"expires_at": expiresAt,
	})
}

func buildWSTicketClaims(admin *models.AdminAccount, req issueWSTicketRequest) (services.WSTicketClaims, bool) {
	req.Purpose = services.WSTicketPurpose(strings.TrimSpace(string(req.Purpose)))
	req.SessionID = strings.TrimSpace(req.SessionID)

	validScope := false
	switch req.Purpose {
	case services.WSTicketServer, services.WSTicketMonitor:
		validScope = req.ServerID != 0 && req.LifeProbeID == 0 && req.SessionID == ""
	case services.WSTicketTerminal:
		validScope = req.ServerID != 0 && req.LifeProbeID == 0 && req.SessionID != ""
	case services.WSTicketServerList, services.WSTicketLifeProbeList:
		validScope = req.ServerID == 0 && req.LifeProbeID == 0 && req.SessionID == ""
	case services.WSTicketLifeProbeDetail:
		validScope = req.ServerID == 0 && req.LifeProbeID != 0 && req.SessionID == ""
	}
	if !validScope || admin == nil || admin.ID != models.SingletonAdminID || admin.SessionVersion == 0 {
		return services.WSTicketClaims{}, false
	}

	return services.WSTicketClaims{
		AdminID:        admin.ID,
		SessionVersion: admin.SessionVersion,
		Purpose:        req.Purpose,
		ServerID:       req.ServerID,
		LifeProbeID:    req.LifeProbeID,
		SessionID:      req.SessionID,
	}, true
}
