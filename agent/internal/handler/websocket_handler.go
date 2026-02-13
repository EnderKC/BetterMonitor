package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// HandleCommand 处理来自面板端的命令
func HandleCommand(c *websocket.Conn, serverID uint, secretKey string, message []byte) {
	// 兼容面板端新消息格式：
	// {
	//   "type": "agent_upgrade",
	//   "request_id": "...",
	//   "payload": { ... }
	// }
	var typedReq struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(message, &typedReq); err == nil && strings.TrimSpace(typedReq.Type) != "" {
		switch typedReq.Type {
		case "agent_upgrade":
			HandleAgentUpgradeMessage(c, serverID, secretKey, typedReq.RequestID, typedReq.Payload)
		default:
			SendErrorResponse(c, fmt.Sprintf("未知的消息类型: %s", typedReq.Type))
		}
		return
	}

	// 解析命令
	var req struct {
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}

	if err := json.Unmarshal(message, &req); err != nil {
		SendErrorResponse(c, "解析命令失败")
		return
	}

	// 处理不同类型的命令
	var response string
	var err error

	// 根据Action分发到不同的处理函数
	switch {
	case isOperationAction(req.Action):
		response, err = handleOperationAction(req.Action, req.Params)

	case req.Action == "ping":
		response = `{"status":"pong"}`

	default:
		err = fmt.Errorf("未知的命令: %s", req.Action)
	}

	// 检查处理结果
	if err != nil {
		SendErrorResponse(c, err.Error())
		return
	}

	// 发送成功响应
	SendSuccessResponse(c, response)
}

// SendErrorResponse 发送错误响应
func SendErrorResponse(c *websocket.Conn, errMsg string) {
	response := map[string]interface{}{
		"status": "error",
		"error":  errMsg,
	}
	responseBytes, _ := json.Marshal(response)
	
	// 添加写入超时
	if c != nil {
		c.SetWriteDeadline(time.Now().Add(10 * time.Second))
		defer c.SetWriteDeadline(time.Time{}) // 重置写入超时
		
		if err := c.WriteMessage(websocket.TextMessage, responseBytes); err != nil {
			// 记录错误但不中断流程
			fmt.Printf("发送错误响应失败: %v\n", err)
		}
	}
}

// SendSuccessResponse 发送成功响应
func SendSuccessResponse(c *websocket.Conn, data string) {
	if c == nil {
		fmt.Println("警告: WebSocket连接为空，无法发送响应")
		return
	}
	
	// 添加写入超时
	c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer c.SetWriteDeadline(time.Time{}) // 重置写入超时
	
	// 添加错误处理
	if err := c.WriteMessage(websocket.TextMessage, []byte(data)); err != nil {
		fmt.Printf("发送成功响应失败: %v\n", err)
	}
}

// isOperationAction 判断 action 是否为操作类命令（terminal/file/process/docker/nginx）
func isOperationAction(action string) bool {
	switch {
	case strings.HasPrefix(action, "file_"),
		strings.HasPrefix(action, "process_"),
		strings.HasPrefix(action, "docker_"),
		strings.HasPrefix(action, "nginx_"),
		action == "terminal_create",
		action == "terminal_resize",
		action == "terminal_input",
		action == "terminal_close":
		return true
	default:
		return false
	}
} 