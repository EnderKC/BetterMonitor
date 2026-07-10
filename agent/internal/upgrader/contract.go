package upgrader

import (
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	agentversion "github.com/user/server-ops-agent/pkg/version"
)

const DefaultMaxDownloadBytes int64 = 256 << 20

type UpgradeInstruction struct {
	RequestID       string `json:"request_id"`
	TargetVersion   string `json:"target_version"`
	TargetAgentType string `json:"target_agent_type"`
	AssetName       string `json:"asset_name"`
	AssetSize       int64  `json:"asset_size"`
	DownloadURL     string `json:"download_url"`
	SHA256          string `json:"sha256"`
}

func ValidateUpgradeInstruction(input UpgradeInstruction) error {
	if strings.TrimSpace(input.RequestID) == "" {
		return fmt.Errorf("upgrade request id is required")
	}
	version := strings.TrimSpace(input.TargetVersion)
	if !isStrictSemVersion(version) {
		return fmt.Errorf("target version must be strict semver")
	}
	agentType := strings.ToLower(strings.TrimSpace(input.TargetAgentType))
	if agentType != "full" && agentType != "monitor" {
		return fmt.Errorf("target agent type must be full or monitor")
	}
	wantAsset := canonicalAgentAssetName(version, agentType)
	if strings.TrimSpace(input.AssetName) != wantAsset {
		return fmt.Errorf("asset name does not match target identity")
	}
	if input.AssetSize <= 0 || input.AssetSize > DefaultMaxDownloadBytes {
		return fmt.Errorf("asset size is outside the allowed range")
	}
	parsedURL, err := url.Parse(strings.TrimSpace(input.DownloadURL))
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return fmt.Errorf("download url must be credential-free https")
	}
	if normalizeSHA256(input.SHA256) == "" {
		return fmt.Errorf("sha256 must be 64 hexadecimal characters")
	}
	return nil
}

func IsUpgradeNoop(input UpgradeInstruction) bool {
	return strings.TrimSpace(input.TargetVersion) == strings.TrimSpace(agentversion.Version) &&
		strings.EqualFold(strings.TrimSpace(input.TargetAgentType), strings.TrimSpace(agentversion.AgentType))
}

func canonicalAgentAssetName(version, agentType string) string {
	base := "better-monitor-agent"
	if agentType == "monitor" {
		base += "-monitor"
	}
	name := fmt.Sprintf("%s-%s-%s-%s", base, version, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func isStrictSemVersion(raw string) bool {
	if raw == "" || strings.HasPrefix(raw, "v") || strings.Count(raw, "+") > 1 {
		return false
	}
	coreAndBuild := strings.SplitN(raw, "+", 2)
	if len(coreAndBuild) == 2 && !validSemVersionIdentifiers(coreAndBuild[1], false) {
		return false
	}
	coreAndPrerelease := strings.SplitN(coreAndBuild[0], "-", 2)
	if len(coreAndPrerelease) == 2 && !validSemVersionIdentifiers(coreAndPrerelease[1], true) {
		return false
	}
	core := strings.Split(coreAndPrerelease[0], ".")
	if len(core) != 3 {
		return false
	}
	for _, component := range core {
		if !validSemVersionNumber(component) {
			return false
		}
	}
	return true
}

func validSemVersionNumber(raw string) bool {
	if raw == "" || (len(raw) > 1 && raw[0] == '0') {
		return false
	}
	_, err := strconv.ParseUint(raw, 10, 64)
	return err == nil
}

func validSemVersionIdentifiers(raw string, rejectNumericLeadingZero bool) bool {
	if raw == "" {
		return false
	}
	for _, identifier := range strings.Split(raw, ".") {
		if identifier == "" {
			return false
		}
		numeric := true
		for _, char := range identifier {
			if char < '0' || char > '9' {
				numeric = false
			}
			if (char >= '0' && char <= '9') || (char >= 'A' && char <= 'Z') ||
				(char >= 'a' && char <= 'z') || char == '-' {
				continue
			}
			return false
		}
		if rejectNumericLeadingZero && numeric && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}
