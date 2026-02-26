//go:build !monitor_only

package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/user/server-ops-agent/internal/monitor"
)

// handleOperationMessage 处理操作类消息（全功能版）
// 包含终端、文件、进程、Docker、Nginx、Shell 等操作命令的路由
func (c *Client) handleOperationMessage(msgType string, message []byte, msgCopy []byte) {
	switch msgType {
	case "terminal_input":
		var termMsg struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Input     string `json:"input"`
		}
		if err := json.Unmarshal(message, &termMsg); err != nil {
			c.log.Error("解析终端输入消息失败: %v", err)
			return
		}
		c.handleTerminalInput(termMsg.SessionID, termMsg.Input)

	case "terminal_resize":
		var resizeMsg struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Data      string `json:"data"`
		}
		if err := json.Unmarshal(message, &resizeMsg); err != nil {
			c.log.Error("解析终端调整大小消息失败: %v", err)
			return
		}
		c.handleTerminalResize(resizeMsg.SessionID, resizeMsg.Data)

	case "terminal_create":
		var createMsg struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(message, &createMsg); err != nil {
			c.log.Error("解析创建终端会话消息失败: %v", err)
			return
		}
		c.handleTerminalCreate(createMsg.SessionID)

	case "terminal_close":
		var closeMsg struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(message, &closeMsg); err != nil {
			c.log.Error("解析关闭终端会话消息失败: %v", err)
			return
		}
		c.handleTerminalClose(closeMsg.SessionID)

	case "file_list":
		go c.handleFileList(msgCopy)

	case "file_content":
		go c.handleFileContent(msgCopy)

	case "file_upload":
		go c.handleFileUpload(msgCopy)

	case "docker_file":
		go c.handleDockerFile(msgCopy)

	case "process_list":
		go c.handleProcessList(msgCopy)

	case "process_kill":
		go c.handleProcessKill(msgCopy)

	case "docker_command":
		go c.handleDockerCommand(msgCopy)

	case "docker_logs_stream":
		go c.handleDockerLogsStream(msgCopy)

	case "nginx_command":
		go c.handleNginxCommand(msgCopy)

	case "shell_command":
		go c.handleShellCommand(msgCopy)

	default:
		c.log.Warn("收到未知类型的WebSocket消息: %s", msgType)
	}
}

// TerminalHandler 终端处理器接口
type TerminalHandler interface {
	StartSession(sessionID string) (*TerminalSession, error)
	GetSession(sessionID string) error
	WriteToTerminal(sessionID string, data string) error
	ResizeTerminal(sessionID string, cols, rows uint16) error
	CloseSession(sessionID string)
}

// RegisterTerminalHandler 注册终端处理器
func (c *Client) RegisterTerminalHandler(handler TerminalHandler) {
	c.log.Info("注册终端处理器")
}

// ─── Shell / 终端命令处理 ──────────────────────────────────────────────────────

