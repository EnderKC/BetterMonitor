package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/user/server-ops-backend/models"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	defaultReleaseHTTPClient httpDoer = &http.Client{Timeout: 10 * time.Second}
	defaultReleaseAPIBaseURL          = "https://api.github.com"
	releaseHTTPClient        httpDoer = defaultReleaseHTTPClient
	releaseAPIBaseURL                 = defaultReleaseAPIBaseURL
	releaseCacheMu           sync.Mutex
	releaseCache             = make(map[string]cachedRelease)
	releaseCacheTTL          = 5 * time.Minute
)

type cachedRelease struct {
	info      *AgentReleaseInfo
	fetchedAt time.Time
}

// AgentReleaseInfo 描述Agent发行版信息
type AgentReleaseInfo struct {
	Version     string         `json:"version"`
	Channel     string         `json:"channel"`
	Name        string         `json:"name"`
	Notes       string         `json:"notes"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// ReleaseAsset 描述发行版资产
type ReleaseAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	OS   string `json:"os,omitempty"`
	Arch string `json:"arch,omitempty"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// SetReleaseHTTPClient 仅用于测试自定义HTTP客户端
func SetReleaseHTTPClient(client httpDoer) {
	if client == nil {
		return
	}
	releaseHTTPClient = client
}

// ResetReleaseHTTPClient 重置HTTP客户端
func ResetReleaseHTTPClient() {
	releaseHTTPClient = defaultReleaseHTTPClient
}

// SetReleaseAPIBaseURL 仅用于测试自定义API地址
func SetReleaseAPIBaseURL(base string) {
	if base == "" {
		return
	}
	releaseAPIBaseURL = strings.TrimRight(base, "/")
}

// ResetReleaseAPIBaseURL 重置API地址
func ResetReleaseAPIBaseURL() {
	releaseAPIBaseURL = defaultReleaseAPIBaseURL
}

// ClearReleaseCache 清理发布缓存（主要用于测试）
func ClearReleaseCache() {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()
	releaseCache = make(map[string]cachedRelease)
}

// FetchLatestAgentRelease 获取最新的Agent发行信息
func FetchLatestAgentRelease(settings *models.SystemSettings) (*AgentReleaseInfo, error) {
	if settings == nil {
		return nil, fmt.Errorf("系统设置为空")
	}

	repo := strings.TrimSpace(settings.AgentReleaseRepo)
	if repo == "" {
		return nil, fmt.Errorf("未配置Agent发行仓库")
	}

	channelInput := strings.TrimSpace(settings.AgentReleaseChannel)
	if channelInput == "" {
		channelInput = "stable"
	}
	channel, err := NormalizeUpgradeChannel(channelInput)
	if err != nil {
		return nil, contractError("release_channel_invalid", err)
	}

	cacheKey := fmt.Sprintf("%s|%s", strings.ToLower(repo), channel)
	if info := getCachedRelease(cacheKey); info != nil {
		return info, nil
	}

	releases, err := fetchGithubReleases(context.Background(), repo)
	if err != nil {
		return nil, err
	}
	release, version, err := selectLatestReleaseForChannel(releases, channel)
	if err != nil {
		return nil, err
	}

	info := convertGithubRelease(&release, version, channel)
	storeReleaseCache(cacheKey, info)
	return info, nil
}

func convertGithubRelease(release *githubRelease, version, channel string) *AgentReleaseInfo {
	if release == nil {
		return nil
	}

	info := &AgentReleaseInfo{
		Version:     version,
		Channel:     channel,
		Name:        release.Name,
		Notes:       release.Body,
		PublishedAt: release.PublishedAt,
	}

	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, "SHA256SUMS") {
			continue
		}
		osName, archName := parsePlatformFromName(asset.Name)
		ra := ReleaseAsset{
			Name: asset.Name,
			Size: asset.Size,
			OS:   osName,
			Arch: archName,
		}
		info.Assets = append(info.Assets, ra)
	}

	return info
}

func parsePlatformFromName(name string) (string, string) {
	nameLower := strings.ToLower(name)
	var osName, archName string

	for _, candidate := range []string{"linux", "windows", "darwin", "mac", "freebsd", "android"} {
		if strings.Contains(nameLower, candidate) {
			if candidate == "mac" {
				osName = "darwin"
			} else {
				osName = candidate
			}
			break
		}
	}

	for _, candidate := range []string{"amd64", "arm64", "armv7", "arm", "386"} {
		if strings.Contains(nameLower, candidate) {
			archName = candidate
			break
		}
	}

	return osName, archName
}

func githubToken() string {
	if token := strings.TrimSpace(os.Getenv("AGENT_RELEASE_GITHUB_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
}

func getCachedRelease(key string) *AgentReleaseInfo {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()

	if entry, ok := releaseCache[key]; ok {
		if time.Since(entry.fetchedAt) < releaseCacheTTL && entry.info != nil {
			return cloneRelease(entry.info)
		}
		delete(releaseCache, key)
	}
	return nil
}

func storeReleaseCache(key string, info *AgentReleaseInfo) {
	releaseCacheMu.Lock()
	defer releaseCacheMu.Unlock()
	releaseCache[key] = cachedRelease{
		info:      cloneRelease(info),
		fetchedAt: time.Now(),
	}
}

func cloneRelease(info *AgentReleaseInfo) *AgentReleaseInfo {
	if info == nil {
		return nil
	}
	cloned := *info
	if len(info.Assets) > 0 {
		cloned.Assets = make([]ReleaseAsset, len(info.Assets))
		copy(cloned.Assets, info.Assets)
	}
	return &cloned
}
