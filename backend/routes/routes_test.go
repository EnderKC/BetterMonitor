package routes

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAgentUpgradeRoutesUseOnlyJobAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SetupRoutes(router)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, expected := range []string{
		"POST /api/agent-upgrades",
		"GET /api/agent-upgrades",
		"GET /api/agent-upgrades/:id",
	} {
		_, ok := routes[expected]
		assert.True(t, ok, "missing route %s", expected)
	}
	for _, legacy := range []string{
		"POST /api/servers/upgrade",
		"POST /api/servers/:id/switch-agent-type",
	} {
		_, ok := routes[legacy]
		assert.False(t, ok, "legacy route remains: %s", legacy)
	}
}
