package services

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-backend/config"
)

func TestLoadLifeIngestMasterKeyUsesExplicitValue(t *testing.T) {
	expected := bytes.Repeat([]byte{'k'}, 32)
	cfg := &config.Config{
		LifeIngestMasterKey:     base64.RawURLEncoding.EncodeToString(expected),
		LifeIngestMasterKeyFile: filepath.Join(t.TempDir(), "unused-key"),
	}

	key, err := LoadLifeIngestMasterKey(cfg)
	require.NoError(t, err)
	assert.Equal(t, expected, key)
}

func TestLoadLifeIngestMasterKeyGeneratesPrivateStableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "life-ingest-master-key")
	cfg := &config.Config{LifeIngestMasterKeyFile: path}

	first, err := LoadLifeIngestMasterKey(cfg)
	require.NoError(t, err)
	require.Len(t, first, 32)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	second, err := LoadLifeIngestMasterKey(cfg)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestLoadLifeIngestMasterKeyRejectsInvalidLength(t *testing.T) {
	cfg := &config.Config{
		LifeIngestMasterKey: base64.RawURLEncoding.EncodeToString([]byte("short")),
	}

	_, err := LoadLifeIngestMasterKey(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestLifeIngestPolicyFromConfig(t *testing.T) {
	cfg := &config.Config{
		LifeIngestMaxBodyBytes:           1024,
		LifeIngestClockSkewSeconds:       90,
		LifeIngestIPRequestsPerMinute:    20,
		LifeIngestProbeRequestsPerMinute: 10,
	}

	policy := LifeIngestPolicyFromConfig(cfg)
	assert.Equal(t, int64(1024), policy.MaxBodyBytes)
	assert.Equal(t, 90*time.Second, policy.ClockSkew)
	assert.Equal(t, 20, policy.IPRequestsPerMinute)
	assert.Equal(t, 10, policy.ProbeRequestsPerMinute)
}