// handleShellCommand 处理Shell命令
func (c *Client) handleShellCommand(message []byte) {
	c.log.Debug("收到Shell命令请求")

	var cmd struct {
		Type    string `json:"type"`
		Payload struct {
			Type        string   `json:"type"`
			Data        string   `json:"data"`
			Session     string   `json:"session"`
			ContainerID string   `json:"container_id,omitempty"`
			Command     []string `json:"command,omitempty"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(message, &cmd); err != nil {
		c.log.Error("解析Shell命令失败: %v", err)
		return
	}

	c.log.Debug("处理Shell命令: 类型=%s, 会话=%s", cmd.Payload.Type, cmd.Payload.Session)

	// 如果指定了容器ID，则使用容器内的 Exec 作为终端
	if cmd.Payload.ContainerID != "" {
		c.handleContainerTerminalCommand(cmd.Payload.ContainerID, cmd.Payload.Session, cmd.Payload.Type, cmd.Payload.Data, cmd.Payload.Command)
		return
	}

	// 根据命令类型处理（宿主机终端）
	switch cmd.Payload.Type {
	case "input":
		c.handleTerminalInput(cmd.Payload.Session, cmd.Payload.Data)
	case "resize":
		c.handleTerminalResize(cmd.Payload.Session, cmd.Payload.Data)
	case "create":
		c.handleTerminalCreate(cmd.Payload.Session)
	case "close":
		c.handleTerminalClose(cmd.Payload.Session)
	case "get_cwd":
		c.handleTerminalGetWorkingDirectory(cmd.Payload.Session)
	default:
		c.log.Warn("未知的Shell命令类型: %s", cmd.Payload.Type)
	}
}

// handleTerminalInput 处理终端输入
func (c *Client) handleTerminalInput(sessionID, input string) {
	c.log.Debug("处理终端输入: 会话=%s", sessionID)

	var session *TerminalSession
	session = GetTerminalSession(sessionID)
	if session == nil {
		var err error
		session, err = StartTerminalSession(sessionID, c.log)
		if err != nil {
			c.log.Error("启动终端会话失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("启动终端会话失败: %v", err))
			return
		}
		go c.readTerminalOutput(session)
	}

	if err := WriteToTerminal(sessionID, input, c.log); err != nil {
		c.log.Error("向终端写入数据失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("向终端写入数据失败: %v", err))
	}
}

// handleTerminalResize 处理终端大小调整
func (c *Client) handleTerminalResize(sessionID, data string) {
	c.log.Debug("处理终端大小调整: 会话=%s", sessionID)

	var dimensions struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal([]byte(data), &dimensions); err != nil {
		c.log.Error("解析终端大小数据失败: %v", err)
		return
	}

	if err := ResizeTerminal(sessionID, dimensions.Cols, dimensions.Rows, c.log); err != nil {
		c.log.Error("调整终端大小失败: %v", err)
	}
}

// handleTerminalCreate 处理终端创建
func (c *Client) handleTerminalCreate(sessionID string) {
	c.log.Debug("处理终端创建: 会话=%s", sessionID)

	if session := GetTerminalSession(sessionID); session != nil {
		c.log.Debug("会话已存在，无需创建: %s", sessionID)
		return
	}

	session, err := StartTerminalSession(sessionID, c.log)
	if err != nil {
		c.log.Error("创建终端会话失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("创建终端会话失败: %v", err))
		return
	}

	go c.readTerminalOutput(session)
}

// handleTerminalClose 处理终端关闭
func (c *Client) handleTerminalClose(sessionID string) {
	c.log.Debug("处理终端关闭: 会话=%s", sessionID)
	CloseTerminalSession(sessionID, c.log)
}

// handleTerminalGetWorkingDirectory 处理获取终端工作目录
func (c *Client) handleTerminalGetWorkingDirectory(sessionID string) {
	c.log.Debug("处理获取终端工作目录: 会话=%s", sessionID)

	workingDir, err := GetTerminalWorkingDirectory(sessionID, c.log)
	if err != nil {
		c.log.Error("获取终端工作目录失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("获取工作目录失败: %v", err))
		return
	}

	response := struct {
		Type       string `json:"type"`
		Session    string `json:"session"`
		WorkingDir string `json:"working_dir"`
	}{
		Type:       "working_directory",
		Session:    sessionID,
		WorkingDir: workingDir,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送工作目录响应失败: %v", err)
	} else {
		c.log.Debug("已发送工作目录响应: 会话=%s, 目录=%s", sessionID, workingDir)
	}
}

// ─── 容器终端处理 ──────────────────────────────────────────────────────────────

func (c *Client) handleContainerTerminalCommand(containerID, sessionID, cmdType, data string, command []string) {
	switch cmdType {
	case "create":
		c.dockerSessionsLock.Lock()
		if _, exists := c.dockerSessions[sessionID]; exists {
			c.dockerSessionsLock.Unlock()
			c.log.Debug("容器终端会话已存在: %s", sessionID)
			return
		}
		c.dockerSessionsLock.Unlock()

		manager, err := monitor.NewDockerManager(c.log)
		if err != nil {
			c.log.Error("创建Docker管理器失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("创建容器终端失败: %v", err))
			return
		}

		execSession, err := manager.StartExecSession(containerID, command)
		if err != nil {
			manager.Close()
			c.log.Error("启动容器终端失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("启动容器终端失败: %v", err))
			return
		}

		sess := &containerExecSession{
			manager:     manager,
			execID:      execSession.ExecID,
			containerID: containerID,
			stopCh:      make(chan struct{}),
		}

		c.dockerSessionsLock.Lock()
		c.dockerSessions[sessionID] = sess
		c.dockerSessionsLock.Unlock()

		c.sendTerminalOutput(sessionID, fmt.Sprintf("已连接到容器 %s\r\n", containerID))
		go c.streamContainerExecOutput(sessionID, sess)

	case "input":
		sess, ok := c.getContainerExecSession(sessionID)
		if !ok {
			c.sendTerminalError(sessionID, "容器终端会话不存在")
			return
		}
		if err := sess.manager.WriteExec(sess.execID, data); err != nil {
			c.log.Error("容器终端写入失败: %v", err)
			c.sendTerminalError(sessionID, fmt.Sprintf("写入失败: %v", err))
		}

	case "resize":
		var dimensions struct {
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if err := json.Unmarshal([]byte(data), &dimensions); err != nil {
			c.log.Error("解析容器终端大小数据失败: %v", err)
			return
		}
		sess, ok := c.getContainerExecSession(sessionID)
		if !ok {
			return
		}
		if err := sess.manager.ResizeExec(sess.execID, uint(dimensions.Cols), uint(dimensions.Rows)); err != nil {
			c.log.Error("调整容器终端大小失败: %v", err)
		}

	case "close":
		c.closeContainerExecSession(sessionID)

	default:
		c.log.Warn("未知的容器终端命令: %s", cmdType)
	}
}

func (c *Client) streamContainerExecOutput(sessionID string, sess *containerExecSession) {
	reader, err := sess.manager.ExecOutput(sess.execID)
	if err != nil {
		c.log.Error("获取容器输出失败: %v", err)
		c.sendTerminalError(sessionID, fmt.Sprintf("容器输出失败: %v", err))
		c.closeContainerExecSession(sessionID)
		return
	}

	buffer := make([]byte, 4096)
	for {
		select {
		case <-sess.stopCh:
			return
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			c.sendTerminalOutput(sessionID, string(buffer[:n]))
		}
		if err != nil {
			if err != io.EOF {
				c.log.Error("读取容器输出失败: %v", err)
			}
			c.closeContainerExecSession(sessionID)
			return
		}
	}
}

func (c *Client) getContainerExecSession(sessionID string) (*containerExecSession, bool) {
	c.dockerSessionsLock.Lock()
	defer c.dockerSessionsLock.Unlock()
	sess, ok := c.dockerSessions[sessionID]
	return sess, ok
}

func (c *Client) closeContainerExecSession(sessionID string) {
	c.dockerSessionsLock.Lock()
	sess, ok := c.dockerSessions[sessionID]
	if ok {
		delete(c.dockerSessions, sessionID)
	}
	c.dockerSessionsLock.Unlock()

	if !ok || sess == nil {
		return
	}

	select {
	case <-sess.stopCh:
	default:
		close(sess.stopCh)
	}

	_ = sess.manager.CloseExec(sess.execID)
	_ = sess.manager.Close()

	c.sendTerminalClose(sessionID)
}

// ─── 终端输出读取 ──────────────────────────────────────────────────────────────

// readTerminalOutput 读取终端输出
func (c *Client) readTerminalOutput(session *TerminalSession) {
	c.log.Debug("开始读取终端输出: 会话=%s", session.ID)

	done := session.Done

	outputChan := make(chan string, 10)
	quitChan := make(chan struct{})

	// 启动专用的输出发送goroutine
	go func() {
		defer close(quitChan)
		for {
			select {
			case output, ok := <-outputChan:
				if !ok {
					return
				}
				c.sendTerminalOutput(session.ID, output)
			case <-done:
				return
			}
		}
	}()

	if session.Pty != nil {
		go func() {
			buffer := make([]byte, 4096)
			defer close(outputChan)
			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Pty.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取PTY输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()
	} else {
		go func() {
			buffer := make([]byte, 4096)
			defer func() {
				select {
				case <-done:
					close(outputChan)
				default:
				}
			}()

			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Stdout.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取终端标准输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()

		go func() {
			buffer := make([]byte, 4096)
			defer func() {
				select {
				case <-done:
					close(outputChan)
				default:
				}
			}()

			for {
				select {
				case <-done:
					return
				default:
					n, err := session.Stderr.Read(buffer)
					if err != nil {
						if err != io.EOF {
							c.log.Error("读取终端标准错误输出失败: %v", err)
						}
						return
					}
					if n > 0 {
						outputChan <- string(buffer[:n])
					}
				}
			}
		}()
	}

	<-done
	c.log.Debug("终端会话已结束: %s", session.ID)

	<-quitChan

	c.sendTerminalClose(session.ID)
}

// ─── 终端消息发送 ──────────────────────────────────────────────────────────────

// sendTerminalOutput 发送终端输出
func (c *Client) sendTerminalOutput(sessionID, output string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端输出")
		return
	}

	response := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Data    string `json:"data"`
	}{
		Type:    "shell_response",
		Session: sessionID,
		Data:    output,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端输出失败: %v", err)
	}
}

// sendTerminalError 发送终端错误
func (c *Client) sendTerminalError(sessionID, errMsg string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端错误")
		return
	}

	response := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Error   string `json:"error"`
	}{
		Type:    "shell_error",
		Session: sessionID,
		Error:   errMsg,
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端错误失败: %v", err)
	}
}

// sendTerminalClose 发送终端关闭消息
func (c *Client) sendTerminalClose(sessionID string) {
	if c.wsConn == nil {
		c.log.Error("WebSocket连接为空，无法发送终端关闭消息")
		return
	}

	response := struct {
		Type    string `json:"type"`
		Session string `json:"session"`
		Message string `json:"message"`
	}{
		Type:    "shell_close",
		Session: sessionID,
		Message: "终端会话已关闭",
	}

	if err := c.writeJSON(response); err != nil {
		c.log.Error("发送终端关闭消息失败: %v", err)
	}
}

// ─── 文件操作处理 ──────────────────────────────────────────────────────────────

// handleFileList 处理文件列表请求
func (c *Client) handleFileList(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path string `json:"path"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析文件列表请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到文件列表请求: 路径=%s", msg.Payload.Path)

	fileManager := NewFileManager(c.log)

	files, err := fileManager.ListFiles(msg.Payload.Path)
	if err != nil {
		c.log.Error("获取文件列表失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取文件列表失败: %v", err),
		})
		return
	}

	c.sendResponse(msg.RequestID, "file_list_response", map[string]interface{}{
		"path":  msg.Payload.Path,
		"files": files,
	})

	c.log.Info("已发送文件列表响应: %d个文件", len(files))
}

