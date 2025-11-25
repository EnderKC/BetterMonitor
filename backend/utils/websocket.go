package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 声明外部变量的引用 - 我们需要访问websocket_controller的ActiveAgentConnections
// 这里假设有一个导出的函数可以获取agent连接
var GetAgentConnectionFunc func(serverID uint) (*websocket.Conn, error)

var (
	// 缓存WebSocket连接 - 保留但逐步废弃
	wsConnections = make(map[uint]*websocket.Conn)
	wsConnMutex   = &sync.Mutex{}
)

// SendCommandToAgent 发送命令到Agent并等待响应
func SendCommandToAgent(serverID uint, secretKey string, data map[string]interface{}) (string, error) {
	log.Printf("[DEBUG] 开始向服务器 %d 发送命令 %s", serverID, data["action"])

	// 添加认证信息
	data["server_id"] = serverID
	data["secret_key"] = secretKey

	// 获取WebSocket连接 - 优先使用新的连接池
	var wsConn *websocket.Conn
	var err error

	// 如果有设置GetAgentConnectionFunc，优先使用它
	if GetAgentConnectionFunc != nil {
		log.Printf("[DEBUG] 尝试从新的连接池获取服务器 %d 的连接", serverID)
		wsConn, err = GetAgentConnectionFunc(serverID)
		if err != nil {
			log.Printf("[WARN] 从新的连接池获取服务器 %d 的连接失败: %v，尝试旧连接池", serverID, err)
		}
	}

	// 如果通过新池没有获取到连接，回退到旧池
	if wsConn == nil {
		log.Printf("[DEBUG] 尝试从旧的连接池获取服务器 %d 的连接", serverID)
		wsConn, err = getAgentConnection(serverID)
		if err != nil {
			log.Printf("[ERROR] 获取服务器 %d 的WebSocket连接失败: %v", serverID, err)
			return "", fmt.Errorf("无法获取代理连接: %v", err)
		}
	}

	// 生成请求ID
	requestID := fmt.Sprintf("%d-%d", serverID, time.Now().UnixNano())
	data["request_id"] = requestID

	log.Printf("[DEBUG] 生成请求ID: %s", requestID)

	// 将命令数据转换为JSON
	cmdData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[ERROR] 序列化命令数据失败: %v", err)
		return "", fmt.Errorf("序列化命令数据失败: %v", err)
	}

	// 创建一个通道用于接收响应
	respChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// 注册响应处理器
	registerResponseHandler(requestID, respChan, errChan)
	defer unregisterResponseHandler(requestID)

	log.Printf("[DEBUG] 已注册请求 %s 的响应处理器", requestID)

	// 发送命令
	wsConnMutex.Lock()
	err = wsConn.WriteMessage(websocket.TextMessage, cmdData)
	wsConnMutex.Unlock()
	if err != nil {
		log.Printf("[ERROR] 向服务器 %d 发送命令失败: %v", serverID, err)
		return "", fmt.Errorf("发送命令失败: %v", err)
	}

	log.Printf("[DEBUG] 已向服务器 %d 发送命令，等待响应...", serverID)

	// 等待响应或超时
	select {
	case resp := <-respChan:
		log.Printf("[DEBUG] 接收到服务器 %d 的响应，请求ID: %s", serverID, requestID)
		return resp, nil
	case err := <-errChan:
		log.Printf("[ERROR] 接收到服务器 %d 的错误响应: %v，请求ID: %s", serverID, err, requestID)
		return "", err
	case <-time.After(30 * time.Second):
		log.Printf("[ERROR] 等待服务器 %d 响应超时，请求ID: %s", serverID, requestID)
		return "", fmt.Errorf("等待Agent响应超时")
	}
}

// 响应处理器映射
var (
	responseHandlers      = make(map[string]chan string)
	responseErrorHandlers = make(map[string]chan error)
	handlersLock          = &sync.Mutex{}
)

// 注册响应处理器
func registerResponseHandler(requestID string, respChan chan string, errChan chan error) {
	handlersLock.Lock()
	defer handlersLock.Unlock()
	responseHandlers[requestID] = respChan
	responseErrorHandlers[requestID] = errChan
}

// 取消注册响应处理器
func unregisterResponseHandler(requestID string) {
	handlersLock.Lock()
	defer handlersLock.Unlock()
	delete(responseHandlers, requestID)
	delete(responseErrorHandlers, requestID)
}

// 获取Agent的WebSocket连接
func getAgentConnection(serverID uint) (*websocket.Conn, error) {
	wsConnMutex.Lock()
	defer wsConnMutex.Unlock()

	// 检查是否已有连接
	if conn, exists := wsConnections[serverID]; exists && isConnectionAlive(conn) {
		return conn, nil
	}

	// 如果不存在连接或连接已断开，则返回错误
	return nil, fmt.Errorf("服务器(ID: %d)未连接", serverID)
}

// 检查WebSocket连接是否存活
func isConnectionAlive(conn *websocket.Conn) bool {
	if conn == nil {
		return false
	}

	// 发送ping消息检测连接
	err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
	return err == nil
}

// RegisterConnection 注册WebSocket连接
func RegisterConnection(serverID uint, conn *websocket.Conn) {
	wsConnMutex.Lock()
	defer wsConnMutex.Unlock()
	wsConnections[serverID] = conn
}

