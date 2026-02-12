package controllers

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/user/server-ops-backend/models"
)

// 进程API的请求映射，用于跟踪请求
var processRequestMap sync.Map
var processResponseChannels sync.Map

// GetProcesses 获取服务器进程列表
func GetProcesses(c *gin.Context) {
	// 获取服务器ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器是否在线
	if server.Status != "online" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 生成请求ID
	requestID := uuid.New().String()

	// 创建响应通道
	responseChan := make(chan interface{}, 1)
	processResponseChannels.Store(requestID, responseChan)
	defer processResponseChannels.Delete(requestID)

	// 查找Agent WebSocket连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器Agent未连接"})
		return
	}

	// 转换为SafeConn类型
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器连接类型错误"})
		return
	}

	// 构造WebSocket消息
	message := map[string]interface{}{
		"type":       "process_list",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"action": "list",
		},
	}

	// 发送WebSocket消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送请求到Agent失败"})
		return
	}

	// 等待响应或超时
	select {
	case response := <-responseChan:
		// 返回响应
		c.JSON(http.StatusOK, response)
	case <-time.After(TimeoutProcessQuery): // 进程查询超时
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "获取进程列表超时"})
	}
}

// KillProcess 终止服务器上的进程
func KillProcess(c *gin.Context) {
	// 获取服务器ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取进程ID
	pidStr := c.Param("pid")
	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的进程ID"})
		return
	}

	// 查找服务器
	server, err := models.GetServerByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器是否在线
	if server.Status != "online" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 生成请求ID
	requestID := uuid.New().String()

	// 创建响应通道
	responseChan := make(chan interface{}, 1)
	processResponseChannels.Store(requestID, responseChan)
	defer processResponseChannels.Delete(requestID)

	// 查找Agent WebSocket连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器Agent未连接"})
		return
	}

	// 转换为SafeConn类型
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器连接类型错误"})
		return
	}

	// 构造WebSocket消息
	message := map[string]interface{}{
		"type":       "process_kill",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"pid": int32(pid),
		},
	}

	// 发送WebSocket消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送请求到Agent失败"})
		return
	}

	// 等待响应或超时
	select {
	case response := <-responseChan:
		// 返回响应
		c.JSON(http.StatusOK, response)
	case <-time.After(TimeoutProcessQuery): // 进程终止超时
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "终止进程超时"})
	}
}

// HandleProcessResponse 处理进程相关响应
func HandleProcessResponse(requestID string, response interface{}) {
	// 查找对应的响应通道
	chanVal, ok := processResponseChannels.Load(requestID)
	if !ok {
		log.Printf("找不到进程请求ID: %s 的响应通道", requestID)
		return
	}

	// 转换为通道类型
	responseChan, ok := chanVal.(chan interface{})
	if !ok {
		log.Printf("响应通道类型错误")
		return
	}

	// 发送响应到通道
	select {
	case responseChan <- response:
		log.Printf("已发送进程响应到通道")
	default:
		log.Printf("无法发送进程响应到通道，可能已关闭")
	}
} 