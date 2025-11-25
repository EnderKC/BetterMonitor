package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/user/server-ops-backend/models"
)

// 定义终端会话结构
type TerminalSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ServerID  uint      `json:"server_id"`
	UserID    uint      `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// 存储终端会话的内存映射
var terminalSessions = make(map[string]TerminalSession)

// CreateTerminalSession 创建一个新的终端会话
func CreateTerminalSession(c *gin.Context) {
	// 获取服务器ID
	serverID, err := parseUintParam(c, "id")
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

	// 获取当前用户ID
	userIDInterface, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}
	userID := userIDInterface.(uint)

	// 解析请求体
	var request struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 生成会话ID
	sessionID := uuid.New().String()

	// 创建会话
	session := TerminalSession{
		ID:        sessionID,
		Name:      request.Name,
		ServerID:  server.ID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	// 存储会话
	terminalSessions[sessionID] = session

	// 检查服务器是否在线
	if server.Status != "online" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "服务器当前离线，无法创建终端会话",
		})
		return
	}

	// 返回会话信息
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "终端会话创建成功",
		"data":    session,
	})
}

// GetTerminalSessions 获取服务器的所有终端会话
func GetTerminalSessions(c *gin.Context) {
	// 获取服务器ID
	serverID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	_, err = models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 获取当前用户ID
	userIDInterface, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}
	userID := userIDInterface.(uint)

	// 查找该用户和该服务器的会话
	var sessions []TerminalSession
	for _, session := range terminalSessions {
		if session.ServerID == serverID && session.UserID == userID {
			sessions = append(sessions, session)
		}
	}

	// 返回会话列表，确保即使没有会话也返回空数组[]而不是null
	if sessions == nil {
		sessions = []TerminalSession{}
	}

	// 使用标准的API响应格式
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取会话列表成功",
		"data":    sessions,
	})
}

// DeleteTerminalSession 删除终端会话
func DeleteTerminalSession(c *gin.Context) {
	// 获取服务器ID
	serverID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在
	_, err = models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 获取会话ID
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的会话ID"})
		return
	}

	// 获取当前用户ID
	userIDInterface, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}
	userID := userIDInterface.(uint)

	// 检查会话是否存在
	session, ok := terminalSessions[sessionID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	// 检查会话是否属于当前用户
	if session.UserID != userID || session.ServerID != serverID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	// 删除会话
	delete(terminalSessions, sessionID)

	// 返回成功消息
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "会话已删除",
	})
}

// GetTerminalWorkingDirectory 获取终端会话的当前工作目录
func GetTerminalWorkingDirectory(c *gin.Context) {
	// 获取服务器ID
	serverID, err := parseUintParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 验证服务器是否存在且在线
	server, err := models.GetServerByID(serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线"})
		return
	}

	// 获取会话ID
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的会话ID"})
		return
	}

	// 获取当前用户ID
	userIDInterface, exists := c.Get("userId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}
	userID := userIDInterface.(uint)

	// 检查会话是否存在
	session, ok := terminalSessions[sessionID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	// 检查会话是否属于当前用户
	if session.UserID != userID || session.ServerID != serverID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	// 通过WebSocket获取工作目录
	workingDir, err := requestTerminalWorkingDirectoryViaWebSocket(serverID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取工作目录失败: %v", err)})
		return
	}

	// 返回工作目录
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"working_dir": workingDir,
	})
}

// 工具函数：解析路由参数为uint
func parseUintParam(c *gin.Context, paramName string) (uint, error) {
	idStr := c.Param(paramName)
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	return id, err
}
