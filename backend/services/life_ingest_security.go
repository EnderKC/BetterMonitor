package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"
)

type LifeIngestSecurity struct {
	authenticator *LifeIngestAuthenticator
	masterKey     []byte
}

func NewLifeIngestSecurity(
	policy LifeIngestPolicy,
	masterKey []byte,
	now func() time.Time,
) (*LifeIngestSecurity, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("life ingest master key must be 32 bytes")
	}
	keyCopy := append([]byte(nil), masterKey...)
	return &LifeIngestSecurity{
		authenticator: NewLifeIngestAuthenticator(policy, keyCopy, now),
		masterKey:     keyCopy,
	}, nil
}

func (s *LifeIngestSecurity) ReadBounded(
	w http.ResponseWriter,
	r *http.Request,
	clientIP string,
) ([]byte, error) {
	return s.authenticator.ReadBounded(w, r, clientIP)
}

func (s *LifeIngestSecurity) Verify(
	r *http.Request,
	body []byte,
	probeID uint,
	encryptedSecret string,
) error {
	return s.authenticator.Verify(r, body, probeID, encryptedSecret)
}

func (s *LifeIngestSecurity) GenerateProbeSecret() (string, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", "", err
	}
	encrypted, err := EncryptLifeIngestSecret(s.masterKey, secret)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), encrypted, nil
}

func DecodeProbeSecret(encoded string) ([]byte, error) {
	secret, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(secret) != 32 {
		return nil, errors.New("life ingest probe secret must decode to 32 bytes")
	}
	return secret, nil
}
