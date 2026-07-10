package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

func TestJWTAuthMiddlewareRejectsStaleSessionVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := models.DB
	t.Cleanup(func() { models.DB = previousDB })

	db, err := gorm.Open(sqlite.Open("file:middleware_stale?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AdminAccount{}))
	require.NoError(t, db.Create(&models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       "admin",
		Password:       "hash",
		SessionVersion: 2,
	}).Error)
	models.DB = db

	token, err := utils.GenerateToken(models.SingletonAdminID, "admin", 1)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/protected", JWTAuthMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJWTAuthMiddlewareSetsAdministratorContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := models.DB
	t.Cleanup(func() { models.DB = previousDB })

	db, err := gorm.Open(sqlite.Open("file:middleware_current?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AdminAccount{}))
	admin := models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       "admin",
		Password:       "hash",
		SessionVersion: 3,
	}
	require.NoError(t, db.Create(&admin).Error)
	models.DB = db

	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/protected", JWTAuthMiddleware(), func(c *gin.Context) {
		_, hasRole := c.Get("role")
		_, hasUserID := c.Get("userId")
		storedAdmin, hasAdmin := c.Get("adminAccount")
		assert.False(t, hasRole)
		assert.False(t, hasUserID)
		assert.True(t, hasAdmin)
		assert.Equal(t, admin.ID, storedAdmin.(*models.AdminAccount).ID)
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
