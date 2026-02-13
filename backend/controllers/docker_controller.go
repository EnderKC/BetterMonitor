package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
)

// parseServerId 解析服务器ID参数
func parseServerId(idStr string) (uint, error) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("无效的服务器ID: %v", err)
	}
	return uint(id), nil
}

// parseIntParam 解析整数参数
func parseIntParam(paramStr string) (int, error) {
	val, err := strconv.Atoi(paramStr)
	if err != nil {
		return 0, fmt.Errorf("无效的整数参数: %v", err)
	}
	return val, nil
}

// GetContainers 获取服务器上的Docker容器列表
func GetContainers(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "list",
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// GetContainerLogs 获取容器日志
func GetContainerLogs(c *gin.Context) {
	// 获取服务器ID和容器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	containerID := c.Param("container_id")
	fmt.Printf("[调试] 获取容器日志: 服务器ID=%d, 容器ID=%s\n", serverID, containerID)

	tail := 100 // 默认获取100行日志

	if tailParam := c.Query("tail"); tailParam != "" {
		if parsedTail, err := parseIntParam(tailParam); err == nil {
			tail = parsedTail
		}
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "logs",
			"params": map[string]interface{}{
				"container_id": containerID,
				"tail":         tail,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// StartContainer 启动容器
func StartContainer(c *gin.Context) {
	// 获取服务器ID和容器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	containerID := c.Param("container_id")
	fmt.Printf("[调试] 启动容器: 服务器ID=%d, 容器ID=%s\n", serverID, containerID)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "start",
			"params": map[string]interface{}{
				"container_id": containerID,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// StopContainer 停止容器
func StopContainer(c *gin.Context) {
	// 获取服务器ID和容器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	containerID := c.Param("container_id")
	fmt.Printf("[调试] 停止容器: 服务器ID=%d, 容器ID=%s\n", serverID, containerID)

	// 解析超时参数
	timeout := 10 // 默认10秒超时
	if timeoutParam := c.Query("timeout"); timeoutParam != "" {
		if parsedTimeout, err := parseIntParam(timeoutParam); err == nil {
			timeout = parsedTimeout
		}
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "stop",
			"params": map[string]interface{}{
				"container_id": containerID,
				"timeout":      timeout,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// RestartContainer 重启容器
func RestartContainer(c *gin.Context) {
	// 获取服务器ID和容器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	containerID := c.Param("container_id")
	fmt.Printf("[调试] 重启容器: 服务器ID=%d, 容器ID=%s\n", serverID, containerID)

	// 解析超时参数
	timeout := 10 // 默认10秒超时
	if timeoutParam := c.Query("timeout"); timeoutParam != "" {
		if parsedTimeout, err := parseIntParam(timeoutParam); err == nil {
			timeout = parsedTimeout
		}
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "restart",
			"params": map[string]interface{}{
				"container_id": containerID,
				"timeout":      timeout,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// RemoveContainer 删除容器
func RemoveContainer(c *gin.Context) {
	// 获取服务器ID和容器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	containerID := c.Param("container_id")
	fmt.Printf("[调试] 删除容器: 服务器ID=%d, 容器ID=%s\n", serverID, containerID)

	// 获取强制删除标志
	force := false
	if forceParam := c.Query("force"); forceParam == "true" {
		force = true
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "remove",
			"params": map[string]interface{}{
				"container_id": containerID,
				"force":        force,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// GetImages 获取服务器上的Docker镜像列表
func GetImages(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "images",
			"action":  "list",
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// PullImage 拉取Docker镜像
func PullImage(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 解析请求体获取镜像名称
	var requestBody struct {
		Image string `json:"image"`
	}
	if err := c.BindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "images",
			"action":  "pull",
			"params": map[string]interface{}{
				"image": requestBody.Image,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// RemoveImage 删除Docker镜像
func RemoveImage(c *gin.Context) {
	// 获取服务器ID和镜像ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	imageID := c.Param("image_id")
	fmt.Printf("[调试] 删除镜像: 服务器ID=%d, 镜像ID=%s\n", serverID, imageID)

	// 获取强制删除标志
	force := false
	if forceParam := c.Query("force"); forceParam == "true" {
		force = true
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "images",
			"action":  "remove",
			"params": map[string]interface{}{
				"image_id": imageID,
				"force":    force,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// GetComposes 获取服务器上的Docker Compose项目列表
func GetComposes(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "list",
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// GetComposeConfig 获取Docker Compose项目配置
func GetComposeConfig(c *gin.Context) {
	// 获取服务器ID和Compose项目名称
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	composeName := c.Param("name")
	fmt.Printf("[调试] 获取Compose配置: 服务器ID=%d, Compose项目=%s\n", serverID, composeName)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "config",
			"params": map[string]interface{}{
				"name": composeName,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// ComposeUp 启动Docker Compose项目
func ComposeUp(c *gin.Context) {
	// 获取服务器ID和Compose项目名称
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	composeName := c.Param("name")
	fmt.Printf("[调试] 启动Compose项目: 服务器ID=%d, Compose项目=%s\n", serverID, composeName)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "up",
			"params": map[string]interface{}{
				"name": composeName,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// ComposeDown 停止Docker Compose项目
func ComposeDown(c *gin.Context) {
	// 获取服务器ID和Compose项目名称
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	composeName := c.Param("name")
	fmt.Printf("[调试] 停止Compose项目: 服务器ID=%d, Compose项目=%s\n", serverID, composeName)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "down",
			"params": map[string]interface{}{
				"name": composeName,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// RemoveCompose 删除Docker Compose项目
func RemoveCompose(c *gin.Context) {
	// 获取服务器ID和Compose项目名称
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	composeName := c.Param("name")
	fmt.Printf("[调试] 删除Compose项目: 服务器ID=%d, Compose项目=%s\n", serverID, composeName)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "remove",
			"params": map[string]interface{}{
				"name": composeName,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// CreateCompose 创建Docker Compose项目
func CreateCompose(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 解析请求体获取Compose项目信息
	var requestBody struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := c.BindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "composes",
			"action":  "create",
			"params": map[string]interface{}{
				"name":    requestBody.Name,
				"content": requestBody.Content,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, responseData)
}

// CreateContainer 创建Docker容器
func CreateContainer(c *gin.Context) {
	// 获取服务器ID
	id := c.Param("id")
	serverID, err := parseServerId(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	fmt.Printf("[调试] 创建容器: 服务器ID=%d\n", serverID)

	// 解析请求体
	var requestBody struct {
		Name    string            `json:"name"`
		Image   string            `json:"image"`
		Ports   []string          `json:"ports"`
		Volumes []string          `json:"volumes"`
		Env     map[string]string `json:"env"`
		Command string            `json:"command"`
		Restart string            `json:"restart"`
		Network string            `json:"network"`
	}

	if err := c.BindJSON(&requestBody); err != nil {
		fmt.Printf("[错误] 解析创建容器请求体失败: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	fmt.Printf("[调试] 创建容器请求解析成功: 容器名=%s, 镜像=%s\n",
		requestBody.Name, requestBody.Image)

	// 验证服务器是否存在
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 生成请求ID
	requestID := generateRequestID()

	// 构建发送到Agent的消息
	message := map[string]interface{}{
		"type":       "docker_command",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"command": "containers",
			"action":  "create",
			"params": map[string]interface{}{
				"name":    requestBody.Name,
				"image":   requestBody.Image,
				"ports":   requestBody.Ports,
				"volumes": requestBody.Volumes,
				"env":     requestBody.Env,
				"command": requestBody.Command,
				"restart": requestBody.Restart,
				"network": requestBody.Network,
			},
		},
	}

	// 发送请求并处理响应
	responseData, err := sendAgentRequest(server, message, requestID)
	if err != nil {
		fmt.Printf("[错误] 发送创建容器请求失败: %v\n", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, responseData)
}

// 发送请求到Agent并处理响应
// 【安全修复】添加success字段验证，确保Agent返回成功状态
func sendAgentRequest(server *models.Server, message map[string]interface{}, requestID string) (map[string]interface{}, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(server.ID)
	if !ok {
		fmt.Printf("[错误] 服务器ID=%d 的Agent未连接\n", server.ID)
		return nil, ErrAgentNotConnected
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		fmt.Printf("[错误] 服务器ID=%d 的连接类型错误，获取到类型: %T\n", server.ID, agentConnVal)
		return nil, ErrInvalidConnectionType
	}

	// 创建响应通道
	responseChan := make(chan interface{}, 1)
	// 使用WebSocket controller中定义的通道变量
	dockerResponseChannels.Store(requestID, responseChan)
	defer dockerResponseChannels.Delete(requestID)

	// 将用户连接与请求ID关联，以便将响应发送回用户
	dockerRequestMap.Store(requestID, agentConn)
	defer dockerRequestMap.Delete(requestID)

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(server.ID, requestID)
	defer unregisterPendingRequest(server.ID, requestID)

	// 转换消息为JSON字符串以便日志记录
	messageBytes, _ := json.Marshal(message)
	fmt.Printf("[调试] 发送Docker命令到服务器ID=%d, 请求ID=%s, 消息内容: %s\n",
		server.ID, requestID, string(messageBytes))

	// 发送消息到Agent
	if err := agentConn.WriteJSON(message); err != nil {
		fmt.Printf("[错误] 向服务器ID=%d发送消息失败: %v\n", server.ID, err)
		return nil, ErrSendRequestFailed
	}

	fmt.Printf("[调试] 消息已发送，等待服务器ID=%d的响应, 请求ID=%s\n", server.ID, requestID)

	// 设置超时时间
	timeout := time.After(TimeoutSimpleQuery)

	// 等待响应
	select {
	case response := <-responseChan:
		fmt.Printf("[调试] 收到服务器ID=%d的响应, 请求ID=%s\n", server.ID, requestID)

		// 转换响应数据
		responseData, ok := response.(map[string]interface{})
		if !ok {
			fmt.Printf("[错误] 响应格式无效, 请求ID=%s, 类型: %T\n", requestID, response)
			return nil, ErrInvalidResponseFormat
		}

		// 记录响应内容
		responseBytes, _ := json.Marshal(responseData)
		fmt.Printf("[调试] 响应内容: %s\n", string(responseBytes))

		// 【安全修复】验证Agent响应的success/error状态
		if err := validateAgentResponse(responseData); err != nil {
			fmt.Printf("[错误] Agent返回错误, 请求ID=%s: %v\n", requestID, err)
			return nil, err
		}

		return responseData, nil
	case <-timeout:
		fmt.Printf("[错误] 请求超时, 服务器ID=%d, 请求ID=%s\n", server.ID, requestID)
		return nil, ErrRequestTimeout
	}
}

// validateAgentResponse 验证Agent响应是否成功
// 检查多种可能的错误指示字段，支持多种类型的字段值
func validateAgentResponse(resp map[string]interface{}) error {
	// 检查 type 字段是否为 error
	if respType, ok := resp["type"].(string); ok {
		if respType == "error" || respType == "docker_error" {
			errMsg := extractErrorMessage(resp)
			return fmt.Errorf("Agent错误: %s", errMsg)
		}
	}

	// 检查 success 字段（支持多种类型：bool, string, number）
	if !getBoolish(resp, "success", true) {
		errMsg := extractErrorMessage(resp)
		return fmt.Errorf("操作失败: %s", errMsg)
	}

	// 检查 status 字段（支持 string 和 int 状态码）
	if isErrorStatus(resp) {
		errMsg := extractErrorMessage(resp)
		return fmt.Errorf("Agent状态错误: %s", errMsg)
	}

	// 检查 error 字段是否存在且非空（支持 string, map, array）
	if errText := getErrorField(resp); errText != "" {
		return fmt.Errorf("Agent错误: %s", errText)
	}

	return nil
}

// getBoolish 从响应中提取布尔值，支持 bool/string/number 类型
// 如果字段不存在，返回 defaultVal
func getBoolish(resp map[string]interface{}, key string, defaultVal bool) bool {
	val, exists := resp[key]
	if !exists {
		return defaultVal
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes" || v == "ok"
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return defaultVal
	}
}

// isErrorStatus 检查响应中的 status 字段是否表示错误
// 支持 string 和 number 类型的状态码
func isErrorStatus(resp map[string]interface{}) bool {
	val, exists := resp["status"]
	if !exists {
		return false
	}
	switch v := val.(type) {
	case string:
		return v == "error" || v == "failed" || v == "failure"
	case float64:
		return v >= 400 // HTTP 错误状态码
	case int:
		return v >= 400
	case int64:
		return v >= 400
	default:
		return false
	}
}

// getErrorField 从响应中提取 error 字段的值
// 支持 string, map, array 类型
func getErrorField(resp map[string]interface{}) string {
	val, exists := resp["error"]
	if !exists || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		// 如果 error 是 map，尝试提取其中的 message 或 error 字段
		if msg, ok := v["message"].(string); ok && msg != "" {
			return msg
		}
		if msg, ok := v["error"].(string); ok && msg != "" {
			return msg
		}
		// 序列化为 JSON 字符串
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
	case []interface{}:
		// 如果 error 是数组，拼接所有字符串元素
		var errMsgs []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				errMsgs = append(errMsgs, s)
			}
		}
		if len(errMsgs) > 0 {
			return strings.Join(errMsgs, "; ")
		}
	default:
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// extractErrorMessage 从响应中提取错误信息
// 支持多种响应格式和嵌套结构
func extractErrorMessage(resp map[string]interface{}) string {
	// 优先从 error 字段提取
	if errText := getErrorField(resp); errText != "" {
		return errText
	}
	// 从 message 字段提取
	if msg, ok := resp["message"].(string); ok && msg != "" {
		return msg
	}
	// 从 msg 字段提取（某些 Agent 可能使用 msg）
	if msg, ok := resp["msg"].(string); ok && msg != "" {
		return msg
	}
	// 从 data.error 或 data.message 字段提取
	if data, ok := resp["data"].(map[string]interface{}); ok {
		if errText := getErrorField(data); errText != "" {
			return errText
		}
		if msg, ok := data["message"].(string); ok && msg != "" {
			return msg
		}
	}
	return "未知错误"
}

// HandleDockerResponse 处理Docker操作的响应
func HandleDockerResponse(requestID string, responseData map[string]interface{}) {
	fmt.Printf("[调试] 处理Docker响应, 请求ID=%s\n", requestID)

	// 从映射中获取响应通道
	respChanVal, ok := dockerResponseChannels.Load(requestID)
	if !ok {
		fmt.Printf("[错误] 未找到请求ID=%s的响应通道\n", requestID)
		return
	}

	// 转换为通道
	respChan, ok := respChanVal.(chan interface{})
	if !ok {
		fmt.Printf("[错误] 响应通道类型错误, 请求ID=%s, 类型: %T\n", requestID, respChanVal)
		return
	}

	// 发送响应到通道
	select {
	case respChan <- responseData:
		fmt.Printf("[调试] 成功将响应发送到通道, 请求ID=%s\n", requestID)
	default:
		fmt.Printf("[错误] 响应通道已满或已关闭, 请求ID=%s\n", requestID)
	}
}

// Docker操作相关错误
var (
	ErrAgentNotConnected     = NewError("服务器Agent未连接")
	ErrInvalidConnectionType = NewError("服务器连接类型错误")
	ErrSendRequestFailed     = NewError("发送请求到Agent失败")
	ErrRequestTimeout        = NewError("请求超时")
	ErrInvalidResponseFormat = NewError("响应数据格式错误")
)

// Error 自定义错误类型
type Error struct {
	Message string
}

// Error 实现error接口
func (e *Error) Error() string {
	return e.Message
}

// NewError 创建新的错误
func NewError(message string) *Error {
	return &Error{Message: message}
}

// 从JSON字符串解析为字典
func parseJSONToMap(jsonStr string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	return result, err
}
