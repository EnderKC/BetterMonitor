package services

import (
	"strings"

	"github.com/user/server-ops-backend/models"
)

// NormalizeArch 将系统报告的内核架构名称归一化为 Go 标准命名
// 例如 x86_64 → amd64, aarch64 → arm64
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

// FindMatchingAsset 根据服务器的 OS、架构和 Agent 类型查找匹配的 release asset
func FindMatchingAsset(assets []ReleaseAsset, serverOS, serverArch, agentType string) *ReleaseAsset {
	osKey := strings.ToLower(strings.TrimSpace(serverOS))
	archKey := NormalizeArch(serverArch)
	wantsMonitor := strings.EqualFold(strings.TrimSpace(agentType), "monitor")

	for i := range assets {
		assetOS := strings.ToLower(assets[i].OS)
		assetArch := strings.ToLower(assets[i].Arch)

		if assetOS != osKey || assetArch != archKey {
			continue
		}

		// 区分 full / monitor 变体
		// 命名约定: full = "better-monitor-agent-{ver}-..." / monitor = "better-monitor-agent-monitor-{ver}-..."
		// 使用 "-agent-monitor-" 精确匹配，避免 "better-monitor-agent-..." 中的 "-monitor-" 误匹配
		nameLower := strings.ToLower(assets[i].Name)
		isMonitorAsset := strings.Contains(nameLower, "-agent-monitor-")
		if wantsMonitor != isMonitorAsset {
			continue
		}

		return &assets[i]
	}
	return nil
}

// BuildUpgradePayload 根据服务器信息和 release 数据构建完整的升级指令 payload
// 当 releaseInfo 可用时，会匹配对应平台的 download_url 和 sha256
func BuildUpgradePayload(
	server *models.Server,
	targetVersion, channel string,
	releaseInfo *AgentReleaseInfo,
	targetAgentType string,
) map[string]interface{} {
	payload := map[string]interface{}{
		"action":         "upgrade",
		"target_version": targetVersion,
		"channel":        channel,
		"server_id":      server.ID,
	}

	// 确定目标 agent 类型
	agentType := strings.TrimSpace(targetAgentType)
	if agentType == "" {
		agentType = server.AgentType
	}
	if agentType == "" {
		agentType = "full"
	}
	payload["target_agent_type"] = agentType

	// 尝试匹配 release asset 以提供 download_url 和 sha256
	if releaseInfo != nil {
		asset := FindMatchingAsset(releaseInfo.Assets, server.OS, server.Arch, agentType)
		if asset != nil {
			payload["download_url"] = asset.DownloadURL
			if asset.SHA256 != "" {
				payload["sha256"] = asset.SHA256
			}
		}
	}

	return payload
}
