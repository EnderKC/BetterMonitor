package config

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorsMiddlewareDefaultDoesNotAllowCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	instance = &Config{}

	router := gin.New()
	router.Use(CorsMiddleware())
	router.OPTIONS("/api/test", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestConfigureTrustedProxiesDefaultsToTrustNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router, nil))
	router.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "192.0.2.10", w.Body.String())
}

func TestConfigureTrustedProxiesUsesExplicitForwardedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router, []string{"192.0.2.10"}))
	router.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "198.51.100.20", w.Body.String())
}

func TestConfigureTrustedProxiesRejectsInvalidValue(t *testing.T) {
	router := gin.New()
	err := ConfigureTrustedProxies(router, []string{"not-a-proxy"})
	require.Error(t, err)
}
