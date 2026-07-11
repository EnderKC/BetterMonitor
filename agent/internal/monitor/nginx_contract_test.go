//go:build !monitor_only

package monitor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nginxContractFixture struct {
	configDir  string
	configPath string
	logDir     string
	logPath    string
	configID   string
	logID      string
}

func setupNginxContractFixture(t *testing.T) nginxContractFixture {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "nginx")
	logDir := filepath.Join(root, "logs")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	configPath := filepath.Join(configDir, "site.conf")
	logPath := filepath.Join(logDir, "access.log")
	require.NoError(t, os.WriteFile(configPath, []byte("server { listen 80; }\n"), 0o644))
	require.NoError(t, os.WriteFile(logPath, []byte("line one\nline two\n"), 0o644))

	originalDetect := detectNginxPaths
	originalLogs := nginxLogDirectories
	originalTester := testNginxConfiguration
	detectNginxPaths = func() (string, string, string) {
		return filepath.Join(configDir, "nginx.conf"), "nginx", configDir
	}
	nginxLogDirectories = func() []string { return []string{logDir} }
	testNginxConfiguration = func() error { return nil }
	t.Cleanup(func() {
		detectNginxPaths = originalDetect
		nginxLogDirectories = originalLogs
		testNginxConfiguration = originalTester
	})

	return nginxContractFixture{
		configDir:  configDir,
		configPath: configPath,
		logDir:     logDir,
		logPath:    logPath,
		configID:   nginxManagedFileID(configPath),
		logID:      nginxManagedFileID(logPath),
	}
}

func TestNginxContractListsAndResolvesOnlyOpaqueIDs(t *testing.T) {
	fixture := setupNginxContractFixture(t)

	configs, err := GetNginxConfigsList()
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, fixture.configID, configs[0].ID)
	assert.Len(t, configs[0].ID, 32)
	encodedConfigs, err := json.Marshal(configs)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedConfigs), fixture.configDir)
	assert.NotContains(t, string(encodedConfigs), `"path"`)

	resolvedConfig, err := resolveNginxConfigID(fixture.configID)
	require.NoError(t, err)
	assert.Equal(t, fixture.configPath, resolvedConfig.Path)
	_, err = resolveNginxConfigID("not-an-id")
	assert.Error(t, err)

	logs, err := GetNginxLogsList()
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, fixture.logID, logs[0].ID)
	encodedLogs, err := json.Marshal(logs)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedLogs), fixture.logDir)
	assert.NotContains(t, string(encodedLogs), `"path"`)

	resolvedLog, err := resolveNginxLogID(fixture.logID)
	require.NoError(t, err)
	assert.Equal(t, fixture.logPath, resolvedLog.Path)
}

func TestNginxContractRejectsPathAndOldIDFallbacks(t *testing.T) {
	fixture := setupNginxContractFixture(t)

	for _, params := range []map[string]interface{}{
		{"path": fixture.configPath},
		{"id": fixture.configID},
	} {
		_, err := HandleNginxCommand("nginx_config_content", params)
		assert.Error(t, err)
	}
	result, err := HandleNginxCommand("nginx_config_content", map[string]interface{}{"config_id": fixture.configID})
	require.NoError(t, err)
	assert.Contains(t, result, "listen 80")

	for _, action := range []string{"nginx_log_content", "nginx_log_download"} {
		for _, params := range []map[string]interface{}{
			{"path": fixture.logPath},
			{"id": fixture.logID},
		} {
			_, err := HandleNginxCommand(action, params)
			assert.Error(t, err)
		}
	}
	result, err = HandleNginxCommand("nginx_log_content", map[string]interface{}{"log_id": fixture.logID})
	require.NoError(t, err)
	assert.Contains(t, result, "line one")
}

func TestNginxContractManagedCreateAndSymlinkRejection(t *testing.T) {
	fixture := setupNginxContractFixture(t)

	for _, name := range []string{"", "../escape", "nested/site", `nested\\site`, ".", "..", "-site"} {
		_, err := validateManagedConfigName(name)
		assert.Error(t, err, name)
	}
	managedName, err := validateManagedConfigName("new-site")
	require.NoError(t, err)
	assert.Equal(t, "new-site.conf", managedName)

	_, err = HandleNginxCommand("nginx_create_config", map[string]interface{}{
		"name":    "new-site",
		"content": "server { listen 8080; }\n",
	})
	require.NoError(t, err)
	created := filepath.Join(fixture.configDir, "new-site.conf")
	assert.FileExists(t, created)

	outside := filepath.Join(t.TempDir(), "outside.conf")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))
	symlink := filepath.Join(fixture.configDir, "linked.conf")
	require.NoError(t, os.Symlink(outside, symlink))
	configs, err := GetNginxConfigsList()
	require.NoError(t, err)
	for _, config := range configs {
		assert.NotEqual(t, nginxManagedFileID(symlink), config.ID)
	}
	_, err = resolveNginxConfigID(nginxManagedFileID(symlink))
	assert.Error(t, err)
}

func TestNginxContractAtomicSaveRollsBackFailedConfigTest(t *testing.T) {
	fixture := setupNginxContractFixture(t)
	testNginxConfiguration = func() error { return errors.New("nginx -t rejected config") }

	_, err := HandleNginxCommand("nginx_save_config", map[string]interface{}{
		"config_id": fixture.configID,
		"content":   "server { broken; }\n",
	})
	assert.Error(t, err)
	content, readErr := os.ReadFile(fixture.configPath)
	require.NoError(t, readErr)
	assert.Equal(t, "server { listen 80; }\n", string(content))

	entries, err := os.ReadDir(fixture.configDir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.Contains(entry.Name(), ".better-monitor-"), entry.Name())
		assert.False(t, strings.HasSuffix(entry.Name(), ".bak"), entry.Name())
	}
}

func TestNginxContractRejectsOversizedConfigContent(t *testing.T) {
	fixture := setupNginxContractFixture(t)
	_, err := HandleNginxCommand("nginx_save_config", map[string]interface{}{
		"config_id": fixture.configID,
		"content":   strings.Repeat("a", (1<<20)+1),
	})
	assert.Error(t, err)
}
