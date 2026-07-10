package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/server-ops-backend/models"
)

type agentReleaseContractFixture struct {
	Repository string                    `json:"repository"`
	Releases   []agentReleaseFixture     `json:"releases"`
	Cases      []agentReleaseFixtureCase `json:"cases"`
}

type agentReleaseFixture struct {
	githubRelease
	Checksums map[string]string `json:"checksums"`
}

type agentReleaseFixtureCase struct {
	Name           string `json:"name"`
	Channel        string `json:"channel"`
	TargetVersion  string `json:"target_version"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	AgentType      string `json:"agent_type"`
	ExpectedTag    string `json:"expected_tag"`
	ExpectedAsset  string `json:"expected_asset"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ExpectedError  string `json:"expected_error"`
}

func TestCanonicalAgentAssetName(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		goos      string
		arch      string
		agentType string
		want      string
		wantErr   bool
	}{
		{
			name: "full linux normalizes arch", version: "v1.4.0", goos: "linux", arch: "x86_64", agentType: "full",
			want: "better-monitor-agent-1.4.0-linux-amd64",
		},
		{
			name: "monitor prerelease", version: "1.5.0-rc.2", goos: "linux", arch: "arm64", agentType: "monitor",
			want: "better-monitor-agent-monitor-1.5.0-rc.2-linux-arm64",
		},
		{
			name: "windows exe", version: "1.4.0", goos: "windows", arch: "amd64", agentType: "full",
			want: "better-monitor-agent-1.4.0-windows-amd64.exe",
		},
		{name: "invalid version", version: "latest", goos: "linux", arch: "amd64", agentType: "full", wantErr: true},
		{name: "invalid os", version: "1.4.0", goos: "plan9", arch: "amd64", agentType: "full", wantErr: true},
		{name: "invalid arch", version: "1.4.0", goos: "linux", arch: "mips64", agentType: "full", wantErr: true},
		{name: "invalid type", version: "1.4.0", goos: "linux", arch: "amd64", agentType: "legacy", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalAgentAssetName(tt.version, tt.goos, tt.arch, tt.agentType)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveAgentUpgradeAssetFromSharedFixture(t *testing.T) {
	fixture := loadAgentReleaseContractFixture(t)
	server := newAgentReleaseFixtureServer(t, fixture)
	defer server.Close()

	SetReleaseAPIBaseURL(server.URL)
	defer ResetReleaseAPIBaseURL()
	SetReleaseHTTPClient(server.Client())
	defer ResetReleaseHTTPClient()
	ClearReleaseCache()
	defer ClearReleaseCache()

	settings := &models.SystemSettings{AgentReleaseRepo: fixture.Repository}
	for _, fixtureCase := range fixture.Cases {
		t.Run(fixtureCase.Name, func(t *testing.T) {
			settings.AgentReleaseChannel = fixtureCase.Channel
			asset, err := ResolveAgentUpgradeAsset(context.Background(), settings, ResolveUpgradeRequest{
				TargetVersion: fixtureCase.TargetVersion,
				Channel:       fixtureCase.Channel,
				OS:            fixtureCase.OS,
				Arch:          fixtureCase.Arch,
				AgentType:     fixtureCase.AgentType,
			})
			if fixtureCase.ExpectedError != "" {
				require.Error(t, err)
				assert.Equal(t, fixtureCase.ExpectedError, UpgradeContractErrorCode(err))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, strings.TrimPrefix(fixtureCase.ExpectedTag, "v"), asset.Version)
			assert.Equal(t, fixtureCase.Channel, asset.Channel)
			assert.Equal(t, fixtureCase.ExpectedAsset, asset.Name)
			assert.Equal(t, fixtureCase.ExpectedSHA256, asset.SHA256)
			assert.Positive(t, asset.Size)
			assert.True(t, strings.HasPrefix(asset.DownloadURL, "https://"))
		})
	}
}

func TestResolveAgentUpgradeAssetRejectsInvalidMetadata(t *testing.T) {
	fixture := loadAgentReleaseContractFixture(t)
	stable := fixture.Releases[2]
	settings := &models.SystemSettings{AgentReleaseRepo: fixture.Repository, AgentReleaseChannel: "stable"}

	tests := []struct {
		name     string
		mutate   func(*agentReleaseFixture)
		wantCode string
	}{
		{
			name: "zero asset size",
			mutate: func(release *agentReleaseFixture) {
				release.Assets[0].Size = 0
			},
			wantCode: "release_asset_size_invalid",
		},
		{
			name: "non https asset",
			mutate: func(release *agentReleaseFixture) {
				release.Assets[0].BrowserDownloadURL = "http://downloads.example.test/agent"
			},
			wantCode: "release_asset_url_invalid",
		},
		{
			name: "invalid checksum",
			mutate: func(release *agentReleaseFixture) {
				release.Checksums[release.Assets[0].Name] = "not-a-sha"
			},
			wantCode: "release_checksum_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := stable
			release.Assets = append([]githubAsset(nil), stable.Assets...)
			release.Checksums = cloneStringMap(stable.Checksums)
			tt.mutate(&release)

			server := newAgentReleaseFixtureServer(t, agentReleaseContractFixture{
				Repository: fixture.Repository,
				Releases:   []agentReleaseFixture{release},
			})
			defer server.Close()

			SetReleaseAPIBaseURL(server.URL)
			SetReleaseHTTPClient(server.Client())
			ClearReleaseCache()

			_, err := ResolveAgentUpgradeAsset(context.Background(), settings, ResolveUpgradeRequest{
				TargetVersion: "1.4.0",
				Channel:       "stable",
				OS:            "linux",
				Arch:          "amd64",
				AgentType:     "full",
			})
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, UpgradeContractErrorCode(err))
		})
	}

	ResetReleaseAPIBaseURL()
	ResetReleaseHTTPClient()
	ClearReleaseCache()
}

