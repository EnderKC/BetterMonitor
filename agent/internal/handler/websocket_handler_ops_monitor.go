//go:build monitor_only

package handler

import "fmt"

// handleOperationAction 监控版：所有操作类命令均返回 unsupported 错误
func handleOperationAction(action string, _ map[string]interface{}) (string, error) {
	return "", fmt.Errorf("当前为监控模式 Agent，不支持操作类命令: %s", action)
}
