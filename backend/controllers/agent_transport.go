package controllers

import (
	"fmt"

	"github.com/user/server-ops-backend/internal/agenttransport"
)

func failAgentRequestsForDisconnectedServer(serverID uint) {
	agenttransport.FailAgentRequests(serverID, errAgentConnectionClosed)
	// File and terminal request owners still use their dedicated registries.
	failAllPendingRequests(serverID)
}

func sendAgentCommandEnvelope(serverID uint, command agenttransport.AgentCommandEnvelope) error {
	safeConn, ok := ActiveAgentConnections.Current(serverID)
	if !ok || safeConn == nil || safeConn.Conn == nil {
		return fmt.Errorf("服务器(ID: %d)未连接", serverID)
	}
	return safeConn.WriteJSON(command)
}

func init() {
	agenttransport.ConfigureAgentCommandSender(sendAgentCommandEnvelope)
}
