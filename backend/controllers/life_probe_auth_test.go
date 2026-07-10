package controllers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
)

func setupLifeProbeAuthTest(t *testing.T) (*gorm.DB, *services.LifeIngestSecurity, time.Time) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB := models.DB
	previousProvider := getLifeIngestSecurity
	t.Cleanup(func() {
		models.DB = previousDB
		getLifeIngestSecurity = previousProvider
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.LifeProbe{},
		&models.LifeLoggerEvent{},
		&models.LifeHeartRate{},
		&models.LifeStepSample{},
		&models.LifeStepDailyTotal{},
		&models.LifeSleepSegment{},
	))
	models.DB = db

	now := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	security, err := services.NewLifeIngestSecurity(
		services.LifeIngestPolicy{
			MaxBodyBytes:           1024,
			ClockSkew:              5 * time.Minute,
			IPRequestsPerMinute:    100,
			ProbeRequestsPerMinute: 100,
		},
		bytes.Repeat([]byte{'m'}, 32),
		func() time.Time { return now },
	)
	require.NoError(t, err)
	getLifeIngestSecurity = func() (*services.LifeIngestSecurity, error) { return security, nil }
	return db, security, now
}

func signLifeControllerRequest(secret []byte, req *http.Request, body []byte, now time.Time, nonce string) {
	timestamp := fmt.Sprint(now.Unix())
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		strings.ToUpper(req.Method),
		req.URL.EscapedPath(),
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("X-Life-Timestamp", timestamp)
	req.Header.Set("X-Life-Nonce", nonce)
	req.Header.Set("X-Life-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func performLifeIngest(t *testing.T, body []byte, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
	req.RemoteAddr = "192.0.2.10:1234"
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	IngestLifeLoggerEvent(c)
	return w
}

func TestLifeLoggerAuthRejectsUnsignedPing(t *testing.T) {
	db, security, _ := setupLifeProbeAuthTest(t)
	_, encrypted, err := security.GenerateProbeSecret()
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.LifeProbe{
		Name: "phone", DeviceID: "phone-1", IngestSecretCiphertext: encrypted, IngestSecretVersion: 1,
	}).Error)
	body := []byte(`{"ping":"test","device_id":"phone-1"}`)

	w := performLifeIngest(t, body, nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "life_auth_required")
}

func TestLifeLoggerAuthAcceptsSignedPingAndRejectsReplay(t *testing.T) {
	db, security, now := setupLifeProbeAuthTest(t)
	plain, encrypted, err := security.GenerateProbeSecret()
	require.NoError(t, err)
	secret, err := services.DecodeProbeSecret(plain)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.LifeProbe{
		Name: "phone", DeviceID: "phone-1", IngestSecretCiphertext: encrypted, IngestSecretVersion: 1,
	}).Error)
	body := []byte(`{"ping":"test","device_id":"phone-1"}`)

	first := performLifeIngest(t, body, func(req *http.Request) {
		signLifeControllerRequest(secret, req, body, now, "nonce_ping_12345")
	})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), "pong")

	replay := performLifeIngest(t, body, func(req *http.Request) {
		signLifeControllerRequest(secret, req, body, now, "nonce_ping_12345")
	})
	assert.Equal(t, http.StatusConflict, replay.Code)
	assert.Contains(t, replay.Body.String(), "life_replay_detected")
}

func TestLifeLoggerAuthPersistsSignedEventOnce(t *testing.T) {
	db, security, now := setupLifeProbeAuthTest(t)
	plain, encrypted, err := security.GenerateProbeSecret()
	require.NoError(t, err)
	secret, err := services.DecodeProbeSecret(plain)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.LifeProbe{
		Name: "phone", DeviceID: "phone-1", IngestSecretCiphertext: encrypted, IngestSecretVersion: 1,
	}).Error)
	body := []byte(`{
		"event_id":"event-1",
		"timestamp":"2026-07-10T09:00:00Z",
		"device_id":"phone-1",
		"data_type":"heart_rate",
		"payload":{"value":72,"unit":"bpm","measure_time":"2026-07-10T09:00:00Z"}
	}`)

	w := performLifeIngest(t, body, func(req *http.Request) {
		signLifeControllerRequest(secret, req, body, now, "nonce_event_1234")
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var eventCount int64
	require.NoError(t, db.Model(&models.LifeLoggerEvent{}).Count(&eventCount).Error)
	assert.Equal(t, int64(1), eventCount)
	var heartRateCount int64
	require.NoError(t, db.Model(&models.LifeHeartRate{}).Count(&heartRateCount).Error)
	assert.Equal(t, int64(1), heartRateCount)
}

func TestLifeLoggerAuthRejectsOversizedBody(t *testing.T) {
	_, _, _ = setupLifeProbeAuthTest(t)
	body := bytes.Repeat([]byte{'x'}, 1025)

	w := performLifeIngest(t, body, nil)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "life_body_too_large")
}

func TestCreateAndRotateLifeProbeSecretReturnPlaintextOnce(t *testing.T) {
	db, _, _ := setupLifeProbeAuthTest(t)
	body := []byte(`{"name":"phone","device_id":"phone-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/life-probes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	CreateLifeProbe(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		LifeProbe    models.LifeProbe `json:"life_probe"`
		IngestSecret string           `json:"ingest_secret"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotEmpty(t, created.IngestSecret)
	assert.Equal(t, uint64(1), created.LifeProbe.IngestSecretVersion)
	assert.NotContains(t, w.Body.String(), "ingest_secret_ciphertext")

	var stored models.LifeProbe
	require.NoError(t, db.First(&stored, created.LifeProbe.ID).Error)
	assert.NotEmpty(t, stored.IngestSecretCiphertext)
	firstCiphertext := stored.IngestSecretCiphertext

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/life-probes/%d", stored.ID), nil)
	getWriter := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getWriter)
	getContext.Request = getReq
	getContext.Params = []gin.Param{{Key: "id", Value: fmt.Sprint(stored.ID)}}
	GetLifeProbe(getContext)
	require.Equal(t, http.StatusOK, getWriter.Code)
	assert.NotContains(t, getWriter.Body.String(), created.IngestSecret)
	assert.NotContains(t, getWriter.Body.String(), firstCiphertext)

	rotateReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/life-probes/%d/rotate-secret", stored.ID), nil)
	rotateWriter := httptest.NewRecorder()
	rotateContext, _ := gin.CreateTestContext(rotateWriter)
	rotateContext.Request = rotateReq
	rotateContext.Params = []gin.Param{{Key: "id", Value: fmt.Sprint(stored.ID)}}
	RotateLifeProbeSecret(rotateContext)
	require.Equal(t, http.StatusOK, rotateWriter.Code, rotateWriter.Body.String())
	var rotated struct {
		IngestSecret        string `json:"ingest_secret"`
		IngestSecretVersion uint64 `json:"ingest_secret_version"`
	}
	require.NoError(t, json.Unmarshal(rotateWriter.Body.Bytes(), &rotated))
	assert.NotEmpty(t, rotated.IngestSecret)
	assert.NotEqual(t, created.IngestSecret, rotated.IngestSecret)
	assert.Equal(t, uint64(2), rotated.IngestSecretVersion)
}