// handleFileContent 处理文件内容请求
func (c *Client) handleFileContent(message []byte) {
	var req struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path    string `json:"path"`
			Action  string `json:"action"`
			Content string `json:"content"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &req); err != nil {
		c.log.Error("解析文件内容请求失败: %v", err)
		c.sendResponse(req.RequestID, "error", map[string]interface{}{
			"error": "无效的请求格式",
		})
		return
	}

	c.log.Debug("处理文件内容请求: %s, 路径: %s", req.Payload.Action, req.Payload.Path)

	fileManager := NewFileManager(c.log)

	switch req.Payload.Action {
	case "get":
		content, err := fileManager.GetFileContent(req.Payload.Path)
		if err != nil {
			c.log.Error("获取文件内容失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"content": content,
		})
		c.log.Debug("文件内容获取成功: %s (%d字节)", req.Payload.Path, len(content))

	case "save":
		c.log.Debug("开始保存文件: %s", req.Payload.Path)

		defer func() {
			if r := recover(); r != nil {
				c.log.Error("保存文件时发生严重错误: %v", r)
				c.sendResponse(req.RequestID, "error", map[string]interface{}{
					"error": fmt.Sprintf("保存文件时发生严重错误: %v", r),
				})
			}
		}()

		backupPath := req.Payload.Path + ".bak"
		if _, err := os.Stat(req.Payload.Path); err == nil {
			c.log.Debug("创建文件备份: %s -> %s", req.Payload.Path, backupPath)
			backupContent, readErr := os.ReadFile(req.Payload.Path)
			if readErr == nil {
				_ = os.WriteFile(backupPath, backupContent, 0644)
			}
		}

		if err := fileManager.SaveFileContent(req.Payload.Path, req.Payload.Content); err != nil {
			c.log.Error("保存文件内容失败: %v", err)

			if _, statErr := os.Stat(backupPath); statErr == nil {
				c.log.Info("尝试从备份恢复文件: %s", backupPath)
				if backupContent, readErr := os.ReadFile(backupPath); readErr == nil {
					if writeErr := os.WriteFile(req.Payload.Path, backupContent, 0644); writeErr == nil {
						c.log.Info("成功从备份恢复文件")
					} else {
						c.log.Error("从备份恢复文件失败: %v", writeErr)
					}
				}
			}

			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		if _, err := os.Stat(backupPath); err == nil {
			_ = os.Remove(backupPath)
		}

		c.log.Debug("文件保存成功: %s", req.Payload.Path)
		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "文件保存成功",
		})

	case "create":
		if err := fileManager.CreateFile(req.Payload.Path, req.Payload.Content); err != nil {
			c.log.Error("创建文件失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "文件创建成功",
		})

	case "mkdir":
		if err := fileManager.CreateDirectory(req.Payload.Path); err != nil {
			c.log.Error("创建目录失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		c.sendResponse(req.RequestID, "file_content_response", map[string]interface{}{
			"path":    req.Payload.Path,
			"success": true,
			"message": "目录创建成功",
		})

	case "tree":
		depth := 3
		if req.Payload.Content != "" {
			if parsedDepth, err := strconv.Atoi(req.Payload.Content); err == nil && parsedDepth > 0 {
				depth = parsedDepth
			}
		}

		tree, err := fileManager.GetDirectoryTree(req.Payload.Path, depth)
		if err != nil {
			c.log.Error("获取目录树失败: %v", err)
			c.sendResponse(req.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		c.sendResponse(req.RequestID, "file_tree_response", map[string]interface{}{
			"path": req.Payload.Path,
			"tree": tree,
		})

	default:
		c.log.Error("未知的文件操作: %s", req.Payload.Action)
		c.sendResponse(req.RequestID, "error", map[string]interface{}{
			"error": "未知的文件操作",
		})
	}
}

// handleFileUpload 处理文件上传
func (c *Client) handleFileUpload(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Path     string `json:"path"`
			Filename string `json:"filename"`
			Content  string `json:"content"` // Base64编码的文件内容
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析文件上传请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到文件上传请求: 路径=%s, 文件名=%s", msg.Payload.Path, msg.Payload.Filename)

	fileManager := NewFileManager(c.log)

	err := fileManager.UploadFile(msg.Payload.Path, msg.Payload.Filename, msg.Payload.Content)
	if err != nil {
		c.log.Error("上传文件失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("上传文件失败: %v", err),
		})
		return
	}

	c.sendResponse(msg.RequestID, "file_upload_response", map[string]interface{}{
		"path":     msg.Payload.Path,
		"filename": msg.Payload.Filename,
		"success":  true,
		"message":  "文件上传成功",
	})

	c.log.Info("文件已上传: %s/%s", msg.Payload.Path, msg.Payload.Filename)
}

// ─── 容器文件操作处理 ──────────────────────────────────────────────────────────

// handleDockerFile 处理容器文件操作
func (c *Client) handleDockerFile(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			ContainerID string   `json:"container_id"`
			Path        string   `json:"path"`
			Action      string   `json:"action"`
			Content     string   `json:"content,omitempty"`
			Filename    string   `json:"filename,omitempty"`
			Paths       []string `json:"paths,omitempty"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析容器文件请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的容器文件请求参数",
		})
		return
	}

	if msg.Payload.ContainerID == "" {
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "缺少容器ID",
		})
		return
	}

	manager, err := NewContainerFileManager(c.log, msg.Payload.ContainerID)
	if err != nil {
		c.log.Error("创建容器文件管理器失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("创建容器文件管理器失败: %v", err),
		})
		return
	}
	defer manager.Close()

	switch msg.Payload.Action {
	case "list":
		files, err := manager.ListFiles(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_list", map[string]interface{}{
			"path":  msg.Payload.Path,
			"files": files,
		})

	case "get":
		content, err := manager.GetFileContent(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"content": content,
		})

	case "save":
		if err := manager.SaveFileContent(msg.Payload.Path, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "保存成功",
		})

	case "create":
		if err := manager.CreateFile(msg.Payload.Path, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "创建成功",
		})

	case "mkdir":
		if err := manager.CreateDirectory(msg.Payload.Path); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "目录创建成功",
		})

	case "tree":
		depth := 3
		if msg.Payload.Content != "" {
			if v, err := strconv.Atoi(msg.Payload.Content); err == nil && v > 0 {
				depth = v
			}
		}
		tree, err := manager.GetDirectoryTree(msg.Payload.Path, depth)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_tree", map[string]interface{}{
			"path": msg.Payload.Path,
			"tree": tree,
		})

	case "upload":
		if err := manager.UploadFile(msg.Payload.Path, msg.Payload.Filename, msg.Payload.Content); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_upload", map[string]interface{}{
			"path":     msg.Payload.Path,
			"filename": msg.Payload.Filename,
			"success":  true,
		})

	case "download":
		data, err := manager.DownloadFile(msg.Payload.Path)
		if err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"content": base64.StdEncoding.EncodeToString(data),
		})

	case "delete":
		if len(msg.Payload.Paths) == 0 && msg.Payload.Path != "" {
			msg.Payload.Paths = []string{msg.Payload.Path}
		}
		if err := manager.DeleteFiles(msg.Payload.Paths); err != nil {
			c.sendResponse(msg.RequestID, "error", map[string]interface{}{
				"error": err.Error(),
			})
			return
		}
		c.sendResponse(msg.RequestID, "docker_file_content", map[string]interface{}{
			"path":    msg.Payload.Path,
			"success": true,
			"message": "删除成功",
		})

	default:
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "未知的容器文件操作",
		})
	}
}

