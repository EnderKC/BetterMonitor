//go:build monitor_only

package server

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// handleOperationMessage 处理操作类消息（监控版）
// 监控版不包含任何操作能力，所有操作类命令均返回 unsupported 错误
func (c *Client) handleOperationMessage(msgType string, message []byte, _ []byte) {
	// 尝试提取 request_id 以便返回对应的错误响应
	var baseMsg struct {
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(message, &baseMsg)

	c.log.Warn("监控版Agent收到不支持的操作命令: %s", msgType)

	// 构建错误响应
	resp := map[string]interface{}{
		"type":       msgType + "_error",
		"request_id": baseMsg.RequestID,
		"payload": map[string]interface{}{
			"error":   fmt.Sprintf("此Agent为监控版(monitor-only)，不支持 %s 操作", msgType),
			"code":    "ERR_UNSUPPORTED_OPERATION",
			"time":    time.Now().UTC().Format(time.RFC3339),
		},
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return
	}

	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	if c.wsConn != nil {
		c.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = c.wsConn.WriteMessage(websocket.TextMessage, respBytes)
		c.wsConn.SetWriteDeadline(time.Time{})
	}
}
