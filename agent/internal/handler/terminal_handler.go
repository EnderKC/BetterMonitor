package handler

import (
	"fmt"
	"sync"

	"github.com/user/server-ops-agent/internal/server"
	"github.com/user/server-ops-agent/pkg/logger"
)

// TerminalHandler 处理终端相关操作
type TerminalHandler struct {
	log          *logger.Logger
	sessions     map[string]*server.TerminalSession
	sessionsLock sync.Mutex
}

// NewTerminalHandler 创建一个新的终端处理器
func NewTerminalHandler(log *logger.Logger) *TerminalHandler {
	return &TerminalHandler{
		log:      log,
		sessions: make(map[string]*server.TerminalSession),
	}
}

// InitTerminalHandling 初始化终端处理功能
func InitTerminalHandling(client *server.Client, log *logger.Logger) {
	// 创建终端处理器
	handler := NewTerminalHandler(log)

	// 注册终端相关的WebSocket消息处理函数
	client.RegisterTerminalHandler(handler)

	log.Info("终端处理功能已初始化")
}

// StartSession 启动一个新的终端会话
func (h *TerminalHandler) StartSession(sessionID string) (*server.TerminalSession, error) {
	// 使用server包中的已实现功能
	session, err := server.StartTerminalSession(sessionID, h.log)
	if err != nil {
		h.log.Error("启动终端会话失败: %v", err)
		return nil, err
	}

	// 存储会话引用
	h.sessionsLock.Lock()
	h.sessions[sessionID] = session
	h.sessionsLock.Unlock()

	h.log.Info("终端会话启动成功: %s", sessionID)
	return session, nil
}

// GetSession 检查会话是否存在
func (h *TerminalHandler) GetSession(sessionID string) error {
	// 检查本地会话映射
	h.sessionsLock.Lock()
	_, exists := h.sessions[sessionID]
	h.sessionsLock.Unlock()

	if exists {
		return nil
	}

	// 检查server包中的会话
	session := server.GetTerminalSession(sessionID)
	if session == nil {
		return fmt.Errorf("会话不存在")
	}

	// 更新本地映射
	h.sessionsLock.Lock()
	h.sessions[sessionID] = session
	h.sessionsLock.Unlock()

	return nil
}

// WriteToTerminal 向终端写入数据
func (h *TerminalHandler) WriteToTerminal(sessionID string, data string) error {
	return server.WriteToTerminal(sessionID, data, h.log)
}

// ResizeTerminal 调整终端大小
func (h *TerminalHandler) ResizeTerminal(sessionID string, cols, rows uint16) error {
	return server.ResizeTerminal(sessionID, cols, rows, h.log)
}

// CloseSession 关闭终端会话
func (h *TerminalHandler) CloseSession(sessionID string) {
	// 从本地映射中删除
	h.sessionsLock.Lock()
	delete(h.sessions, sessionID)
	h.sessionsLock.Unlock()

	// 调用server包的关闭函数
	server.CloseTerminalSession(sessionID, h.log)

	h.log.Info("终端会话已关闭: %s", sessionID)
}
