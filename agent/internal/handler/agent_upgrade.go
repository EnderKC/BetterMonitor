package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/user/server-ops-agent/internal/upgrader"
	agentversion "github.com/user/server-ops-agent/pkg/version"
)

type agentUpgradePayload struct {
	Action        string `json:"action"`
	TargetVersion string `json:"target_version"`
	Channel       string `json:"channel"`
	ServerID      uint   `json:"server_id"`

	// 可选：若面板端愿意直接提供下载信息，Agent 就不需要自行拼接/推断 URL
	DownloadURL string `json:"download_url,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// HandleAgentUpgradeMessage 处理面板端下发的 agent_upgrade 消息（type/payload 格式）
func HandleAgentUpgradeMessage(c *websocket.Conn, serverID uint, secretKey string, requestID string, payload json.RawMessage) {
	if strings.TrimSpace(requestID) == "" {
		requestID = fmt.Sprintf("upgrade-%d-%d", serverID, time.Now().Unix())
	}

	sendAgentUpgradeStatus(c, requestID, "received", "收到升级指令", map[string]interface{}{
		"platform": runtime.GOOS,
		"arch":     runtime.GOARCH,
	})

	var p agentUpgradePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		sendAgentUpgradeStatus(c, requestID, "failed", fmt.Sprintf("解析升级 payload 失败: %v", err), nil)
		return
	}

	if p.ServerID == 0 {
		p.ServerID = serverID
	}
	if strings.TrimSpace(p.Action) == "" {
		p.Action = "upgrade"
	}
	if p.Action != "upgrade" {
		sendAgentUpgradeStatus(c, requestID, "failed", fmt.Sprintf("不支持的升级动作: %s", p.Action), nil)
		return
	}
	if strings.TrimSpace(p.TargetVersion) == "" {
		sendAgentUpgradeStatus(c, requestID, "failed", "缺少 target_version", nil)
		return
	}
	if strings.TrimSpace(p.Channel) == "" {
		p.Channel = "stable"
	}

	current := agentversion.GetVersion()
	if current != nil && strings.TrimSpace(current.Version) != "" && strings.TrimSpace(current.Version) == strings.TrimSpace(p.TargetVersion) {
		sendAgentUpgradeStatus(c, requestID, "noop", "当前版本已是目标版本，无需升级", map[string]interface{}{
			"current_version": current.Version,
			"target_version":  p.TargetVersion,
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	req := upgrader.UpgradeRequest{
		RequestID:     requestID,
		TargetVersion: strings.TrimSpace(p.TargetVersion),
		Channel:       strings.TrimSpace(p.Channel),
		DownloadURL:   strings.TrimSpace(p.DownloadURL),
		SHA256:        strings.TrimSpace(p.SHA256),
		ServerID:      p.ServerID,
		SecretKey:     secretKey,
		Args:          osArgs(),
		Env:           osEnviron(),
	}

	sendAgentUpgradeStatus(c, requestID, "starting", "开始执行升级流程", map[string]interface{}{
		"current_version": safeVersion(current),
		"target_version":  req.TargetVersion,
		"channel":         req.Channel,
	})

	err := upgrader.Upgrade(ctx, req, func(pr upgrader.Progress) {
		fields := map[string]interface{}{
			"current_version": safeVersion(agentversion.GetVersion()),
			"target_version":  req.TargetVersion,
			"channel":         req.Channel,
		}
		if pr.DownloadURL != "" {
			fields["download_url"] = pr.DownloadURL
		}
		if pr.SHA256 != "" {
			fields["sha256"] = pr.SHA256
		}
		if pr.BytesDownloaded > 0 {
			fields["bytes_downloaded"] = pr.BytesDownloaded
		}
		sendAgentUpgradeStatus(c, requestID, pr.Status, pr.Message, fields)
	})
	if err != nil {
		sendAgentUpgradeStatus(c, requestID, "failed", fmt.Sprintf("升级失败: %v", err), nil)
		return
	}

	// 通常情况下 Upgrade 在成功时会直接触发进程重启（Unix: exec；Windows: 退出后由 updater 拉起），不会返回到这里
	sendAgentUpgradeStatus(c, requestID, "success", "升级流程完成", nil)
}

func safeVersion(info *agentversion.Info) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.Version)
}

func sendAgentUpgradeStatus(c *websocket.Conn, requestID, status, message string, extra map[string]interface{}) {
	info := agentversion.GetVersion()

	payload := map[string]interface{}{
		"status":  status,
		"message": message,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"agent": map[string]interface{}{
			"version":    safeVersion(info),
			"commit":     strings.TrimSpace(info.Commit),
			"build_date": strings.TrimSpace(info.BuildDate),
			"go_version": strings.TrimSpace(info.GoVersion),
			"platform":   strings.TrimSpace(info.Platform),
			"arch":       strings.TrimSpace(info.Arch),
		},
	}
	for k, v := range extra {
		payload[k] = v
	}

	msg := map[string]interface{}{
		"type":       "agent_upgrade_status",
		"request_id": requestID,
		"payload":    payload,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.WriteMessage(websocket.TextMessage, b)
}
