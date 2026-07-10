package utils

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
)

func TestGenerateTokenUsesAdminClaimsWithoutRole(t *testing.T) {
	tokenString, err := GenerateToken(1, "admin", 7)
	require.NoError(t, err)

	claims, err := ParseToken(tokenString)
	require.NoError(t, err)
	assert.Equal(t, uint(1), claims.AdminID)
	assert.Equal(t, "admin", claims.Username)
	assert.Equal(t, uint64(7), claims.SessionVersion)

	parts := strings.Split(tokenString, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(payload, &raw))
	assert.NotContains(t, raw, "role")
	assert.NotContains(t, raw, "user_id")
	assert.Equal(t, float64(1), raw["admin_id"])
	assert.Equal(t, float64(7), raw["session_version"])
}

func TestParseTokenRejectsUnexpectedSigningMethod(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{"admin_id": 1})
	tokenString, err := token.SignedString([]byte("wrong-secret"))
	require.NoError(t, err)

	_, err = ParseToken(tokenString)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "signing")
}

func TestValidateAdminTokenRejectsStaleSessionVersion(t *testing.T) {
	previousDB := models.DB
	t.Cleanup(func() { models.DB = previousDB })

	db, err := gorm.Open(sqlite.Open("file:jwt_stale?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AdminAccount{}))
	require.NoError(t, db.Create(&models.AdminAccount{
		ID:             models.SingletonAdminID,
		Username:       "admin",
		Password:       "hash",
		SessionVersion: 2,
	}).Error)
	models.DB = db

	tokenString, err := GenerateToken(models.SingletonAdminID, "admin", 1)
	require.NoError(t, err)

	_, _, err = ValidateAdminToken(tokenString)
	require.ErrorIs(t, err, ErrInvalidAdminSession)
}
