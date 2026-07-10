package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLifeIngestMaxBodyBytes           = 4 << 20
	defaultLifeIngestClockSkew              = 5 * time.Minute
	defaultLifeIngestIPRequestsPerMinute    = 240
	defaultLifeIngestProbeRequestsPerMinute = 120
)

var lifeNoncePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

type LifeIngestPolicy struct {
	MaxBodyBytes           int64
	ClockSkew              time.Duration
	IPRequestsPerMinute    int
	ProbeRequestsPerMinute int
}

type LifeIngestError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *LifeIngestError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *LifeIngestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type rateWindow struct {
	StartedAt time.Time
	Count     int
}

type LifeIngestAuthenticator struct {
	policy       LifeIngestPolicy
	masterKey    []byte
	now          func() time.Time
	mu           sync.Mutex
	nonces       map[string]time.Time
	ipWindows    map[string]rateWindow
	probeWindows map[uint]rateWindow
	lastCleanup  time.Time
}

func NewLifeIngestAuthenticator(
	policy LifeIngestPolicy,
	masterKey []byte,
	now func() time.Time,
) *LifeIngestAuthenticator {
	if policy.MaxBodyBytes <= 0 {
		policy.MaxBodyBytes = defaultLifeIngestMaxBodyBytes
	}
	if policy.ClockSkew <= 0 {
		policy.ClockSkew = defaultLifeIngestClockSkew
	}
	if policy.IPRequestsPerMinute <= 0 {
		policy.IPRequestsPerMinute = defaultLifeIngestIPRequestsPerMinute
	}
	if policy.ProbeRequestsPerMinute <= 0 {
		policy.ProbeRequestsPerMinute = defaultLifeIngestProbeRequestsPerMinute
	}
	if now == nil {
		now = time.Now
	}

	return &LifeIngestAuthenticator{
		policy:       policy,
		masterKey:    append([]byte(nil), masterKey...),
		now:          now,
		nonces:       make(map[string]time.Time),
		ipWindows:    make(map[string]rateWindow),
		probeWindows: make(map[uint]rateWindow),
	}
}

func EncryptLifeIngestSecret(masterKey, secret []byte) (string, error) {
	if len(masterKey) != 32 {
		return "", errors.New("life ingest master key must be 32 bytes")
	}
	if len(secret) != 32 {
		return "", errors.New("life ingest probe secret must be 32 bytes")
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate life ingest encryption nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, secret, nil)
	payload := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecryptLifeIngestSecret(masterKey []byte, encoded string) ([]byte, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("life ingest master key must be 32 bytes")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode life ingest secret: %w", err)
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) <= gcm.NonceSize() {
		return nil, errors.New("encrypted life ingest secret is truncated")
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt life ingest secret: %w", err)
	}
	if len(plaintext) != 32 {
		return nil, errors.New("decrypted life ingest secret must be 32 bytes")
	}
	return plaintext, nil
}

func (a *LifeIngestAuthenticator) ReadBounded(
	w http.ResponseWriter,
	r *http.Request,
	clientIP string,
) ([]byte, error) {
	if a == nil || r == nil || r.Body == nil {
		return nil, lifeIngestError(http.StatusBadRequest, "life_auth_invalid", "invalid ingest request", nil)
	}
	if err := a.allowIP(clientIP); err != nil {
		return nil, err
	}

	r.Body = http.MaxBytesReader(w, r.Body, a.policy.MaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, lifeIngestError(
				http.StatusRequestEntityTooLarge,
				"life_body_too_large",
				"life ingest request body is too large",
				err,
			)
		}
		return nil, lifeIngestError(http.StatusBadRequest, "life_auth_invalid", "failed to read ingest request", err)
	}
	return body, nil
}