// ─── 进程操作处理 ──────────────────────────────────────────────────────────────

// handleProcessList 处理进程列表请求
func (c *Client) handleProcessList(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Action string `json:"action"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析进程列表请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到进程列表请求")

	pm := monitor.NewProcessManager(c.log)

	processes, err := pm.GetProcessList()
	if err != nil {
		c.log.Error("获取进程列表失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取进程列表失败: %v", err),
		})
		return
	}

	c.sendResponse(msg.RequestID, "process_list_response", map[string]interface{}{
		"processes": processes,
		"count":     len(processes),
		"timestamp": time.Now().Unix(),
	})

	c.log.Info("已发送进程列表，共 %d 个进程", len(processes))
}

// handleProcessKill 处理进程终止请求
func (c *Client) handleProcessKill(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			PID int32 `json:"pid"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析进程终止请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到进程终止请求: PID=%d", msg.Payload.PID)

	pm := monitor.NewProcessManager(c.log)

	proc, err := pm.GetProcess(msg.Payload.PID)
	if err != nil {
		c.log.Error("获取进程 %d 信息失败: %v", msg.Payload.PID, err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("获取进程信息失败: %v", err),
		})
		return
	}

	if err := pm.KillProcess(msg.Payload.PID); err != nil {
		c.log.Error("终止进程 %d 失败: %v", msg.Payload.PID, err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("终止进程失败: %v", err),
		})
		return
	}

	c.sendResponse(msg.RequestID, "process_kill_response", map[string]interface{}{
		"pid":       msg.Payload.PID,
		"name":      proc.Name,
		"success":   true,
		"message":   "进程已成功终止",
		"timestamp": time.Now().Unix(),
	})

	c.log.Info("进程 %d(%s) 已成功终止", msg.Payload.PID, proc.Name)
}

