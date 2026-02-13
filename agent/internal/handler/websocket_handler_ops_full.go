//go:build !monitor_only

package handler

import (
	"fmt"
	"strings"

	"github.com/user/server-ops-agent/internal/monitor"
)

// handleOperationAction 全功能版：分发操作类命令到对应的 handler
func handleOperationAction(action string, params map[string]interface{}) (string, error) {
	switch {
	case strings.HasPrefix(action, "file_"):
		return monitor.HandleFileCommand(action, params)
	case strings.HasPrefix(action, "process_"):
		return monitor.HandleProcessCommand(action, params)
	case strings.HasPrefix(action, "docker_"):
		return monitor.HandleDockerCommand(action, params)
	case strings.HasPrefix(action, "nginx_"):
		return monitor.HandleNginxCommand(action, params)
	case action == "terminal_create":
		return monitor.CreateTerminalSession(params)
	case action == "terminal_resize":
		return monitor.ResizeTerminal(params)
	case action == "terminal_input":
		return monitor.SendTerminalInput(params)
	case action == "terminal_close":
		return monitor.CloseTerminalSession(params)
	default:
		return "", fmt.Errorf("unsupported operation: %s", action)
	}
}
