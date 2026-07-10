package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/user/server-ops-backend/models"
)

const maxReleaseMetadataBytes = 8 << 20

func NormalizeArch(kernelArch string) string {
	switch strings.ToLower(strings.TrimSpace(kernelArch)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armv6l", "armv8l", "armhf", "arm":
		return "arm"
	case "i386", "i686", "386":
		return "386"
	default:
		return strings.ToLower(strings.TrimSpace(kernelArch))
	}
}

type UpgradeContractError struct {
	Code string
	Err  error
}

func (e *UpgradeContractError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *UpgradeContractError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func UpgradeContractErrorCode(err error) string {
	var contractErr *UpgradeContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code
	}
	return ""
}

type ResolveUpgradeRequest struct {
	TargetVersion string
	Channel       string
	OS            string
	Arch          string
	AgentType     string
}

type AgentUpgradeAsset struct {
	Version     string
	Channel     string
	AgentType   string
	OS          string
	Arch        string
	Name        string
	DownloadURL string
	Size        int64
	SHA256      string
}

func CanonicalAgentAssetName(version, goos, arch, agentType string) (string, error) {
	normalizedVersion, err := normalizeSemVersionText(version)
	if err != nil {
		return "", contractError("release_version_invalid", err)
	}

	normalizedOS := strings.ToLower(strings.TrimSpace(goos))
	switch normalizedOS {
	case "linux", "windows", "darwin", "android":
	default:
		return "", contractError("release_platform_invalid", fmt.Errorf("unsupported os %q", goos))
	}

	normalizedArch := NormalizeArch(arch)
	switch normalizedArch {
	case "amd64", "arm64", "arm", "386":
	default:
		return "", contractError("release_platform_invalid", fmt.Errorf("unsupported arch %q", arch))
	}

	normalizedType := strings.ToLower(strings.TrimSpace(agentType))
	var base string
	switch normalizedType {
	case "full":
		base = "better-monitor-agent"
	case "monitor":
		base = "better-monitor-agent-monitor"
	default:
		return "", contractError("release_agent_type_invalid", fmt.Errorf("unsupported agent type %q", agentType))
	}

	name := fmt.Sprintf("%s-%s-%s-%s", base, normalizedVersion, normalizedOS, normalizedArch)
	if normalizedOS == "windows" {
		name += ".exe"
	}
	return name, nil
}

func ResolveAgentUpgradeAsset(
	ctx context.Context,
	settings *models.SystemSettings,
	request ResolveUpgradeRequest,
) (AgentUpgradeAsset, error) {
	if settings == nil {
		return AgentUpgradeAsset{}, contractError("release_settings_invalid", fmt.Errorf("settings are required"))
	}
	repo := strings.TrimSpace(settings.AgentReleaseRepo)
	if repo == "" {
		return AgentUpgradeAsset{}, contractError("release_repository_missing", fmt.Errorf("release repository is required"))
	}

	requestedChannel := strings.TrimSpace(request.Channel)
	if requestedChannel == "" {
		requestedChannel = strings.TrimSpace(settings.AgentReleaseChannel)
	}
	if requestedChannel == "" {
		requestedChannel = "stable"
	}
	channel, err := NormalizeUpgradeChannel(requestedChannel)
	if err != nil {
		return AgentUpgradeAsset{}, contractError("release_channel_invalid", err)
	}

	canonicalName, err := CanonicalAgentAssetName(
		firstNonEmpty(request.TargetVersion, "0.0.0"),
		request.OS,
		request.Arch,
		request.AgentType,
	)
	if err != nil && strings.TrimSpace(request.TargetVersion) != "" {
		return AgentUpgradeAsset{}, err
	}

	var release githubRelease
	var version string
	if strings.TrimSpace(request.TargetVersion) != "" {
		version, err = normalizeSemVersionText(request.TargetVersion)
		if err != nil {
			return AgentUpgradeAsset{}, contractError("release_version_invalid", err)
		}
		release, err = fetchGithubReleaseByTag(ctx, repo, "v"+version)
		if err != nil {
			return AgentUpgradeAsset{}, err
		}
		matches, matchErr := releaseMatchesChannel(release, channel)
		if matchErr != nil {
			return AgentUpgradeAsset{}, matchErr
		}
		if !matches {
			return AgentUpgradeAsset{}, contractError("release_channel_mismatch", fmt.Errorf("target release does not match channel %s", channel))
		}
	} else {
		releases, fetchErr := fetchGithubReleases(ctx, repo)
		if fetchErr != nil {
			return AgentUpgradeAsset{}, fetchErr
		}
		release, version, err = selectLatestReleaseForChannel(releases, channel)
		if err != nil {
			return AgentUpgradeAsset{}, err
		}
	}

	canonicalName, err = CanonicalAgentAssetName(version, request.OS, request.Arch, request.AgentType)
	if err != nil {
		return AgentUpgradeAsset{}, err
	}
	selected, ok := findExactGithubAsset(release.Assets, canonicalName)
	if !ok {
		return AgentUpgradeAsset{}, contractError("release_asset_missing", fmt.Errorf("canonical asset is missing"))
	}
	if selected.Size <= 0 {
		return AgentUpgradeAsset{}, contractError("release_asset_size_invalid", fmt.Errorf("asset size must be positive"))
	}

	downloadURL, err := applyStrictReleaseMirror(selected.BrowserDownloadURL, settings.AgentReleaseMirror)
	if err != nil {
		return AgentUpgradeAsset{}, err
	}
	checksumAsset, ok := findExactGithubAsset(release.Assets, "SHA256SUMS")
	if !ok {
		return AgentUpgradeAsset{}, contractError("release_checksum_missing", fmt.Errorf("SHA256SUMS asset is missing"))
	}
	checksumURL, err := validateHTTPSURL(checksumAsset.BrowserDownloadURL)
	if err != nil {
		return AgentUpgradeAsset{}, contractError("release_checksum_url_invalid", err)
	}
	checksum, err := fetchRequiredAssetChecksum(ctx, checksumURL, canonicalName)
	if err != nil {
		return AgentUpgradeAsset{}, err
	}

	return AgentUpgradeAsset{
		Version:     version,
		Channel:     channel,
		AgentType:   strings.ToLower(strings.TrimSpace(request.AgentType)),
		OS:          strings.ToLower(strings.TrimSpace(request.OS)),
		Arch:        NormalizeArch(request.Arch),
		Name:        canonicalName,
		DownloadURL: downloadURL,
		Size:        selected.Size,
		SHA256:      checksum,
	}, nil
}