// ─── Docker 命令处理 ──────────────────────────────────────────────────────────

// handleDockerCommand 处理Docker命令
func (c *Client) handleDockerCommand(message []byte) {
	var msg struct {
		RequestID string `json:"request_id"`
		Payload   struct {
			Command string          `json:"command"`
			Action  string          `json:"action"`
			Params  json.RawMessage `json:"params,omitempty"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析Docker命令请求失败: %v", err)
		c.sendResponse(msg.RequestID, "error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到Docker命令请求: 操作=%s, 命令=%s", msg.Payload.Action, msg.Payload.Command)

	dockerManager, err := monitor.NewDockerManager(c.log)
	if err != nil {
		c.log.Error("创建Docker管理器失败: %v", err)
		c.sendResponse(msg.RequestID, "docker_error", map[string]interface{}{
			"error": fmt.Sprintf("创建Docker管理器失败: %v", err),
		})
		return
	}
	defer dockerManager.Close()

	switch msg.Payload.Command {
	case "containers":
		c.handleContainersCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	case "images":
		c.handleImagesCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	case "composes":
		c.handleComposesCommand(msg.RequestID, msg.Payload.Action, msg.Payload.Params, dockerManager)
	default:
		c.log.Error("未知的Docker命令: %s", msg.Payload.Command)
		c.sendResponse(msg.RequestID, "docker_error", map[string]interface{}{
			"error": fmt.Sprintf("未知的Docker命令: %s", msg.Payload.Command),
		})
	}
}

// handleContainersCommand 处理容器相关命令
func (c *Client) handleContainersCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		containers, err := dockerManager.GetContainers(true)
		if err != nil {
			c.log.Error("获取容器列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取容器列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_containers", map[string]interface{}{
			"containers": containers,
		})

	case "logs":
		var logParams struct {
			ContainerID string `json:"container_id"`
			Tail        int    `json:"tail"`
		}
		if err := json.Unmarshal(params, &logParams); err != nil {
			c.log.Error("解析容器日志参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的容器日志参数",
			})
			return
		}

		if logParams.Tail <= 0 {
			logParams.Tail = 100
		}

		logs, err := dockerManager.GetContainerLogs(logParams.ContainerID, logParams.Tail)
		if err != nil {
			c.log.Error("获取容器日志失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取容器日志失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_container_logs", map[string]interface{}{
			"logs": logs,
		})

	case "start":
		var startParams struct {
			ContainerID string `json:"container_id"`
		}
		if err := json.Unmarshal(params, &startParams); err != nil {
			c.log.Error("解析启动容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的启动容器参数",
			})
			return
		}

		if err := dockerManager.StartContainer(startParams.ContainerID); err != nil {
			c.log.Error("启动容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("启动容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器启动成功",
		})

	case "stop":
		var stopParams struct {
			ContainerID string `json:"container_id"`
			Timeout     int    `json:"timeout,omitempty"`
		}
		if err := json.Unmarshal(params, &stopParams); err != nil {
			c.log.Error("解析停止容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的停止容器参数",
			})
			return
		}

		if err := dockerManager.StopContainer(stopParams.ContainerID, stopParams.Timeout); err != nil {
			c.log.Error("停止容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("停止容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器停止成功",
		})

	case "restart":
		var restartParams struct {
			ContainerID string `json:"container_id"`
			Timeout     int    `json:"timeout,omitempty"`
		}
		if err := json.Unmarshal(params, &restartParams); err != nil {
			c.log.Error("解析重启容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的重启容器参数",
			})
			return
		}

		if err := dockerManager.RestartContainer(restartParams.ContainerID, restartParams.Timeout); err != nil {
			c.log.Error("重启容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("重启容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器重启成功",
		})

	case "remove":
		var removeParams struct {
			ContainerID string `json:"container_id"`
			Force       bool   `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除容器参数",
			})
			return
		}

		if err := dockerManager.RemoveContainer(removeParams.ContainerID, removeParams.Force); err != nil {
			c.log.Error("删除容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "容器删除成功",
		})

	case "create":
		var createParams struct {
			Name    string            `json:"name"`
			Image   string            `json:"image"`
			Ports   []string          `json:"ports"`
			Volumes []string          `json:"volumes"`
			Env     map[string]string `json:"env"`
			Command string            `json:"command"`
			Restart string            `json:"restart"`
			Network string            `json:"network"`
		}
		if err := json.Unmarshal(params, &createParams); err != nil {
			c.log.Error("解析创建容器参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的创建容器参数",
			})
			return
		}

		containerID, err := dockerManager.CreateContainer(createParams.Name, createParams.Image,
			createParams.Ports, createParams.Volumes, createParams.Env,
			createParams.Command, createParams.Restart, createParams.Network)

		if err != nil {
			c.log.Error("创建容器失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("创建容器失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message":      "容器创建成功",
			"container_id": containerID,
		})

	default:
		c.log.Error("未知的容器操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的容器操作: %s", action),
		})
	}
}

