package upgrader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type failingReadCloser struct {
	read bool
}

func (reader *failingReadCloser) Read(buffer []byte) (int, error) {
	if !reader.read {
		reader.read = true
		copy(buffer, "partial")
		return len("partial"), nil
	}
	return 0, errors.New("stream interrupted")
}

func (*failingReadCloser) Close() error { return nil }

func TestDownloadEnforcesDeclaredAndActualSizeBoundaries(t *testing.T) {
	payload := []byte("12345678")
	sha := sha256.Sum256(payload)
	validSHA := hex.EncodeToString(sha[:])

	tests := []struct {
		name          string
		assetSize     int64
		maxBytes      int64
		contentLength int64
		body          io.ReadCloser
		sha256        string
		wantCode      string
	}{
		{
			name:          "declared asset exceeds limit",
			assetSize:     9,
			maxBytes:      8,
			contentLength: -1,
			body:          io.NopCloser(bytes.NewReader(payload)),
			sha256:        validSHA,
			wantCode:      "download_too_large",
		},
		{
			name:          "content length exceeds limit",
			assetSize:     8,
			maxBytes:      8,
			contentLength: 9,
			body:          io.NopCloser(bytes.NewReader(append(payload, '9'))),
			sha256:        validSHA,
			wantCode:      "download_too_large",
		},
		{
			name:          "content length mismatches declaration",
			assetSize:     8,
			maxBytes:      16,
			contentLength: 7,
			body:          io.NopCloser(bytes.NewReader(payload[:7])),
			sha256:        validSHA,
			wantCode:      "download_size_mismatch",
		},
		{
			name:          "chunked body exceeds limit",
			assetSize:     8,
			maxBytes:      8,
			contentLength: -1,
			body:          io.NopCloser(bytes.NewReader(append(payload, '9'))),
			sha256:        validSHA,
			wantCode:      "download_too_large",
		},
		{
			name:          "actual size mismatches declaration",
			assetSize:     9,
			maxBytes:      16,
			contentLength: -1,
			body:          io.NopCloser(bytes.NewReader(payload)),
			sha256:        validSHA,
			wantCode:      "download_size_mismatch",
		},
		{
			name:          "sha mismatch",
			assetSize:     8,
			maxBytes:      16,
			contentLength: -1,
			body:          io.NopCloser(bytes.NewReader(payload)),
			sha256:        strings.Repeat("b", 64),
			wantCode:      "download_sha_mismatch",
		},
		{
			name:          "mid stream failure",
			assetSize:     8,
			maxBytes:      16,
			contentLength: -1,
			body:          &failingReadCloser{},
			sha256:        validSHA,
			wantCode:      "download_stream_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Status:        "200 OK",
					Body:          tt.body,
					ContentLength: tt.contentLength,
					Header:        make(http.Header),
				}, nil
			})}
			destination := filepath.Join(t.TempDir(), "download")
			_, _, err := downloadFileSHA256(context.Background(), client, UpgradeRequest{
				RequestID:        "job-request-id",
				TargetVersion:    "1.4.0",
				AssetSize:        tt.assetSize,
				MaxDownloadBytes: tt.maxBytes,
				DownloadURL:      "https://secret.example/releases/agent?ticket=hidden",
				SHA256:           tt.sha256,
			}, destination, nil)
			assert.Equal(t, tt.wantCode, UpgradeErrorCode(err))
			assert.NotContains(t, err.Error(), "https://secret.example")
			assert.NotContains(t, err.Error(), tt.sha256)
			_, statErr := os.Stat(destination)
			assert.True(t, errors.Is(statErr, os.ErrNotExist))
		})
	}
}

type fakeInspector struct {
	result SelfTestResult
	err    error
}

func (inspector fakeInspector) Inspect(context.Context, string) (SelfTestResult, error) {
	return inspector.result, inspector.err
}

type recordingApplyOps struct {
	calls []string
}

func (ops *recordingApplyOps) Backup(string, string) error {
	ops.calls = append(ops.calls, "backup")
	return nil
}
func (ops *recordingApplyOps) Replace(string, string) error {
	ops.calls = append(ops.calls, "replace")
	return nil
}
func (ops *recordingApplyOps) Restore(string, string) error {
	ops.calls = append(ops.calls, "restore")
	return nil
}
func (ops *recordingApplyOps) Exec(string, []string, []string) error {
	ops.calls = append(ops.calls, "exec")
	return nil
}

func TestInspectRejectsEveryIdentityMismatchBeforeApply(t *testing.T) {
	payload := []byte("new agent binary")
	digest := sha256.Sum256(payload)
	assetName := canonicalAgentAssetName("1.4.0", "monitor")
	matching := SelfTestResult{Version: "1.4.0", AgentType: "monitor", OS: runtime.GOOS, Arch: runtime.GOARCH}

	tests := map[string]func(*SelfTestResult){
		"version":    func(result *SelfTestResult) { result.Version = "1.4.1" },
		"agent type": func(result *SelfTestResult) { result.AgentType = "full" },
		"os":         func(result *SelfTestResult) { result.OS = "unexpected-os" },
		"arch":       func(result *SelfTestResult) { result.Arch = "unexpected-arch" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := matching
			mutate(&result)
			ops := &recordingApplyOps{}
			current := filepath.Join(t.TempDir(), "agent")
			require.NoError(t, os.WriteFile(current, []byte("old"), 0o755))
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Status:        "200 OK",
					Body:          io.NopCloser(bytes.NewReader(payload)),
					ContentLength: int64(len(payload)),
					Header:        make(http.Header),
				}, nil
			})}
			err := Upgrade(context.Background(), UpgradeRequest{
				RequestID:        "job-request-id",
				TargetVersion:    "1.4.0",
				TargetAgentType:  "monitor",
				AssetName:        assetName,
				AssetSize:        int64(len(payload)),
				MaxDownloadBytes: 1024,
				DownloadURL:      "https://downloads.example/" + assetName,
				SHA256:           hex.EncodeToString(digest[:]),
				MarkerPath:       filepath.Join(filepath.Dir(current), "upgrade-marker.json"),
				ExecutablePath:   current,
				HTTPClient:       client,
				Inspector:        fakeInspector{result: result},
				ApplyOps:         ops,
			}, nil)
			assert.Equal(t, "self_test_identity_mismatch", UpgradeErrorCode(err))
			assert.Empty(t, ops.calls)
		})
	}
}
