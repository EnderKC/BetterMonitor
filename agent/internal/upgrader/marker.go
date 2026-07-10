package upgrader

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxUpgradeMarkerBytes = 64 << 10

type UpgradeMarker struct {
	RequestID       string `json:"request_id"`
	TargetVersion   string `json:"target_version"`
	TargetAgentType string `json:"target_agent_type"`
	Outcome         string `json:"outcome"`
	ErrorCode       string `json:"error_code"`
}

func DefaultMarkerPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve upgrade marker path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil && resolved != "" {
		executable = resolved
	}
	return executable + ".upgrade-marker.json", nil
}

func LoadUpgradeMarker(path string) (*UpgradeMarker, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("upgrade marker path is required")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open upgrade marker: %w", err)
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxUpgradeMarkerBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read upgrade marker: %w", err)
	}
	if len(raw) > maxUpgradeMarkerBytes {
		return nil, fmt.Errorf("upgrade marker is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var marker UpgradeMarker
	if err := decoder.Decode(&marker); err != nil {
		return nil, fmt.Errorf("decode upgrade marker: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := validateUpgradeMarker(marker); err != nil {
		return nil, err
	}
	return &marker, nil
}

func WriteUpgradeMarker(path string, marker UpgradeMarker) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("upgrade marker path is required")
	}
	if err := validateUpgradeMarker(marker); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create upgrade marker directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create upgrade marker temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set upgrade marker permissions: %w", err)
	}
	encoder := json.NewEncoder(temp)
	if err := encoder.Encode(marker); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode upgrade marker: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync upgrade marker: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close upgrade marker: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace upgrade marker: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func ClearUpgradeMarker(path string) error {
	err := os.Remove(strings.TrimSpace(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("clear upgrade marker: %w", err)
	}
	return nil
}

func validateUpgradeMarker(marker UpgradeMarker) error {
	if strings.TrimSpace(marker.RequestID) == "" || !isStrictSemVersion(strings.TrimSpace(marker.TargetVersion)) {
		return fmt.Errorf("upgrade marker identity is invalid")
	}
	agentType := strings.ToLower(strings.TrimSpace(marker.TargetAgentType))
	if agentType != "full" && agentType != "monitor" {
		return fmt.Errorf("upgrade marker agent type is invalid")
	}
	switch strings.ToLower(strings.TrimSpace(marker.Outcome)) {
	case "applied", "rolled_back", "failed":
	default:
		return fmt.Errorf("upgrade marker outcome is invalid")
	}
	if marker.ErrorCode != "" && !isStableErrorCode(marker.ErrorCode) {
		return fmt.Errorf("upgrade marker error code is invalid")
	}
	return nil
}

func isStableErrorCode(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64 {
		return false
	}
	for _, char := range raw {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("upgrade marker contains trailing data")
		}
		return fmt.Errorf("decode upgrade marker trailing data: %w", err)
	}
	return nil
}
