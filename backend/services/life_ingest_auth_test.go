package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lifeTestPolicy() LifeIngestPolicy {
	return LifeIngestPolicy{
		MaxBodyBytes:           128,
		ClockSkew:              5 * time.Minute,
		IPRequestsPerMinute:    2,
		ProbeRequestsPerMinute: 2,
	}
}

func newLifeTestAuthenticator(now *time.Time) (*LifeIngestAuthenticator, []byte, string) {
	masterKey := bytes.Repeat([]byte{'m'}, 32)
	probeSecret := bytes.Repeat([]byte{'s'}, 32)
	encryptedSecret, err := EncryptLifeIngestSecret(masterKey, probeSecret)
	if err != nil {
		panic(err)
	}
	authenticator := NewLifeIngestAuthenticator(lifeTestPolicy(), masterKey, func() time.Time { return *now })
	return authenticator, probeSecret, encryptedSecret
}

func newSignedLifeRequest(secret, body []byte, now time.Time, nonce string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
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
	return req
}

func requireLifeError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var ingestErr *LifeIngestError
	require.ErrorAs(t, err, &ingestErr)
	assert.Equal(t, status, ingestErr.Status)
	assert.Equal(t, code, ingestErr.Code)
}

func TestEncryptLifeIngestSecretRoundTrip(t *testing.T) {
	masterKey := bytes.Repeat([]byte{'m'}, 32)
	secret := bytes.Repeat([]byte{'s'}, 32)

	encoded, err := EncryptLifeIngestSecret(masterKey, secret)
	require.NoError(t, err)
	assert.NotContains(t, encoded, string(secret))

	decoded, err := DecryptLifeIngestSecret(masterKey, encoded)
	require.NoError(t, err)
	assert.Equal(t, secret, decoded)

	_, err = DecryptLifeIngestSecret(bytes.Repeat([]byte{'x'}, 32), encoded)
	require.Error(t, err)
}

func TestLifeIngestValidRequest(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, secret, encrypted := newLifeTestAuthenticator(&now)
	body := []byte(`{"device_id":"phone-1","ping":"test"}`)
	req := newSignedLifeRequest(secret, body, now, "nonce_1234567890")
	w := httptest.NewRecorder()

	readBody, err := authenticator.ReadBounded(w, req, "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, body, readBody)
	require.NoError(t, authenticator.Verify(req, readBody, 1, encrypted))
}

func TestLifeIngestRejectsMissingHeadersAndBadSignature(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, secret, encrypted := newLifeTestAuthenticator(&now)
	body := []byte(`{"device_id":"phone-1"}`)

	missing := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
	err := authenticator.Verify(missing, body, 1, encrypted)
	requireLifeError(t, err, http.StatusUnauthorized, "life_auth_required")

	badSignature := newSignedLifeRequest(secret, body, now, "nonce_1234567890")
	badSignature.Header.Set("X-Life-Signature", strings.Repeat("0", 64))
	err = authenticator.Verify(badSignature, body, 1, encrypted)
	requireLifeError(t, err, http.StatusUnauthorized, "life_signature_invalid")
}

func TestLifeIngestRejectsTimestampAndNonceViolations(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, secret, encrypted := newLifeTestAuthenticator(&now)
	body := []byte(`{"device_id":"phone-1"}`)

	for _, timestamp := range []time.Time{now.Add(-6 * time.Minute), now.Add(6 * time.Minute)} {
		req := newSignedLifeRequest(secret, body, timestamp, "nonce_1234567890")
		err := authenticator.Verify(req, body, 1, encrypted)
		requireLifeError(t, err, http.StatusUnauthorized, "life_timestamp_invalid")
	}

	malformed := newSignedLifeRequest(secret, body, now, "short")
	err := authenticator.Verify(malformed, body, 1, encrypted)
	requireLifeError(t, err, http.StatusUnauthorized, "life_auth_required")
}

func TestLifeIngestRejectsReplay(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, secret, encrypted := newLifeTestAuthenticator(&now)
	body := []byte(`{"device_id":"phone-1"}`)
	req := newSignedLifeRequest(secret, body, now, "nonce_1234567890")

	require.NoError(t, authenticator.Verify(req, body, 1, encrypted))
	err := authenticator.Verify(req, body, 1, encrypted)
	requireLifeError(t, err, http.StatusConflict, "life_replay_detected")
}

func TestLifeIngestRejectsOversizedBodyBeforeVerification(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, _, _ := newLifeTestAuthenticator(&now)
	body := bytes.Repeat([]byte{'x'}, int(lifeTestPolicy().MaxBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
	w := httptest.NewRecorder()

	_, err := authenticator.ReadBounded(w, req, "192.0.2.1")
	requireLifeError(t, err, http.StatusRequestEntityTooLarge, "life_body_too_large")
}

func TestLifeIngestAppliesIPAndProbeRateLimits(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	authenticator, secret, encrypted := newLifeTestAuthenticator(&now)
	body := []byte(`{"device_id":"phone-1"}`)

	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
		_, err := authenticator.ReadBounded(httptest.NewRecorder(), req, "192.0.2.1")
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/life-logger/events", bytes.NewReader(body))
	_, err := authenticator.ReadBounded(httptest.NewRecorder(), req, "192.0.2.1")
	requireLifeError(t, err, http.StatusTooManyRequests, "life_rate_limited")

	for attempt := 0; attempt < 2; attempt++ {
		nonce := fmt.Sprintf("nonce_%010d", attempt)
		req := newSignedLifeRequest(secret, body, now, nonce)
		require.NoError(t, authenticator.Verify(req, body, 1, encrypted))
	}
	req = newSignedLifeRequest(secret, body, now, "nonce_9999999999")
	err = authenticator.Verify(req, body, 1, encrypted)
	requireLifeError(t, err, http.StatusTooManyRequests, "life_rate_limited")
}

func TestLifeIngestConcurrentReplayAllowsExactlyOne(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	policy := lifeTestPolicy()
	policy.ProbeRequestsPerMinute = 100
	masterKey := bytes.Repeat([]byte{'m'}, 32)
	secret := bytes.Repeat([]byte{'s'}, 32)
	encrypted, err := EncryptLifeIngestSecret(masterKey, secret)
	require.NoError(t, err)
	authenticator := NewLifeIngestAuthenticator(policy, masterKey, func() time.Time { return now })
	body := []byte(`{"device_id":"phone-1"}`)

	var successes atomic.Int32
	var replays atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := newSignedLifeRequest(secret, body, now, "nonce_concurrent_1")
			err := authenticator.Verify(req, body, 1, encrypted)
			if err == nil {
				successes.Add(1)
				return
			}
			var ingestErr *LifeIngestError
			if errors.As(err, &ingestErr) && ingestErr.Code == "life_replay_detected" {
				replays.Add(1)
				return
			}
			unexpected.Add(1)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), successes.Load())
	assert.Equal(t, int32(15), replays.Load())
	assert.Zero(t, unexpected.Load())
}
