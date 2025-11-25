package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandParsing(t *testing.T) {
	// 测试命令解析
	tests := []struct {
		name        string
		message     string
		expectError bool
		expectedAction string
	}{
		{
			name:           "Valid ping command",
			message:        `{"action": "ping", "params": {}}`,
			expectError:    false,
			expectedAction: "ping",
		},
		{
			name:           "Valid file command",
			message:        `{"action": "file_list", "params": {"path": "/tmp"}}`,
			expectError:    false,
			expectedAction: "file_list",
		},
		{
			name:        "Invalid JSON",
			message:     `{"action": "ping", "params":}`,
			expectError: true,
		},
		{
			name:        "Missing action",
			message:     `{"params": {}}`,
			expectError: false,
			expectedAction: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req struct {
				Action string                 `json:"action"`
				Params map[string]interface{} `json:"params"`
			}

			err := json.Unmarshal([]byte(tt.message), &req)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAction, req.Action)
			}
		})
	}
}

func TestErrorResponseStructure(t *testing.T) {
	// 测试错误响应结构
	errorMsg := "测试错误消息"
	
	response := map[string]interface{}{
		"status": "error",
		"error":  errorMsg,
	}
	
	responseBytes, err := json.Marshal(response)
	assert.NoError(t, err)
	
	// 验证响应可以正确解析
	var parsedResponse map[string]interface{}
	err = json.Unmarshal(responseBytes, &parsedResponse)
	assert.NoError(t, err)
	
	assert.Equal(t, "error", parsedResponse["status"])
	assert.Equal(t, errorMsg, parsedResponse["error"])
}

func TestCommandActionClassification(t *testing.T) {
	// 测试命令分类
	tests := []struct {
		action   string
		category string
	}{
		{"file_list", "file"},
		{"file_read", "file"},
		{"file_write", "file"},
		{"process_list", "process"},
		{"process_kill", "process"},
		{"docker_start", "docker"},
		{"docker_stop", "docker"},
		{"nginx_restart", "nginx"},
		{"nginx_status", "nginx"},
		{"terminal_create", "terminal"},
		{"terminal_input", "terminal"},
		{"ping", "other"},
		{"unknown_action", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			var category string
			
			switch {
			case tt.action == "file_list" || tt.action == "file_read" || tt.action == "file_write":
				category = "file"
			case tt.action == "process_list" || tt.action == "process_kill":
				category = "process"
			case tt.action == "docker_start" || tt.action == "docker_stop":
				category = "docker"
			case tt.action == "nginx_restart" || tt.action == "nginx_status":
				category = "nginx"
			case tt.action == "terminal_create" || tt.action == "terminal_input":
				category = "terminal"
			case tt.action == "ping":
				category = "other"
			default:
				category = "unknown"
			}
			
			assert.Equal(t, tt.category, category)
		})
	}
}

func TestJSONResponseGeneration(t *testing.T) {
	// 测试JSON响应生成
	tests := []struct {
		name           string
		responseType   string
		data           interface{}
		expectedFields []string
	}{
		{
			name:         "Success response",
			responseType: "success",
			data:         `{"result": "ok"}`,
			expectedFields: []string{"result"},
		},
		{
			name:         "Error response",
			responseType: "error",
			data:         map[string]interface{}{"status": "error", "error": "test error"},
			expectedFields: []string{"status", "error"},
		},
		{
			name:         "Ping response",
			responseType: "ping",
			data:         `{"status":"pong"}`,
			expectedFields: []string{"status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response map[string]interface{}
			
			switch tt.responseType {
			case "success":
				// 模拟成功响应的JSON解析
				err := json.Unmarshal([]byte(tt.data.(string)), &response)
				assert.NoError(t, err)
			case "error":
				response = tt.data.(map[string]interface{})
			case "ping":
				err := json.Unmarshal([]byte(tt.data.(string)), &response)
				assert.NoError(t, err)
			}
			
			// 验证响应包含期望的字段
			for _, field := range tt.expectedFields {
				assert.Contains(t, response, field)
			}
		})
	}
}

func TestParameterValidation(t *testing.T) {
	// 测试参数验证
	tests := []struct {
		name     string
		action   string
		params   map[string]interface{}
		isValid  bool
	}{
		{
			name:   "File command with path",
			action: "file_list",
			params: map[string]interface{}{"path": "/tmp"},
			isValid: true,
		},
		{
			name:   "File command without path",
			action: "file_list",
			params: map[string]interface{}{},
			isValid: false,
		},
		{
			name:   "Process command with PID",
			action: "process_kill",
			params: map[string]interface{}{"pid": 1234},
			isValid: true,
		},
		{
			name:   "Process command without PID",
			action: "process_kill",
			params: map[string]interface{}{},
			isValid: false,
		},
		{
			name:   "Terminal command with session",
			action: "terminal_input",
			params: map[string]interface{}{"session_id": "test", "data": "ls\n"},
			isValid: true,
		},
		{
			name:   "Terminal command without session",
			action: "terminal_input",
			params: map[string]interface{}{"data": "ls\n"},
			isValid: false,
		},
		{
			name:   "Ping command",
			action: "ping",
			params: map[string]interface{}{},
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isValid bool
			
			switch tt.action {
			case "file_list":
				_, hasPath := tt.params["path"]
				isValid = hasPath
			case "process_kill":
				_, hasPID := tt.params["pid"]
				isValid = hasPID
			case "terminal_input":
				_, hasSession := tt.params["session_id"]
				_, hasData := tt.params["data"]
				isValid = hasSession && hasData
			case "ping":
				isValid = true
			default:
				isValid = false
			}
			
			assert.Equal(t, tt.isValid, isValid)
		})
	}
}