package upgrader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type UpgradeRequest struct {
	RequestID string

	TargetVersion string
	Channel       string

	// 推荐：由面板端直接提供 URL 与 SHA256，Agent 只负责执行升级动作
	DownloadURL string
	SHA256      string

	// 可选：若下载地址是面板端的受保护接口，可用于鉴权 header
	ServerID  uint
	SecretKey string

	Args []string
	Env  []string

	HTTPClient *http.Client
}

type Progress struct {
	RequestID string
	Status    string
	Message   string

	TargetVersion   string
	DownloadURL     string
	SHA256          string
	BytesDownloaded int64
	Time            time.Time
}

type ProgressFunc func(Progress)

func Upgrade(ctx context.Context, req UpgradeRequest, report ProgressFunc) error {
	if report == nil {
		report = func(Progress) {}
	}

	req.TargetVersion = strings.TrimSpace(req.TargetVersion)
	req.Channel = strings.TrimSpace(req.Channel)
	req.DownloadURL = strings.TrimSpace(req.DownloadURL)
	req.SHA256 = strings.TrimSpace(req.SHA256)

	if req.TargetVersion == "" {
		return errors.New("missing target_version")
	}
	if req.Channel == "" {
		req.Channel = "stable"
	}
	if len(req.Args) == 0 {
		req.Args = os.Args
	}
	if req.Env == nil {
		req.Env = os.Environ()
	}

	client := req.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Minute}
	}

	report(Progress{
		RequestID:     req.RequestID,
		Status:        "resolving",
		Message:       "解析下载地址",
		TargetVersion: req.TargetVersion,
		Time:          time.Now().UTC(),
	})

	downloadURL, err := resolveDownloadURL(req)
	if err != nil {
		return err
	}
	req.DownloadURL = downloadURL

	report(Progress{
		RequestID:     req.RequestID,
		Status:        "downloading",
		Message:       "下载新版本二进制",
		TargetVersion: req.TargetVersion,
		DownloadURL:   req.DownloadURL,
		Time:          time.Now().UTC(),
	})

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil && resolved != "" {
		exePath = resolved
	}

	downloadDir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(downloadDir, filepath.Base(exePath)+".download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// 注意：Windows 下升级需要让外部 updater 使用 tmpPath 完成替换与重启，不能在这里 defer remove
	_ = tmpFile.Close()

	actualSHA, bytesDownloaded, err := downloadFileSHA256(ctx, client, req, tmpPath, report)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	report(Progress{
		RequestID:       req.RequestID,
		Status:          "verifying",
		Message:         "校验 SHA256",
		TargetVersion:   req.TargetVersion,
		DownloadURL:     req.DownloadURL,
		SHA256:          req.SHA256,
		BytesDownloaded: bytesDownloaded,
		Time:            time.Now().UTC(),
	})

	expected := normalizeSHA256(req.SHA256)
	if expected == "" {
		_ = os.Remove(tmpPath)
		return errors.New("missing or invalid sha256")
	}
	if !strings.EqualFold(expected, actualSHA) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sha256 mismatch: expected=%s actual=%s", expected, actualSHA)
	}

	// 继承原二进制的权限（Unix）
	if st, err := os.Stat(exePath); err == nil {
		_ = os.Chmod(tmpPath, st.Mode())
	} else {
		_ = os.Chmod(tmpPath, 0755)
	}

	report(Progress{
		RequestID:     req.RequestID,
		Status:        "applying",
		Message:       "原子替换并重启",
		TargetVersion: req.TargetVersion,
		DownloadURL:   req.DownloadURL,
		Time:          time.Now().UTC(),
	})

	return applyAndRestart(ctx, req, exePath, tmpPath, report)
}

func resolveDownloadURL(req UpgradeRequest) (string, error) {
	if req.DownloadURL != "" {
		return req.DownloadURL, nil
	}

	// 1) 显式模板：BETTER_MONITOR_AGENT_UPGRADE_URL_TEMPLATE
	//    例：https://github.com/user/server-ops-backend/releases/download/v{version}/better-monitor-agent-{version}-{os}-{arch}
	if tpl := strings.TrimSpace(os.Getenv("BETTER_MONITOR_AGENT_UPGRADE_URL_TEMPLATE")); tpl != "" {
		return applyURLTemplate(tpl, req.TargetVersion, req.Channel, runtime.GOOS, runtime.GOARCH), nil
	}

	// 2) GitHub Repo：BETTER_MONITOR_AGENT_GITHUB_REPO=user/server-ops-backend
	//    默认按 GitHub Releases 约定拼 URL
	if repo := strings.TrimSpace(os.Getenv("BETTER_MONITOR_AGENT_GITHUB_REPO")); repo != "" {
		versionTag := req.TargetVersion
		if !strings.HasPrefix(versionTag, "v") {
			versionTag = "v" + versionTag
		}
		name := fmt.Sprintf("better-monitor-agent-%s-%s-%s", req.TargetVersion, runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
			name += ".exe"
		}
		return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", strings.TrimSuffix(repo, "/"), versionTag, name), nil
	}

	return "", errors.New("missing download_url; set BETTER_MONITOR_AGENT_UPGRADE_URL_TEMPLATE or BETTER_MONITOR_AGENT_GITHUB_REPO, or have panel include payload.download_url")
}

func applyURLTemplate(tpl, version, channel, goos, arch string) string {
	out := tpl
	out = strings.ReplaceAll(out, "{version}", version)
	out = strings.ReplaceAll(out, "{channel}", channel)
	out = strings.ReplaceAll(out, "{os}", goos)
	out = strings.ReplaceAll(out, "{arch}", arch)
	return out
}

func downloadFileSHA256(ctx context.Context, client *http.Client, req UpgradeRequest, dstPath string, report ProgressFunc) (shaHex string, bytesDownloaded int64, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.DownloadURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create download request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "better-monitor-agent-upgrader")
	if req.SecretKey != "" {
		// 可选鉴权 header（后端是否使用由实现决定）
		httpReq.Header.Set("X-Secret-Key", req.SecretKey)
	}
	if req.ServerID != 0 {
		httpReq.Header.Set("X-Server-ID", fmt.Sprintf("%d", req.ServerID))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return "", 0, fmt.Errorf("download failed: status=%s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}

	f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", 0, fmt.Errorf("open temp file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	w := io.MultiWriter(f, h)

	// 定期上报下载进度
	reader := &progressReader{
		reader: resp.Body,
		onProgress: func(n int64) {
			if report != nil {
				report(Progress{
					RequestID:       req.RequestID,
					Status:          "downloading",
					Message:         fmt.Sprintf("已下载 %d 字节", n),
					TargetVersion:   req.TargetVersion,
					DownloadURL:     req.DownloadURL,
					BytesDownloaded: n,
					Time:            time.Now().UTC(),
				})
			}
		},
		interval: 2 * time.Second,
	}

	n, err := io.Copy(w, reader)
	if err != nil {
		return "", n, fmt.Errorf("write temp file: %w", err)
	}
	_ = f.Sync()

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// progressReader 包装 io.Reader 以定期报告进度
type progressReader struct {
	reader     io.Reader
	onProgress func(int64)
	interval   time.Duration

	total      int64
	lastReport time.Time
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	pr.total += int64(n)

	if pr.onProgress != nil && time.Since(pr.lastReport) >= pr.interval {
		pr.onProgress(pr.total)
		pr.lastReport = time.Now()
	}

	return n, err
}

func normalizeSHA256(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToLower(s), "sha256:")
	s = strings.TrimSpace(s)
	if len(s) != 64 {
		return ""
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return ""
	}
	return s
}
