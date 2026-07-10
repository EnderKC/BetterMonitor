package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/user/server-ops-backend/config"
)

func TestResolveAdminBootstrapUsesExplicitPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin-password")
	cfg := &config.Config{
		AdminUsername:     "ops-admin",
		AdminPassword:     "explicit-password-123",
		AdminPasswordFile: path,
	}

	bootstrap, err := ResolveAdminBootstrap(cfg)
	require.NoError(t, err)
	assert.Equal(t, "ops-admin", bootstrap.Username)
	assert.Equal(t, "explicit-password-123", bootstrap.Password)
	assert.Equal(t, path, bootstrap.PasswordFile)
	_, err = os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestResolveAdminBootstrapReadsExistingPasswordFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin-password")
	require.NoError(t, os.WriteFile(path, []byte("stored-password-123\n"), 0o600))
	cfg := &config.Config{AdminUsername: "admin", AdminPasswordFile: path}

	bootstrap, err := ResolveAdminBootstrap(cfg)
	require.NoError(t, err)
	assert.Equal(t, "stored-password-123", bootstrap.Password)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestResolveAdminBootstrapGeneratesPrivatePasswordFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin-password")
	cfg := &config.Config{AdminUsername: "admin", AdminPasswordFile: path}

	bootstrap, err := ResolveAdminBootstrap(cfg)
	require.NoError(t, err)
	assert.Equal(t, "admin", bootstrap.Username)
	assert.GreaterOrEqual(t, len(bootstrap.Password), 12)
	assert.NotContains(t, bootstrap.Password, "\n")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	persisted, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, bootstrap.Password, strings.TrimSpace(string(persisted)))
}

func TestResolveAdminBootstrapRejectsShortPassword(t *testing.T) {
	cfg := &config.Config{
		AdminUsername: "admin",
		AdminPassword: "too-short",
	}

	_, err := ResolveAdminBootstrap(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 12")
}

func TestResolveAdminBootstrapRejectsInvalidUsername(t *testing.T) {
	cfg := &config.Config{
		AdminUsername: " invalid username ",
		AdminPassword: "valid-password-123",
	}

	_, err := ResolveAdminBootstrap(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "username")
}
