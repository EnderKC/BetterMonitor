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
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"github.com/user/server-ops-backend/utils"
)

func newPrivacyControllerDB(t *testing.T, values ...interface{}) *gorm.DB {
	t.Helper()
	previousDB := models.DB
	t.Cleanup(func() { models.DB = previousDB })

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(values...))
	models.DB = db
	return db
}

func TestCreateServerDefaultsPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPrivacyControllerDB(t, &models.Server{})
	body, err := json.Marshal(map[string]string{"name": "private-by-default"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	CreateServer(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var server models.Server
	require.NoError(t, db.First(&server).Error)
	assert.False(t, server.AllowPublicView)
}

func TestCreateLifeProbeDefaultsPrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPrivacyControllerDB(t, &models.LifeProbe{})
	previousProvider := getLifeIngestSecurity
	security, err := services.NewLifeIngestSecurity(
		services.LifeIngestPolicy{},
		bytes.Repeat([]byte{'p'}, 32),
		time.Now,
	)
	require.NoError(t, err)
	getLifeIngestSecurity = func() (*services.LifeIngestSecurity, error) { return security, nil }
	t.Cleanup(func() { getLifeIngestSecurity = previousProvider })
	body, err := json.Marshal(map[string]string{"name": "private-probe", "device_id": "device-1"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/life-probes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	CreateLifeProbe(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var probe models.LifeProbe
	require.NoError(t, db.First(&probe).Error)
	assert.False(t, probe.AllowPublicView)
}

func TestGetServerStatusAllowsAuthenticatedAdminForPrivateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPrivacyControllerDB(t, &models.AdminAccount{}, &models.Server{})
	hash, err := models.HashPassword("correct-password")
	require.NoError(t, err)
	admin := models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       "admin",
		Password:       hash,
		SessionVersion: 1,
	}
	require.NoError(t, db.Create(&admin).Error)
	server := models.Server{Name: "private", AllowPublicView: false}
	require.NoError(t, db.Create(&server).Error)
	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/servers/%d/status", server.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprint(server.ID)}}

	GetServerStatus(c)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGetServerStatusRejectsAnonymousPrivateServerWithoutPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPrivacyControllerDB(t, &models.Server{})
	server := models.Server{Name: "private", AllowPublicView: false}
	require.NoError(t, db.Create(&server).Error)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/servers/%d/status", server.ID), nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprint(server.ID)}}

	GetServerStatus(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), "last_heartbeat")
	assert.NotContains(t, w.Body.String(), "\"name\"")
}