func fetchGithubReleases(ctx context.Context, repo string) ([]githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=100", releaseAPIBaseURL, repo)
	var releases []githubRelease
	if err := fetchGithubJSON(ctx, endpoint, &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func fetchGithubReleaseByTag(ctx context.Context, repo, tag string) (githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/tags/%s", releaseAPIBaseURL, repo, tag)
	var release githubRelease
	if err := fetchGithubJSON(ctx, endpoint, &release); err != nil {
		return githubRelease{}, err
	}
	return release, nil
}

func fetchGithubJSON(ctx context.Context, endpoint string, output interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return contractError("release_request_failed", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "better-monitor-dashboard")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return contractError("release_request_failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return contractError("release_request_failed", fmt.Errorf("unexpected GitHub status %d", resp.StatusCode))
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseMetadataBytes))
	if err := decoder.Decode(output); err != nil {
		return contractError("release_response_invalid", err)
	}
	return nil
}

func selectLatestReleaseForChannel(releases []githubRelease, channel string) (githubRelease, string, error) {
	type candidate struct {
		release githubRelease
		version SemVersion
		raw     string
	}
	var candidates []candidate
	for _, release := range releases {
		matches, err := releaseMatchesChannel(release, channel)
		if err != nil || !matches {
			continue
		}
		raw, err := normalizeSemVersionText(release.TagName)
		if err != nil {
			continue
		}
		parsed, err := ParseSemVersion(raw)
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{release: release, version: parsed, raw: raw})
	}
	if len(candidates) == 0 {
		return githubRelease{}, "", contractError("release_not_found", fmt.Errorf("no release matches channel %s", channel))
	}
	sort.Slice(candidates, func(i, j int) bool {
		return CompareSemVersion(candidates[i].version, candidates[j].version) > 0
	})
	return candidates[0].release, candidates[0].raw, nil
}

func releaseMatchesChannel(release githubRelease, channel string) (bool, error) {
	if release.Draft {
		return false, nil
	}
	raw, err := normalizeSemVersionText(release.TagName)
	if err != nil {
		return false, nil
	}
	version, err := ParseSemVersion(raw)
	if err != nil {
		return false, nil
	}
	hasPrerelease := len(version.Prerelease) > 0
	hasNightly := false
	for _, identifier := range version.Prerelease {
		if strings.Contains(strings.ToLower(identifier), "nightly") {
			hasNightly = true
			break
		}
	}

	switch channel {
	case "stable":
		return !release.Prerelease && !hasPrerelease, nil
	case "prerelease":
		return release.Prerelease && hasPrerelease && !hasNightly, nil
	case "nightly":
		return release.Prerelease && hasPrerelease && hasNightly, nil
	default:
		return false, contractError("release_channel_invalid", fmt.Errorf("unsupported channel %q", channel))
	}
}

func findExactGithubAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func applyStrictReleaseMirror(downloadURL, mirror string) (string, error) {
	result := downloadURL
	if strings.TrimSpace(mirror) != "" && strings.HasPrefix(downloadURL, "https://github.com") {
		mirrorURL, err := validateHTTPSURL(strings.TrimRight(strings.TrimSpace(mirror), "/"))
		if err != nil {
			return "", contractError("release_mirror_invalid", err)
		}
		result = mirrorURL + strings.TrimPrefix(downloadURL, "https://github.com")
	}
	validated, err := validateHTTPSURL(result)
	if err != nil {
		return "", contractError("release_asset_url_invalid", err)
	}
	return validated, nil
}

func validateHTTPSURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("URL must be absolute HTTPS without credentials")
	}
	return parsed.String(), nil
}

func fetchRequiredAssetChecksum(ctx context.Context, checksumURL, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return "", contractError("release_checksum_fetch_failed", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return "", contractError("release_checksum_fetch_failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", contractError("release_checksum_fetch_failed", fmt.Errorf("unexpected checksum status %d", resp.StatusCode))
	}

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, maxReleaseMetadataBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(filepathBase(fields[len(fields)-1]), "*")
		if name != assetName {
			continue
		}
		checksum := strings.ToLower(strings.TrimSpace(fields[0]))
		if !isSHA256Hex(checksum) {
			return "", contractError("release_checksum_invalid", fmt.Errorf("asset checksum is invalid"))
		}
		return checksum, nil
	}
	if err := scanner.Err(); err != nil {
		return "", contractError("release_checksum_fetch_failed", err)
	}
	return "", contractError("release_checksum_missing", fmt.Errorf("asset checksum is missing"))
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return false
	}
	return true
}

func normalizeSemVersionText(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	if _, err := ParseSemVersion(value); err != nil {
		return "", err
	}
	return value, nil
}

func filepathBase(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[index+1:]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func contractError(code string, err error) error {
	return &UpgradeContractError{Code: code, Err: err}
}
