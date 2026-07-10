package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSemVersion(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		major      uint64
		minor      uint64
		patch      uint64
		prerelease []string
		wantErr    bool
	}{
		{name: "stable", raw: "1.4.0", major: 1, minor: 4, patch: 0},
		{name: "leading v", raw: "v2.3.4", major: 2, minor: 3, patch: 4},
		{name: "prerelease and build", raw: "1.5.0-rc.2+build.7", major: 1, minor: 5, patch: 0, prerelease: []string{"rc", "2"}},
		{name: "missing patch", raw: "1.4", wantErr: true},
		{name: "leading zero major", raw: "01.4.0", wantErr: true},
		{name: "leading zero prerelease", raw: "1.4.0-rc.02", wantErr: true},
		{name: "empty prerelease", raw: "1.4.0-", wantErr: true},
		{name: "latest", raw: "latest", wantErr: true},
		{name: "dev", raw: "dev", wantErr: true},
		{name: "unknown", raw: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSemVersion(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.major, got.Major)
			assert.Equal(t, tt.minor, got.Minor)
			assert.Equal(t, tt.patch, got.Patch)
			assert.Equal(t, tt.prerelease, got.Prerelease)
		})
	}
}

func TestCompareSemVersion(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
		"1.0.1",
		"1.1.0",
		"2.0.0",
	}

	for i := 0; i < len(ordered)-1; i++ {
		left, err := ParseSemVersion(ordered[i])
		require.NoError(t, err)
		right, err := ParseSemVersion(ordered[i+1])
		require.NoError(t, err)
		assert.Negative(t, CompareSemVersion(left, right), "%s must sort before %s", ordered[i], ordered[i+1])
		assert.Positive(t, CompareSemVersion(right, left), "%s must sort after %s", ordered[i+1], ordered[i])
	}

	base, err := ParseSemVersion("1.2.3+build.1")
	require.NoError(t, err)
	other, err := ParseSemVersion("1.2.3+build.9")
	require.NoError(t, err)
	assert.Zero(t, CompareSemVersion(base, other))
}

func TestNormalizeUpgradeChannel(t *testing.T) {
	for _, channel := range []string{"stable", "prerelease", "nightly"} {
		got, err := NormalizeUpgradeChannel("  " + channel + "  ")
		require.NoError(t, err)
		assert.Equal(t, channel, got)
	}

	for _, channel := range []string{"", "dev", "canary", "latest"} {
		_, err := NormalizeUpgradeChannel(channel)
		require.Error(t, err)
	}
}