// RemoveConnection 移除WebSocket连接
func RemoveConnection(serverID uint) {
	wsConnMutex.Lock()
	defer wsConnMutex.Unlock()
	if conn, exists := wsConnections[serverID]; exists {
		conn.Close()
		delete(wsConnections, serverID)
	}
}

// HandleAgentResponse 处理来自Agent的响应
func HandleAgentResponse(response []byte) error {
	log.Printf("[DEBUG] 收到Agent响应: %s", string(response))

	// 首先解析基本结构，获取请求ID
	var baseResp struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id"`
		Status    string          `json:"status"`
		Data      json.RawMessage `json:"data"`
		Error     string          `json:"error"`
	}

	if err := json.Unmarshal(response, &baseResp); err != nil {
		log.Printf("[ERROR] 解析Agent基本响应结构失败: %v", err)
		return fmt.Errorf("解析响应失败: %v", err)
	}

	log.Printf("[DEBUG] 解析出请求ID: %s, 状态: %s, 类型: %s", baseResp.RequestID, baseResp.Status, baseResp.Type)

	// 检查是否为Nginx相关类型
	isNginxResponse := strings.Contains(baseResp.Type, "nginx") ||
		baseResp.Type == "nginx_success" || baseResp.Type == "nginx_error" ||
		(baseResp.Type == "success" && strings.HasPrefix(baseResp.RequestID, "nginx_"))

	// 查找对应的处理器
	handlersLock.Lock()
	respChan, respExists := responseHandlers[baseResp.RequestID]
	errChan, errExists := responseErrorHandlers[baseResp.RequestID]
	handlersLock.Unlock()

	if !respExists || !errExists {
		log.Printf("[WARN] 未找到请求ID为%s的处理器，可能请求已超时", baseResp.RequestID)
		return fmt.Errorf("未找到请求ID为%s的处理器", baseResp.RequestID)
	}

	// 处理错误响应
	if baseResp.Status == "error" || baseResp.Type == "error" || baseResp.Type == "nginx_error" {
		// 提取错误信息
		errMsg := baseResp.Error
		if errMsg == "" && len(baseResp.Data) > 0 {
			// 尝试从Data字段解析错误信息
			var errorData struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(baseResp.Data, &errorData); err == nil && errorData.Error != "" {
				errMsg = errorData.Error
			} else {
				// 使用原始Data作为错误信息
				errMsg = string(baseResp.Data)
			}
		}
		log.Printf("[ERROR] Agent返回错误: %s", errMsg)
		errChan <- fmt.Errorf("Agent错误: %s", errMsg)
		return nil
	}

	// 处理成功响应
	var dataStr string

	// 处理不同类型的响应
	switch {
	case baseResp.Type == "nginx_success" || isNginxResponse:
		// Nginx成功响应处理
		log.Printf("[DEBUG] 处理Nginx响应，类型: %s", baseResp.Type)

		// 直接使用原始Data字段的JSON数据
		if len(baseResp.Data) > 0 && string(baseResp.Data) != "null" {
			dataStr = string(baseResp.Data)
		} else {
			// 如果Data为空，构造一个空结果
			dataStr = `{"status": "success", "message": "操作成功"}`
		}

		// 直接发送响应到通道
		log.Printf("[DEBUG] 发送Nginx响应数据到通道，长度: %d", len(dataStr))
		respChan <- dataStr
		return nil

	default:
		// 默认处理
		if len(baseResp.Data) == 0 || string(baseResp.Data) == "null" {
			// 如果Data字段为空或为null，则可能是旧格式
			// 移除RequestID, Type, Status字段，保留其他字段作为数据
			var fullResp map[string]interface{}
			if err := json.Unmarshal(response, &fullResp); err != nil {
				log.Printf("[ERROR] 解析完整响应失败: %v", err)
				return fmt.Errorf("解析响应失败: %v", err)
			}

			// 删除非数据字段
			delete(fullResp, "request_id")
			delete(fullResp, "type")
			delete(fullResp, "status")
			delete(fullResp, "error")

			// 将剩余字段重新编码为JSON
			if remainingData, err := json.Marshal(fullResp); err == nil && len(remainingData) > 2 {
				dataStr = string(remainingData)
			} else {
				// 如果没有剩余字段，返回空对象
				dataStr = "{}"
			}
		} else {
			// 直接使用Data字段的内容
			dataStr = string(baseResp.Data)
		}
	}

	log.Printf("[DEBUG] 发送响应数据到通道，长度: %d", len(dataStr))
	respChan <- dataStr

	return nil
}

// GetAgentConnectionFromMap 直接从wsConnections映射中获取连接，仅用于调试
func GetAgentConnectionFromMap(serverID uint) (*websocket.Conn, error) {
	wsConnMutex.Lock()
	defer wsConnMutex.Unlock()

	conn, exists := wsConnections[serverID]
	if !exists {
		return nil, fmt.Errorf("服务器(ID: %d)在旧连接池中未连接", serverID)
	}

	if conn == nil {
		return nil, fmt.Errorf("服务器(ID: %d)连接为空", serverID)
	}

	return conn, nil
}
