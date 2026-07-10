package upgrader

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgradeMarkerAtomicPrivateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade-marker.json")
	want := UpgradeMarker{
		RequestID:       "job-request-id",
		TargetVersion:   "1.4.0",
		TargetAgentType: "monitor",
		Outcome:         "applied",
		ErrorCode:       "",
	}
	require.NoError(t, WriteUpgradeMarker(path, want))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	got, err := LoadUpgradeMarker(path)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, *got)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "download_url")
	assert.NotContains(t, string(raw), "secret")
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, []string{"error_code", "outcome", "request_id", "target_agent_type", "target_version"}, sortedKeys(decoded))

	require.NoError(t, ClearUpgradeMarker(path))
	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestUpgradeMarkerRejectsMalformedFileWithoutDeletingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade-marker.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"request_id":"job","outcome":"mystery"}`), 0o600))

	marker, err := LoadUpgradeMarker(path)
	assert.Error(t, err)
	assert.Nil(t, marker)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func sortedKeys(input map[string]interface{}) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