func (a *LifeIngestAuthenticator) Verify(
	r *http.Request,
	body []byte,
	probeID uint,
	encryptedSecret string,
) error {
	if a == nil || r == nil || probeID == 0 || encryptedSecret == "" {
		return lifeIngestError(http.StatusUnauthorized, "life_auth_invalid", "invalid life ingest credentials", nil)
	}

	timestampValue := strings.TrimSpace(r.Header.Get("X-Life-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("X-Life-Nonce"))
	signatureValue := strings.TrimSpace(r.Header.Get("X-Life-Signature"))
	if timestampValue == "" || nonce == "" || signatureValue == "" || !lifeNoncePattern.MatchString(nonce) {
		return lifeIngestError(http.StatusUnauthorized, "life_auth_required", "life ingest authentication is required", nil)
	}

	timestampSeconds, err := strconv.ParseInt(timestampValue, 10, 64)
	if err != nil {
		return lifeIngestError(http.StatusUnauthorized, "life_timestamp_invalid", "invalid life ingest timestamp", err)
	}
	requestTime := time.Unix(timestampSeconds, 0)
	now := a.now()
	if requestTime.Before(now.Add(-a.policy.ClockSkew)) || requestTime.After(now.Add(a.policy.ClockSkew)) {
		return lifeIngestError(http.StatusUnauthorized, "life_timestamp_invalid", "life ingest timestamp is outside the allowed window", nil)
	}

	if signatureValue != strings.ToLower(signatureValue) || len(signatureValue) != sha256.Size*2 {
		return lifeIngestError(http.StatusUnauthorized, "life_signature_invalid", "invalid life ingest signature", nil)
	}
	providedSignature, err := hex.DecodeString(signatureValue)
	if err != nil || len(providedSignature) != sha256.Size {
		return lifeIngestError(http.StatusUnauthorized, "life_signature_invalid", "invalid life ingest signature", err)
	}

	secret, err := DecryptLifeIngestSecret(a.masterKey, encryptedSecret)
	if err != nil {
		return lifeIngestError(http.StatusServiceUnavailable, "life_auth_unavailable", "life ingest authentication is unavailable", err)
	}
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		strings.ToUpper(r.Method),
		r.URL.EscapedPath(),
		timestampValue,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	if !hmac.Equal(providedSignature, mac.Sum(nil)) {
		return lifeIngestError(http.StatusUnauthorized, "life_signature_invalid", "invalid life ingest signature", nil)
	}

	return a.acceptProbeNonce(probeID, nonce)
}

func (a *LifeIngestAuthenticator) allowIP(clientIP string) error {
	key := strings.TrimSpace(clientIP)
	if key == "" {
		key = "unknown"
	}
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanupLocked(now)

	window := a.ipWindows[key]
	if window.StartedAt.IsZero() || now.Sub(window.StartedAt) >= time.Minute {
		window = rateWindow{StartedAt: now}
	}
	if window.Count >= a.policy.IPRequestsPerMinute {
		return lifeIngestError(http.StatusTooManyRequests, "life_rate_limited", "life ingest rate limit exceeded", nil)
	}
	window.Count++
	a.ipWindows[key] = window
	return nil
}

func (a *LifeIngestAuthenticator) acceptProbeNonce(probeID uint, nonce string) error {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanupLocked(now)

	nonceKey := fmt.Sprintf("%d:%s", probeID, nonce)
	if expiresAt, ok := a.nonces[nonceKey]; ok && now.Before(expiresAt) {
		return lifeIngestError(http.StatusConflict, "life_replay_detected", "life ingest request was already used", nil)
	}

	window := a.probeWindows[probeID]
	if window.StartedAt.IsZero() || now.Sub(window.StartedAt) >= time.Minute {
		window = rateWindow{StartedAt: now}
	}
	if window.Count >= a.policy.ProbeRequestsPerMinute {
		return lifeIngestError(http.StatusTooManyRequests, "life_rate_limited", "life ingest rate limit exceeded", nil)
	}
	window.Count++
	a.probeWindows[probeID] = window
	a.nonces[nonceKey] = now.Add(a.policy.ClockSkew)
	return nil
}

func (a *LifeIngestAuthenticator) cleanupLocked(now time.Time) {
	if !a.lastCleanup.IsZero() && now.Sub(a.lastCleanup) < time.Minute {
		return
	}
	for key, expiresAt := range a.nonces {
		if !now.Before(expiresAt) {
			delete(a.nonces, key)
		}
	}
	for key, window := range a.ipWindows {
		if now.Sub(window.StartedAt) >= time.Minute {
			delete(a.ipWindows, key)
		}
	}
	for key, window := range a.probeWindows {
		if now.Sub(window.StartedAt) >= time.Minute {
			delete(a.probeWindows, key)
		}
	}
	a.lastCleanup = now
}

func lifeIngestError(status int, code, message string, err error) *LifeIngestError {
	return &LifeIngestError{Status: status, Code: code, Message: message, Err: err}
}