// handleImagesCommand 处理镜像相关命令
func (c *Client) handleImagesCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		images, err := dockerManager.GetImages()
		if err != nil {
			c.log.Error("获取镜像列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取镜像列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_images", map[string]interface{}{
			"images": images,
		})

	case "pull":
		var pullParams struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(params, &pullParams); err != nil {
			c.log.Error("解析拉取镜像参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的拉取镜像参数",
			})
			return
		}

		go func() {
			if err := dockerManager.PullImage(pullParams.Image); err != nil {
				c.log.Error("拉取镜像失败: %v", err)
				return
			}
			c.log.Info("镜像 %s 拉取成功", pullParams.Image)
		}()

		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": fmt.Sprintf("正在拉取镜像: %s，请稍后刷新", pullParams.Image),
		})

	case "remove":
		var removeParams struct {
			ImageID string `json:"image_id"`
			Force   bool   `json:"force,omitempty"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除镜像参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除镜像参数",
			})
			return
		}

		if err := dockerManager.RemoveImage(removeParams.ImageID, removeParams.Force); err != nil {
			c.log.Error("删除镜像失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除镜像失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "镜像删除成功",
		})

	default:
		c.log.Error("未知的镜像操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的镜像操作: %s", action),
		})
	}
}

// handleComposesCommand 处理Compose相关命令
func (c *Client) handleComposesCommand(requestID string, action string, params json.RawMessage, dockerManager *monitor.DockerManager) {
	switch action {
	case "list":
		composes, err := dockerManager.GetComposes()
		if err != nil {
			c.log.Error("获取Compose项目列表失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取Compose项目列表失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_composes", map[string]interface{}{
			"composes": composes,
		})

	case "up":
		var upParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &upParams); err != nil {
			c.log.Error("解析启动Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的启动Compose项目参数",
			})
			return
		}

		if err := dockerManager.ComposeUp(upParams.Name); err != nil {
			c.log.Error("启动Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("启动Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目启动成功",
		})

	case "down":
		var downParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &downParams); err != nil {
			c.log.Error("解析停止Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的停止Compose项目参数",
			})
			return
		}

		if err := dockerManager.ComposeDown(downParams.Name); err != nil {
			c.log.Error("停止Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("停止Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目停止成功",
		})

	case "config":
		var configParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &configParams); err != nil {
			c.log.Error("解析获取Compose配置参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的获取Compose配置参数",
			})
			return
		}

		config, err := dockerManager.GetComposeConfig(configParams.Name)
		if err != nil {
			c.log.Error("获取Compose配置失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("获取Compose配置失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "docker_compose_config", map[string]interface{}{
			"config": config,
		})

	case "create":
		var createParams struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(params, &createParams); err != nil {
			c.log.Error("解析创建Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的创建Compose项目参数",
			})
			return
		}

		if err := dockerManager.CreateCompose(createParams.Name, createParams.Content); err != nil {
			c.log.Error("创建Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("创建Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目创建成功",
		})

	case "remove":
		var removeParams struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &removeParams); err != nil {
			c.log.Error("解析删除Compose项目参数失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": "无效的删除Compose项目参数",
			})
			return
		}

		if err := dockerManager.RemoveCompose(removeParams.Name); err != nil {
			c.log.Error("删除Compose项目失败: %v", err)
			c.sendResponse(requestID, "error", map[string]interface{}{
				"error": fmt.Sprintf("删除Compose项目失败: %v", err),
			})
			return
		}
		c.sendResponse(requestID, "success", map[string]interface{}{
			"message": "Compose项目删除成功",
		})

	default:
		c.log.Error("未知的Compose操作: %s", action)
		c.sendResponse(requestID, "error", map[string]interface{}{
			"error": fmt.Sprintf("未知的Compose操作: %s", action),
		})
	}
}

// ─── Nginx 命令处理 ──────────────────────────────────────────────────────────

// handleNginxCommand 处理Nginx命令
func (c *Client) handleNginxCommand(message []byte) {
	var msg struct {
		RequestID string                 `json:"request_id"`
		Payload   map[string]interface{} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析Nginx命令请求失败: %v", err)
		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": "无效的请求参数",
		})
		return
	}

	c.log.Info("收到Nginx命令请求: RequestID=%s", msg.RequestID)

	action, ok := msg.Payload["action"].(string)
	if !ok {
		c.log.Error("Nginx命令请求缺少action字段")
		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": "请求缺少action字段",
		})
		return
	}

	action = strings.TrimSpace(strings.ToLower(action))

	c.log.Info("处理Nginx命令: %s", action)

	result, err := monitor.HandleNginxCommand(action, msg.Payload)
	if err != nil {
		c.log.Error("执行Nginx命令失败: %v", err)

		c.sendResponse(msg.RequestID, "nginx_error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	c.log.Info("Nginx命令执行成功: %s", action)
	c.log.Debug("Nginx命令执行结果: %s", result)

	c.sendRawResponse(msg.RequestID, "nginx_success", result)
}

// sendRawResponse 发送原始响应，不包装result字段
func (c *Client) sendRawResponse(requestID, responseType, jsonData string) {
	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	if responseType == "success" && requestID != "" {
		responseType = "nginx_success"
	} else if responseType == "error" && requestID != "" {
		responseType = "nginx_error"
	}

	c.log.Debug("发送原始响应，类型: %s, 请求ID: %s", responseType, requestID)

	response := struct {
		Type      string          `json:"type"`
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}{
		Type:      responseType,
		RequestID: requestID,
		Data:      json.RawMessage(jsonData),
	}

	if c.wsConn != nil {
		if err := c.wsConn.WriteJSON(response); err != nil {
			c.log.Error("发送WebSocket响应失败: %v", err)
		}
	} else {
		c.log.Error("WebSocket连接未建立，无法发送响应")
	}
}

// ==================== Docker 日志流 ====================

// handleDockerLogsStream 处理容器日志流请求（start / stop）
func (c *Client) handleDockerLogsStream(message []byte) {
	var msg struct {
		Type    string `json:"type"`
		Payload struct {
			Action      string `json:"action"`
			StreamID    string `json:"stream_id"`
			ContainerID string `json:"container_id"`
			Tail        int    `json:"tail"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.log.Error("解析日志流请求失败: %v", err)
		return
	}

	switch msg.Payload.Action {
	case "start":
		c.startLogStream(msg.Payload.StreamID, msg.Payload.ContainerID, msg.Payload.Tail)
	case "stop":
		c.closeLogStream(msg.Payload.StreamID)
	default:
		c.log.Warn("未知的日志流操作: %s", msg.Payload.Action)
	}
}