func loadAgentReleaseContractFixture(t *testing.T) agentReleaseContractFixture {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "agent-release-contract.json")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture agentReleaseContractFixture
	require.NoError(t, json.Unmarshal(body, &fixture))
	return fixture
}

func newAgentReleaseFixtureServer(t *testing.T, fixture agentReleaseContractFixture) *httptest.Server {
	t.Helper()
	var serverURL string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			writeFixtureReleases(t, w, serverURL, fixture.Releases)
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			tag := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			for _, release := range fixture.Releases {
				if release.TagName == tag {
					writeFixtureRelease(t, w, serverURL, release)
					return
				}
			}
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) < 3 {
				http.NotFound(w, r)
				return
			}
			tag := parts[1]
			for _, release := range fixture.Releases {
				if release.TagName != tag {
					continue
				}
				for name, checksum := range release.Checksums {
					fmt.Fprintf(w, "%s  %s\n", checksum, name)
				}
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	return server
}

func writeFixtureReleases(t *testing.T, w http.ResponseWriter, serverURL string, releases []agentReleaseFixture) {
	t.Helper()
	output := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		output = append(output, fixtureGithubRelease(serverURL, release))
	}
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(output))
}

func writeFixtureRelease(t *testing.T, w http.ResponseWriter, serverURL string, release agentReleaseFixture) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(fixtureGithubRelease(serverURL, release)))
}

func fixtureGithubRelease(serverURL string, release agentReleaseFixture) githubRelease {
	output := release.githubRelease
	output.Assets = append([]githubAsset(nil), release.Assets...)
	for index := range output.Assets {
		if strings.HasPrefix(output.Assets[index].BrowserDownloadURL, "http://") {
			continue
		}
		output.Assets[index].BrowserDownloadURL = fmt.Sprintf(
			"%s/downloads/%s/%s",
			serverURL,
			release.TagName,
			output.Assets[index].Name,
		)
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
