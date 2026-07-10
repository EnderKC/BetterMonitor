package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/user/server-ops-backend/config"
)

func LoadLifeIngestMasterKey(cfg *config.Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("life ingest config is required")
	}
	if cfg.LifeIngestMasterKey != "" {
		return decodeLifeIngestMasterKey(cfg.LifeIngestMasterKey)
	}

	path := strings.TrimSpace(cfg.LifeIngestMasterKeyFile)
	if path == "" {
		return nil, errors.New("life ingest master key file is required")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("secure life ingest master key file: %w", err)
		}
		return decodeLifeIngestMasterKey(strings.TrimSpace(string(data)))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read life ingest master key file: %w", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate life ingest master key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	if err := config.WritePrivateFileAtomic(path, []byte(encoded+"\n")); err != nil {
		return nil, fmt.Errorf("write life ingest master key file: %w", err)
	}
	return key, nil
}

func LifeIngestPolicyFromConfig(cfg *config.Config) LifeIngestPolicy {
	if cfg == nil {
		return LifeIngestPolicy{}
	}
	return LifeIngestPolicy{
		MaxBodyBytes:           cfg.LifeIngestMaxBodyBytes,
		ClockSkew:              time.Duration(cfg.LifeIngestClockSkewSeconds) * time.Second,
		IPRequestsPerMinute:    cfg.LifeIngestIPRequestsPerMinute,
		ProbeRequestsPerMinute: cfg.LifeIngestProbeRequestsPerMinute,
	}
}

func decodeLifeIngestMasterKey(encoded string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode life ingest master key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("life ingest master key must be 32 bytes")
	}
	return key, nil
}