// startLogStream 启动一个容器日志流
func (c *Client) startLogStream(streamID, containerID string, tail int) {
	if streamID == "" || containerID == "" {
		c.log.Error("日志流参数不完整: streamID=%s, containerID=%s", streamID, containerID)
		return
	}

	// 检查是否已存在同 ID 的流
	c.logStreamsLock.Lock()
	if _, exists := c.logStreams[streamID]; exists {
		c.logStreamsLock.Unlock()
		c.log.Warn("日志流 %s 已存在，忽略重复 start 请求", streamID)
		return
	}
	c.logStreamsLock.Unlock()

	// 创建独立的 DockerManager（流式连接生命周期独立）
	dockerManager, err := monitor.NewDockerManager(c.log)
	if err != nil {
		c.log.Error("创建Docker管理器失败: %v", err)
		c.sendStreamMessage(streamID, "docker_logs_stream_end", map[string]interface{}{
			"reason": fmt.Sprintf("创建Docker管理器失败: %v", err),
		})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	reader, err := dockerManager.StreamContainerLogs(ctx, containerID, tail)
	if err != nil {
		c.log.Error("启动容器日志流失败: %v", err)
		cancel()
		dockerManager.Close()
		c.sendStreamMessage(streamID, "docker_logs_stream_end", map[string]interface{}{
			"reason": fmt.Sprintf("启动日志流失败: %v", err),
		})
		return
	}

	sess := &logStreamSession{
		reader:      reader,
		cancel:      cancel,
		stopCh:      make(chan struct{}),
		containerID: containerID,
		manager:     dockerManager,
	}

	c.logStreamsLock.Lock()
	c.logStreams[streamID] = sess
	c.logStreamsLock.Unlock()

	c.log.Info("日志流 %s 已启动，容器: %s", streamID, containerID)

	go c.streamDockerLogs(streamID, sess)
}

// streamDockerLogs 在 goroutine 中按行读取日志并发送给后端
func (c *Client) streamDockerLogs(streamID string, sess *logStreamSession) {
	defer c.closeLogStream(streamID)

	scanner := bufio.NewScanner(sess.reader)
	// 设置较大的行缓冲，应对单行很长的日志（如 JSON 日志）
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	// 将阻塞的 Scan 放在独立 goroutine 中，通过 channel 传递行数据
	// 这样主循环的 select 可以及时响应 stopCh 和 ticker
	lineCh := make(chan string, 100)
	scanDone := make(chan error, 1)
	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		scanDone <- scanner.Err()
	}()

	// 批量发送缓冲：每 100ms 或累积 50 行时发送一次，减少消息频率
	var batch []string
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		logs := strings.Join(batch, "\n") + "\n"
		c.sendStreamMessage(streamID, "docker_logs_stream_data", map[string]interface{}{
			"logs": logs,
		})
		batch = batch[:0]
	}

	for {
		select {
		case <-sess.stopCh:
			flushBatch()
			return

		case line, ok := <-lineCh:
			if !ok {
				// channel 关闭，说明 Scan 结束
				flushBatch()
				err := <-scanDone
				if err != nil {
					c.log.Error("读取容器日志流失败 [%s]: %v", streamID, err)
					c.sendStreamMessage(streamID, "docker_logs_stream_end", map[string]interface{}{
						"reason": fmt.Sprintf("读取日志流错误: %v", err),
					})
				} else {
					c.log.Info("容器日志流 %s 已结束（容器可能已停止）", streamID)
					c.sendStreamMessage(streamID, "docker_logs_stream_end", map[string]interface{}{
						"reason": "container_stopped",
					})
				}
				return
			}
			batch = append(batch, line)
			if len(batch) >= 50 {
				flushBatch()
			}

		case <-ticker.C:
			flushBatch()
		}
	}
}

