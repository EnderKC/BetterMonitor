package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/user/server-ops-backend/models"
)

const (
	releaseMaxRetries    = 3
	releaseRetryBaseWait = 500 * time.Millisecond
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
	repo      string
	channel   string
}

// AgentReleaseInfo 描述Agent发行版信息
type AgentReleaseInfo struct {
	Version     string         `json:"version"`
	Name        string         `json:"name"`
	Notes       string         `json:"notes"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// ReleaseAsset 描述发行版资产
type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
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

	channel := strings.ToLower(strings.TrimSpace(settings.AgentReleaseChannel))
	if channel == "" {
		channel = "stable"
	}

	cacheKey := fmt.Sprintf("%s|%s", strings.ToLower(repo), channel)
	if info := getCachedRelease(cacheKey); info != nil {
		return applyDownloadMirror(info, settings.AgentReleaseMirror), nil
	}

	release, err := fetchReleaseFromGitHub(repo, channel)
	if err != nil {
		return nil, err
	}

	info := convertGithubRelease(release)
	storeReleaseCache(cacheKey, info)
	return applyDownloadMirror(info, settings.AgentReleaseMirror), nil
}

func fetchReleaseFromGitHub(repo, channel string) (*githubRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", releaseAPIBaseURL, repo)
	if channel == "dev" || channel == "nightly" || channel == "prerelease" || channel == "canary" {
		endpoint = fmt.Sprintf("%s/repos/%s/releases?per_page=1", releaseAPIBaseURL, repo)
	}

	var lastErr error
	for attempt := 0; attempt < releaseMaxRetries; attempt++ {
		if attempt > 0 {
			wait := releaseRetryBaseWait * (1 << (attempt - 1)) // 500ms, 1s
			log.Printf("GitHub API 请求失败，%v 后进行第 %d 次重试: %v", wait, attempt+1, lastErr)
			time.Sleep(wait)
		}

		release, retryable, err := doFetchRelease(endpoint)
		if err == nil {
			return release, nil
		}
		lastErr = err
		if !retryable {
			return nil, lastErr
		}
	}
	return nil, fmt.Errorf("请求发布信息失败（已重试 %d 次）: %w", releaseMaxRetries, lastErr)
}

// doFetchRelease 执行单次 GitHub API 请求，返回结果、是否可重试、错误
func doFetchRelease(endpoint string) (*githubRelease, bool, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, false, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		// 网络错误（超时、DNS、连接拒绝等）可重试
		return nil, true, fmt.Errorf("请求发布信息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= http.StatusInternalServerError
		return nil, retryable, fmt.Errorf("GitHub API 状态码异常: %d", resp.StatusCode)
	}

	if strings.Contains(endpoint, "/releases?") {
		var list []githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, false, fmt.Errorf("解析发布列表失败: %w", err)
		}
		if len(list) == 0 {
			return nil, false, fmt.Errorf("发布列表为空")
		}
		return &list[0], false, nil
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("解析发布信息失败: %w", err)
	}
	return &release, false, nil
}

func convertGithubRelease(release *githubRelease) *AgentReleaseInfo {
	if release == nil {
		return nil
	}

	version := strings.TrimSpace(release.TagName)
	if version == "" {
		version = release.Name
	}
	version = strings.TrimPrefix(version, "v")

	info := &AgentReleaseInfo{
		Version:     version,
		Name:        release.Name,
		Notes:       release.Body,
		PublishedAt: release.PublishedAt,
	}

	for _, asset := range release.Assets {
		osName, archName := parsePlatformFromName(asset.Name)
		info.Assets = append(info.Assets, ReleaseAsset{
			Name:        asset.Name,
			DownloadURL: asset.BrowserDownloadURL,
			Size:        asset.Size,
			OS:          osName,
			Arch:        archName,
		})
	}

	return info
}

func parsePlatformFromName(name string) (string, string) {
	nameLower := strings.ToLower(name)
	var osName, archName string

	for _, candidate := range []string{"linux", "windows", "darwin", "mac", "freebsd"} {
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

func applyDownloadMirror(info *AgentReleaseInfo, mirror string) *AgentReleaseInfo {
	if info == nil {
		return nil
	}
	if mirror == "" {
		return info
	}

	result := cloneRelease(info)
	prefix := "https://github.com"
	mirror = strings.TrimRight(mirror, "/")

	for i := range result.Assets {
		if strings.HasPrefix(result.Assets[i].DownloadURL, prefix) {
			result.Assets[i].DownloadURL = mirror + strings.TrimPrefix(result.Assets[i].DownloadURL, prefix)
		}
	}
	return result
}
