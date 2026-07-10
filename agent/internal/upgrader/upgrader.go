package upgrader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxSelfTestOutputBytes = 64 << 10

type SelfTestResult struct {
	Version   string `json:"version"`
	AgentType string `json:"agent_type"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type BinaryInspector interface {
	Inspect(ctx context.Context, path string) (SelfTestResult, error)
}

type UpgradeRequest struct {
	RequestID string

	TargetVersion     string
	TargetAgentType   string
	AssetName         string
	AssetSize         int64
	MaxDownloadBytes  int64
	MarkerPath        string
	ScheduledTaskName string
	DownloadURL       string
	SHA256            string

	Args []string
	Env  []string

	ExecutablePath string
	HTTPClient     *http.Client
	Inspector      BinaryInspector
	ApplyOps       ApplyOps
}

type Progress struct {
	RequestID       string
	Status          string
	Message         string
	TargetVersion   string
	BytesDownloaded int64
	ErrorCode       string
	Time            time.Time
}

type ProgressFunc func(Progress)

func Upgrade(ctx context.Context, req UpgradeRequest, report ProgressFunc) error {
	if ctx == nil {
		return newUpgradeError("upgrade_context_missing")
	}
	if report == nil {
		report = func(Progress) {}
	}
	if req.MaxDownloadBytes <= 0 {
		req.MaxDownloadBytes = DefaultMaxDownloadBytes
	}
	instruction := UpgradeInstruction{
		RequestID:       strings.TrimSpace(req.RequestID),
		TargetVersion:   strings.TrimSpace(req.TargetVersion),
		TargetAgentType: strings.ToLower(strings.TrimSpace(req.TargetAgentType)),
		AssetName:       strings.TrimSpace(req.AssetName),
		AssetSize:       req.AssetSize,
		DownloadURL:     strings.TrimSpace(req.DownloadURL),
		SHA256:          strings.ToLower(strings.TrimSpace(req.SHA256)),
	}
	if err := ValidateUpgradeInstruction(instruction); err != nil {
		return newUpgradeError("upgrade_instruction_invalid")
	}
	if instruction.AssetSize > req.MaxDownloadBytes {
		return newUpgradeError("download_too_large")
	}
	req.RequestID = instruction.RequestID
	req.TargetVersion = instruction.TargetVersion
	req.TargetAgentType = instruction.TargetAgentType
	req.AssetName = instruction.AssetName
	req.DownloadURL = instruction.DownloadURL
	req.SHA256 = instruction.SHA256
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
		Status:        "downloading",
		Message:       "downloading upgrade asset",
		TargetVersion: req.TargetVersion,
		Time:          time.Now().UTC(),
	})

	exePath := strings.TrimSpace(req.ExecutablePath)
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return newUpgradeError("executable_path_unavailable")
		}
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil && resolved != "" {
			exePath = resolved
		}
	}
	tempFile, err := os.CreateTemp(filepath.Dir(exePath), filepath.Base(exePath)+".download-*")
	if err != nil {
		return newUpgradeError("download_temp_create_failed")
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return newUpgradeError("download_temp_close_failed")
	}

	_, bytesDownloaded, err := downloadFileSHA256(ctx, client, req, tempPath, report)
	if err != nil {
		return err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	report(Progress{
		RequestID:       req.RequestID,
		Status:          "verifying",
		Message:         "verifying upgrade asset",
		TargetVersion:   req.TargetVersion,
		BytesDownloaded: bytesDownloaded,
		Time:            time.Now().UTC(),
	})
	if current, statErr := os.Stat(exePath); statErr == nil {
		if err := os.Chmod(tempPath, current.Mode()); err != nil {
			return newUpgradeError("download_permission_failed")
		}
	} else if err := os.Chmod(tempPath, 0o755); err != nil {
		return newUpgradeError("download_permission_failed")
	}

	inspector := req.Inspector
	if inspector == nil {
		inspector = commandBinaryInspector{}
	}
	identity, err := inspector.Inspect(ctx, tempPath)
	if err != nil {
		return newUpgradeError("self_test_failed")
	}
	if strings.TrimSpace(identity.Version) != req.TargetVersion ||
		!strings.EqualFold(strings.TrimSpace(identity.AgentType), req.TargetAgentType) ||
		strings.TrimSpace(identity.OS) != runtime.GOOS ||
		strings.TrimSpace(identity.Arch) != runtime.GOARCH {
		return newUpgradeError("self_test_identity_mismatch")
	}
	if err := persistUpgradeOutcome(req, "applied", ""); err != nil {
		return err
	}
	report(Progress{
		RequestID:     req.RequestID,
		Status:        "applying",
		Message:       "applying upgrade asset",
		TargetVersion: req.TargetVersion,
		Time:          time.Now().UTC(),
	})

	cleanupTemp = runtime.GOOS != "windows"
	return applyAndRestart(ctx, req, exePath, tempPath, report)
}

func downloadFileSHA256(
	ctx context.Context,
	client *http.Client,
	req UpgradeRequest,
	dstPath string,
	report ProgressFunc,
) (shaHex string, bytesDownloaded int64, err error) {
	maxBytes := req.MaxDownloadBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDownloadBytes
	}
	if req.AssetSize <= 0 {
		return "", 0, newUpgradeError("download_size_invalid")
	}
	if req.AssetSize > maxBytes {
		return "", 0, newUpgradeError("download_too_large")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, req.DownloadURL, nil)
	if err != nil {
		return "", 0, newUpgradeError("download_request_invalid")
	}
	httpRequest.Header.Set("User-Agent", "better-monitor-agent-upgrader")
	response, err := client.Do(httpRequest)
	if err != nil {
		return "", 0, newUpgradeError("download_request_failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", 0, newUpgradeError("download_http_status")
	}
	if response.ContentLength >= 0 {
		if response.ContentLength > maxBytes {
			return "", 0, newUpgradeError("download_too_large")
		}
		if response.ContentLength != req.AssetSize {
			return "", 0, newUpgradeError("download_size_mismatch")
		}
	}

	file, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", 0, newUpgradeError("download_file_open_failed")
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = os.Remove(dstPath)
		}
	}()

	hash := sha256.New()
	limited := &io.LimitedReader{R: response.Body, N: maxBytes + 1}
	reader := &progressReader{
		reader: limited,
		onProgress: func(total int64) {
			if report != nil {
				report(Progress{
					RequestID:       req.RequestID,
					Status:          "downloading",
					Message:         "downloading upgrade asset",
					TargetVersion:   req.TargetVersion,
					BytesDownloaded: total,
					Time:            time.Now().UTC(),
				})
			}
		},
		interval: 2 * time.Second,
	}
	written, copyErr := io.Copy(io.MultiWriter(file, hash), reader)
	if copyErr != nil {
		return "", written, newUpgradeError("download_stream_failed")
	}
	if written > maxBytes {
		return "", written, newUpgradeError("download_too_large")
	}
	if written != req.AssetSize {
		return "", written, newUpgradeError("download_size_mismatch")
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualSHA, normalizeSHA256(req.SHA256)) {
		return "", written, newUpgradeError("download_sha_mismatch")
	}
	if err := file.Sync(); err != nil {
		return "", written, newUpgradeError("download_file_sync_failed")
	}
	if err := file.Close(); err != nil {
		return "", written, newUpgradeError("download_file_close_failed")
	}
	success = true
	return actualSHA, written, nil
}

type commandBinaryInspector struct{}

func (commandBinaryInspector) Inspect(ctx context.Context, path string) (SelfTestResult, error) {
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(inspectCtx, path, "--self-test")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return SelfTestResult{}, err
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return SelfTestResult{}, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(stdout, maxSelfTestOutputBytes+1))
	waitErr := command.Wait()
	if readErr != nil {
		return SelfTestResult{}, readErr
	}
	if waitErr != nil {
		return SelfTestResult{}, waitErr
	}
	if len(raw) > maxSelfTestOutputBytes {
		return SelfTestResult{}, errors.New("self-test output too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result SelfTestResult
	if err := decoder.Decode(&result); err != nil {
		return SelfTestResult{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return SelfTestResult{}, err
	}
	return result, nil
}

type progressReader struct {
	reader     io.Reader
	onProgress func(int64)
	interval   time.Duration
	total      int64
	lastReport time.Time
}

func (reader *progressReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.total += int64(count)
	if reader.onProgress != nil && time.Since(reader.lastReport) >= reader.interval {
		reader.onProgress(reader.total)
		reader.lastReport = time.Now()
	}
	return count, err
}

func normalizeSHA256(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.ToLower(raw), "sha256:")
	raw = strings.TrimSpace(raw)
	if len(raw) != 64 {
		return ""
	}
	for _, char := range raw {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return ""
	}
	return raw
}