// closeLogStream 关闭指定的日志流并释放所有资源
func (c *Client) closeLogStream(streamID string) {
	c.logStreamsLock.Lock()
	sess, ok := c.logStreams[streamID]
	if ok {
		delete(c.logStreams, streamID)
	}
	c.logStreamsLock.Unlock()

	if !ok || sess == nil {
		return
	}

	// 通知读取 goroutine 退出
	select {
	case <-sess.stopCh:
		// 已关闭
	default:
		close(sess.stopCh)
	}

	// 取消 context 以中断 Docker SDK 的 Follow 阻塞
	sess.cancel()

	// 关闭 reader
	if sess.reader != nil {
		_ = sess.reader.Close()
	}

	// 释放 DockerManager
	if sess.manager != nil {
		_ = sess.manager.Close()
	}

	c.log.Info("日志流 %s 已关闭", streamID)
}

// closeAllLogStreams 关闭所有日志流（Agent 断连时调用）
func (c *Client) closeAllLogStreams() {
	c.logStreamsLock.Lock()
	streamIDs := make([]string, 0, len(c.logStreams))
	for id := range c.logStreams {
		streamIDs = append(streamIDs, id)
	}
	c.logStreamsLock.Unlock()

	for _, id := range streamIDs {
		c.closeLogStream(id)
	}
}

// sendStreamMessage 发送日志流消息（使用 stream_id 而非 request_id）
func (c *Client) sendStreamMessage(streamID, msgType string, data map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("发送日志流消息时 panic: %v", r)
		}
	}()

	msg := map[string]interface{}{
		"type":      msgType,
		"stream_id": streamID,
		"data":      data,
	}

	c.wsWriteMutex.Lock()
	defer c.wsWriteMutex.Unlock()

	if c.wsConn != nil {
		if err := c.wsConn.WriteJSON(msg); err != nil {
			c.log.Error("发送日志流消息失败: streamID=%s, type=%s, error=%v", streamID, msgType, err)
		}
	}
}
