package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/config"
	"github.com/user/server-ops-backend/middleware"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

func setupAdminAuthTest(t *testing.T, username, password string) (*gin.Engine, *models.AdminAccount) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB := models.DB
	t.Cleanup(func() { models.DB = previousDB })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AdminAccount{}))
	hash, err := models.HashPassword(password)
	require.NoError(t, err)
	admin := &models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       username,
		Password:       hash,
		Email:          "admin@example.com",
		Phone:          "10086",
		SessionVersion: 1,
	}
	require.NoError(t, db.Create(admin).Error)
	models.DB = db
	loginAttempts = make(map[string]*loginAttempt)

	router := gin.New()
	router.POST("/login", Login)
	auth := router.Group("/")
	auth.Use(middleware.JWTAuthMiddleware())
	auth.GET("/profile", GetProfile)
	auth.PUT("/profile", UpdateProfile)
	auth.POST("/change-password", ChangePassword)
	return router, admin
}

func performJSONRequest(t *testing.T, router http.Handler, method, path string, payload any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(payload))
	}
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func loginForToken(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	w := performJSONRequest(t, router, http.MethodPost, "/login", map[string]string{
		"username": username,
		"password": password,
	}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Token string                 `json:"token"`
		Admin map[string]interface{} `json:"admin"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, response.Token)
	assert.Equal(t, username, response.Admin["username"])
	assert.NotContains(t, response.Admin, "role")
	return response.Token
}

func TestLoginReturnsSingletonAdminWithoutRole(t *testing.T) {
	router, _ := setupAdminAuthTest(t, "admin", "correct-password")

	token := loginForToken(t, router, "admin", "correct-password")
	assert.NotEmpty(t, token)
}

func TestChangePasswordInvalidatesOldToken(t *testing.T) {
	router, _ := setupAdminAuthTest(t, "admin", "old-password-123")
	oldToken := loginForToken(t, router, "admin", "old-password-123")

	changeResponse := performJSONRequest(t, router, http.MethodPost, "/change-password", map[string]string{
		"old_password": "old-password-123",
		"new_password": "new-password-456",
	}, oldToken)
	require.Equal(t, http.StatusOK, changeResponse.Code, changeResponse.Body.String())

	oldSessionResponse := performJSONRequest(t, router, http.MethodGet, "/profile", nil, oldToken)
	assert.Equal(t, http.StatusUnauthorized, oldSessionResponse.Code)

	oldPasswordResponse := performJSONRequest(t, router, http.MethodPost, "/login", map[string]string{
		"username": "admin",
		"password": "old-password-123",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, oldPasswordResponse.Code)

	newToken := loginForToken(t, router, "admin", "new-password-456")
	assert.NotEqual(t, oldToken, newToken)
}

func TestUpdateProfileUsernameReturnsReplacementToken(t *testing.T) {
	router, _ := setupAdminAuthTest(t, "admin", "correct-password")
	oldToken := loginForToken(t, router, "admin", "correct-password")

	w := performJSONRequest(t, router, http.MethodPut, "/profile", map[string]string{
		"username": "renamed-admin",
		"email":    "renamed@example.com",
		"phone":    "10010",
	}, oldToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Data  map[string]interface{} `json:"data"`
		Token string                 `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "renamed-admin", response.Data["username"])
	assert.NotContains(t, response.Data, "role")
	assert.NotEmpty(t, response.Token)

	oldSessionResponse := performJSONRequest(t, router, http.MethodGet, "/profile", nil, oldToken)
	assert.Equal(t, http.StatusUnauthorized, oldSessionResponse.Code)
	newSessionResponse := performJSONRequest(t, router, http.MethodGet, "/profile", nil, response.Token)
	assert.Equal(t, http.StatusOK, newSessionResponse.Code)
}

func TestUpdateProfileRejectsSessionChangedAfterMiddlewareValidation(t *testing.T) {
	_, admin := setupAdminAuthTest(t, "admin", "correct-password")
	staleAdmin := *admin
	require.NoError(t, models.DB.Model(&models.AdminAccount{}).
		Where("id = ?", admin.ID).
		Update("session_version", admin.SessionVersion+1).Error)

	body, err := json.Marshal(map[string]string{"username": "should-not-apply"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("adminAccount", &staleAdmin)

	UpdateProfile(c)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "\"token\"")
	var persisted models.AdminAccount
	require.NoError(t, models.DB.First(&persisted, models.SingletonAdminID).Error)
	assert.Equal(t, "admin", persisted.Username)
}

func TestDownloadQueryTokenRejectsStaleSession(t *testing.T) {
	_, admin := setupAdminAuthTest(t, "admin", "correct-password")
	staleToken, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)
	require.NoError(t, models.DB.Model(&models.AdminAccount{}).
		Where("id = ?", admin.ID).
		Update("session_version", admin.SessionVersion+1).Error)

	assert.Error(t, validateAdminDownloadToken(staleToken))
}

func TestLoginRateLimitIgnoresSpoofedForwardedForByDefault(t *testing.T) {
	router, _ := setupAdminAuthTest(t, "admin", "correct-password")
	require.NoError(t, config.ConfigureTrustedProxies(router, nil))

	for attempt := 0; attempt < loginMaxFailures; attempt++ {
		body, err := json.Marshal(map[string]string{
			"username": "admin",
			"password": "wrong-password",
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", attempt+1))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}

	body, err := json.Marshal(map[string]string{
		"username": "admin",
		"password": "correct-password",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.200")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
