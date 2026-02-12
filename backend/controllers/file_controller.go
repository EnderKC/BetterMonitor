package controllers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

// 请求响应映射和锁
var (
	fileRequestMutex   sync.Mutex
	fileRequestMap     = make(map[string]chan map[string]interface{})
	fileRequestTimeout = TimeoutFileOperation
)

// 外部引用WebSocket控制器的变量已在websocket_controller.go中定义
// 使用ActiveAgentConnections直接引用

// FileInfo 文件信息结构体
type FileInfo struct {
	Name     string      `json:"name"`               // 文件名
	Size     int64       `json:"size"`               // 文件大小
	ModTime  string      `json:"mod_time"`           // 修改时间
	IsDir    bool        `json:"is_dir"`             // 是否是目录
	Mode     string      `json:"mode"`               // 文件权限
	Children []*FileInfo `json:"children,omitempty"` // 子文件（目录树使用）
}

// 响应结构
type FileResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// GetFileList 获取文件列表
func GetFileList(c *gin.Context) {
	serverID := c.Param("id")
	path := c.Query("path")

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 通过WebSocket获取文件列表
	result, err := requestFileListViaWebSocket(server.ID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取文件列表失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetFileTree 获取文件目录树
func GetFileTree(c *gin.Context) {
	serverID := c.Param("id")
	depth := c.DefaultQuery("depth", "3") // 默认递归深度为3

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 通过WebSocket获取文件树
	result, err := requestFileTreeViaWebSocket(server.ID, depth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取文件树失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetFileContent 获取文件内容
func GetFileContent(c *gin.Context) {
	serverID := c.Param("id")
	path := c.Query("path")

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证文件路径
	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	// 通过WebSocket获取文件内容
	content, err := requestFileContentViaWebSocket(server.ID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取文件内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, content)
}

// SaveFileContent 保存文件内容
func SaveFileContent(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证文件路径
	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	// 通过WebSocket保存文件内容
	err := saveFileContentViaWebSocket(server.ID, req.Path, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存文件内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件保存成功"})
}

// CreateFile 创建文件
func CreateFile(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证文件路径
	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	// 通过WebSocket创建文件
	err := createFileViaWebSocket(server.ID, req.Path, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件创建成功"})
}

// CreateDirectory 创建目录
func CreateDirectory(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Path string `json:"path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证目录路径
	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的目录路径"})
		return
	}

	// 通过WebSocket创建目录
	err := createDirectoryViaWebSocket(server.ID, req.Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建目录失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "目录创建成功"})
}

// UploadFile 上传文件
func UploadFile(c *gin.Context) {
	serverID := c.Param("id")

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 获取上传路径
	path := c.PostForm("path")
	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	// 获取上传的文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "获取上传文件失败"})
		return
	}
	defer file.Close()

	// 检查文件大小
	if header.Size > 50*1024*1024 { // 50MB
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件太大，最大允许50MB"})
		return
	}

	// 读取文件内容
	fileContent, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上传文件失败"})
		return
	}

	// 构造目标文件路径
	targetPath := filepath.Join(path, header.Filename)
	targetPath = filepath.Clean(targetPath)

	// 通过WebSocket上传文件
	err = uploadFileViaWebSocket(server.ID, targetPath, fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("上传文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件上传成功"})
}

// DownloadFile 下载文件
func DownloadFile(c *gin.Context) {
	serverID := c.Param("id")
	path := c.Query("path")
	token := c.Query("token")

	// 验证token
	claims, err := utils.ParseToken(token)
	if err != nil || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权，请重新登录"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证文件路径
	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	// 通过WebSocket获取文件内容
	fileData, err := downloadFileViaWebSocket(server.ID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("下载文件失败: %v", err)})
		return
	}

	// 设置文件名
	filename := filepath.Base(path)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", len(fileData)))

	// 发送文件
	c.Data(http.StatusOK, "application/octet-stream", fileData)
}

// DeleteFiles 删除文件或目录
func DeleteFiles(c *gin.Context) {
	serverID := c.Param("id")

	var req struct {
		Paths []string `json:"paths"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证所有路径
	for _, path := range req.Paths {
		if !isValidFilePath(path) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的文件路径: %s", path)})
			return
		}
	}

	// 通过WebSocket删除文件
	err := deleteFilesViaWebSocket(server.ID, req.Paths)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件删除成功"})
}

// ---------------- 容器文件管理 ----------------

// GetContainerFileList 获取容器文件列表
func GetContainerFileList(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")
	path := c.DefaultQuery("path", "/")

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	result, err := requestContainerFileListViaWebSocket(server.ID, containerID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取容器文件列表失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetContainerDirectoryChildren 获取容器目录的直接子目录
func GetContainerDirectoryChildren(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")
	path := c.DefaultQuery("path", "/")

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的目录路径"})
		return
	}

	result, err := requestContainerDirectoryChildrenViaWebSocket(server.ID, containerID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取容器目录子节点失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetContainerFileContent 获取容器文件内容
func GetContainerFileContent(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")
	path := c.Query("path")

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	content, err := requestContainerFileContentViaWebSocket(server.ID, containerID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取文件内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, content)
}

// SaveContainerFileContent 保存容器文件内容
func SaveContainerFileContent(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	if err := saveContainerFileContentViaWebSocket(server.ID, containerID, req.Path, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存文件内容失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件保存成功"})
}

// CreateContainerFile 创建容器文件
func CreateContainerFile(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	if err := createContainerFileViaWebSocket(server.ID, containerID, req.Path, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件创建成功"})
}

// CreateContainerDirectory 创建容器目录
func CreateContainerDirectory(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")

	var req struct {
		Path string `json:"path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的目录路径"})
		return
	}

	if err := createContainerDirectoryViaWebSocket(server.ID, containerID, req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建目录失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "目录创建成功"})
}

// 容器文件上传大小限制（100MB，容器场景可能需要上传较大文件）
const maxContainerUploadSize int64 = 100 * 1024 * 1024

// UploadContainerFile 上传容器文件
func UploadContainerFile(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 【安全修复】限制请求体大小，防止DoS攻击
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxContainerUploadSize)

	path := c.PostForm("path")
	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// 【安全修复】使用 errors.As 进行类型匹配，替代脆弱的字符串比较
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("文件太大，最大允许%dMB", maxContainerUploadSize/1024/1024)})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "获取上传文件失败"})
		return
	}
	defer file.Close()

	// 【安全修复】检查文件大小
	if header.Size <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件内容为空"})
		return
	}
	if header.Size > maxContainerUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("文件太大，最大允许%dMB", maxContainerUploadSize/1024/1024)})
		return
	}

	// 使用LimitReader作为额外保护
	limitedReader := io.LimitReader(file, maxContainerUploadSize+1)
	fileContent, err := io.ReadAll(limitedReader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上传文件失败"})
		return
	}

	// 检查实际读取的大小
	if int64(len(fileContent)) > maxContainerUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("文件太大，最大允许%dMB", maxContainerUploadSize/1024/1024)})
		return
	}

	filename := strings.TrimSpace(header.Filename)
	filename = strings.ReplaceAll(filename, "\\", "/")
	filename = filepath.Base(filename)
	if filename == "" || filename == "." || filename == "/" || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件名"})
		return
	}

	targetPath := filepath.Join(path, filename)
	targetPath = filepath.Clean(targetPath)
	if !isValidFilePath(targetPath) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	if err := uploadContainerFileViaWebSocket(server.ID, containerID, targetPath, fileContent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("上传文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件上传成功"})
}

// DownloadContainerFile 下载容器文件
func DownloadContainerFile(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")
	path := c.Query("path")
	token := c.Query("token")

	claims, err := utils.ParseToken(token)
	if err != nil || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权，请重新登录"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的文件路径"})
		return
	}

	fileData, err := downloadContainerFileViaWebSocket(server.ID, containerID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("下载文件失败: %v", err)})
		return
	}

	filename := filepath.Base(path)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", len(fileData)))
	c.Data(http.StatusOK, "application/octet-stream", fileData)
}

// DeleteContainerFiles 删除容器文件
func DeleteContainerFiles(c *gin.Context) {
	serverID := c.Param("id")
	containerID := c.Param("container_id")

	var req struct {
		Paths []string `json:"paths"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	for _, path := range req.Paths {
		if !isValidFilePath(path) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的文件路径: %s", path)})
			return
		}
	}

	if err := deleteContainerFilesViaWebSocket(server.ID, containerID, req.Paths); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除文件失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "文件删除成功"})
}

// GetDirectoryChildren 获取指定目录的直接子目录
func GetDirectoryChildren(c *gin.Context) {
	serverID := c.Param("id")
	path := c.DefaultQuery("path", "/")

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器离线"})
		return
	}

	// 验证路径
	if !isValidFilePath(path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的路径"})
		return
	}

	// 通过WebSocket获取目录子节点（深度为1）
	result, err := requestDirectoryChildrenViaWebSocket(server.ID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取目录子节点失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// 通过WebSocket获取文件列表
func requestFileListViaWebSocket(serverID uint, path string) ([]FileInfo, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_list_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_list",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path": path,
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		fileListData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}

		filesData, ok := fileListData["files"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的文件列表格式")
		}

		// 转换文件列表
		files := make([]FileInfo, 0, len(filesData))
		for _, fileData := range filesData {
			fileMap, ok := fileData.(map[string]interface{})
			if !ok {
				continue
			}

			file := FileInfo{
				Name:    getString(fileMap, "name"),
				Size:    getInt64(fileMap, "size"),
				ModTime: getString(fileMap, "mod_time"),
				IsDir:   getBool(fileMap, "is_dir"),
				Mode:    getString(fileMap, "mode"),
			}

			files = append(files, file)
		}

		return files, nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

// 通过WebSocket获取文件树
func requestFileTreeViaWebSocket(serverID uint, depth string) ([]*FileInfo, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_tree_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":    "/", // 从根目录开始
			"action":  "tree",
			"content": depth, // 将深度作为内容传递
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		treeData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}

		// 使用辅助函数递归转换文件树
		treeArray, ok := treeData["tree"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的文件树格式")
		}

		var result []*FileInfo
		for _, item := range treeArray {
			if fileItem, ok := item.(map[string]interface{}); ok {
				fi := convertToFileInfo(fileItem)
				result = append(result, fi)
			}
		}

		return result, nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

// 通过WebSocket获取文件内容
func requestFileContentViaWebSocket(serverID uint, path string) (string, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return "", fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return "", fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_content_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":   path,
			"action": "get",
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return "", fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return "", fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		contentData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("无效的响应格式")
		}

		content, ok := contentData["content"].(string)
		if !ok {
			return "", fmt.Errorf("无效的文件内容格式")
		}

		return content, nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return "", fmt.Errorf("请求超时")
	}
}

// 辅助函数：递归转换文件信息
func convertToFileInfo(data map[string]interface{}) *FileInfo {
	fileInfo := &FileInfo{
		Name:    getString(data, "name"),
		Size:    getInt64(data, "size"),
		ModTime: getString(data, "mod_time"),
		IsDir:   getBool(data, "is_dir"),
		Mode:    getString(data, "mode"),
	}

	// 处理子文件
	if children, ok := data["children"].([]interface{}); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				fileInfo.Children = append(fileInfo.Children, convertToFileInfo(childMap))
			}
		}
	}

	return fileInfo
}

// 获取map中的字符串值
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// 获取map中的int64值
func getInt64(data map[string]interface{}, key string) int64 {
	switch v := data[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

// 获取map中的布尔值
func getBool(data map[string]interface{}, key string) bool {
	if val, ok := data[key].(bool); ok {
		return val
	}
	return false
}

// 处理文件相关的WebSocket响应
func HandleFileResponse(requestID string, data map[string]interface{}) {
	fileRequestMutex.Lock()
	defer fileRequestMutex.Unlock()

	// 查找请求回调
	respChan, ok := fileRequestMap[requestID]
	if !ok {
		// 请求可能已超时或被取消
		return
	}

	// 发送响应
	select {
	case respChan <- data:
		// 响应已发送
	default:
		// 通道已满或已关闭
	}

	// 删除请求通道
	delete(fileRequestMap, requestID)
}

// 通过WebSocket保存文件内容
func saveFileContentViaWebSocket(serverID uint, path string, content string) error {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_save_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":    path,
			"action":  "save",
			"content": content,
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		return nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// 通过WebSocket创建文件
func createFileViaWebSocket(serverID uint, path string, content string) error {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_create_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":    path,
			"action":  "create",
			"content": content,
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		return nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// 通过WebSocket创建目录
func createDirectoryViaWebSocket(serverID uint, path string) error {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("dir_create_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":   path,
			"action": "mkdir",
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		return nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// 通过WebSocket上传文件
func uploadFileViaWebSocket(serverID uint, path string, content []byte) error {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_upload_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// Base64编码文件内容
	base64Content := base64.StdEncoding.EncodeToString(content)

	// 提取文件名
	filename := filepath.Base(path)
	// 提取目录
	dir := filepath.Dir(path)
	if dir == "." {
		dir = "/"
	}

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_upload",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":     dir,
			"filename": filename,
			"content":  base64Content,
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		return nil

	case <-time.After(fileRequestTimeout): // 上传可能需要更长时间
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// 通过WebSocket下载文件
func downloadFileViaWebSocket(serverID uint, path string) ([]byte, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_download_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息 - 这里使用file_content消息类型的"download"操作
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":   path,
			"action": "download",
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		contentData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}

		content, ok := contentData["content"].(string)
		if !ok {
			return nil, fmt.Errorf("无效的文件内容格式")
		}

		// 解码Base64内容
		fileData, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("解码文件内容失败: %v", err)
		}

		return fileData, nil

	case <-time.After(fileRequestTimeout): // 下载可能需要更长时间
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

// 通过WebSocket删除文件
func deleteFilesViaWebSocket(serverID uint, paths []string) error {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("file_delete_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 将路径列表转为JSON字符串
	pathsJSON, err := json.Marshal(paths)
	if err != nil {
		return fmt.Errorf("序列化路径列表失败: %v", err)
	}

	// 构造请求消息
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":    "", // 路径列表在content中
			"action":  "delete",
			"content": string(pathsJSON),
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		return nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// 通过WebSocket获取指定目录的直接子目录
func requestDirectoryChildrenViaWebSocket(serverID uint, path string) ([]*FileInfo, error) {
	// 获取Agent连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}

	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("dir_children_%d", time.Now().UnixNano())

	// 创建响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造请求消息 - 使用深度1来只获取直接子目录
	request := map[string]interface{}{
		"type":       "file_content",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"path":    path,
			"action":  "tree",
			"content": "1", // 深度为1，只获取直接子目录
		},
	}

	// 发送请求
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respChan:
		// 处理响应
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}

		treeData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}

		// 使用辅助函数转换文件树
		treeArray, ok := treeData["tree"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的文件树格式")
		}

		var result []*FileInfo
		for _, item := range treeArray {
			if fileItem, ok := item.(map[string]interface{}); ok {
				fi := convertToFileInfo(fileItem)
				// 对于动态加载，只返回目录，不包含子目录信息
				if fi.IsDir {
					fi.Children = nil // 清除子目录信息，强制动态加载
				}
				result = append(result, fi)
			}
		}

		return result, nil

	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

// ---------------- 容器文件 WebSocket 请求封装 ----------------

func requestContainerFileListViaWebSocket(serverID uint, containerID string, path string) ([]FileInfo, error) {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_list_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         path,
			"action":       "list",
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		data, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}
		filesData, ok := data["files"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的文件列表格式")
		}
		files := make([]FileInfo, 0, len(filesData))
		for _, fileData := range filesData {
			if fileMap, ok := fileData.(map[string]interface{}); ok {
				files = append(files, FileInfo{
					Name:    getString(fileMap, "name"),
					Size:    getInt64(fileMap, "size"),
					ModTime: getString(fileMap, "mod_time"),
					IsDir:   getBool(fileMap, "is_dir"),
					Mode:    getString(fileMap, "mode"),
				})
			}
		}
		return files, nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

func requestContainerDirectoryChildrenViaWebSocket(serverID uint, containerID, path string) ([]*FileInfo, error) {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_dir_children_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         path,
			"action":       "tree",
			"content":      "1",
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		treeData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}
		treeArray, ok := treeData["tree"].([]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的文件树格式")
		}
		var result []*FileInfo
		for _, item := range treeArray {
			if fileItem, ok := item.(map[string]interface{}); ok {
				fi := convertToFileInfo(fileItem)
				if fi.IsDir {
					fi.Children = nil
				}
				result = append(result, fi)
			}
		}
		return result, nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

func requestContainerFileContentViaWebSocket(serverID uint, containerID, path string) (string, error) {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return "", fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return "", fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_content_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         path,
			"action":       "get",
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return "", fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return "", fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		contentData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("无效的响应格式")
		}
		content, ok := contentData["content"].(string)
		if !ok {
			return "", fmt.Errorf("无效的文件内容格式")
		}
		return content, nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return "", fmt.Errorf("请求超时")
	}
}

func saveContainerFileContentViaWebSocket(serverID uint, containerID, path, content string) error {
	return genericContainerFileContentAction(serverID, containerID, path, "save", content)
}

func createContainerFileViaWebSocket(serverID uint, containerID, path, content string) error {
	return genericContainerFileContentAction(serverID, containerID, path, "create", content)
}

func createContainerDirectoryViaWebSocket(serverID uint, containerID, path string) error {
	return genericContainerFileContentAction(serverID, containerID, path, "mkdir", "")
}

func deleteContainerFilesViaWebSocket(serverID uint, containerID string, paths []string) error {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_delete_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	pathsJSON, err := json.Marshal(paths)
	if err != nil {
		return fmt.Errorf("序列化路径列表失败: %v", err)
	}

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         "",
			"action":       "delete",
			"content":      string(pathsJSON),
			"paths":        paths,
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		return nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

func uploadContainerFileViaWebSocket(serverID uint, containerID, path string, content []byte) error {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_upload_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	base64Content := base64.StdEncoding.EncodeToString(content)
	filename := filepath.Base(path)
	dir := filepath.Dir(path)
	if dir == "." {
		dir = "/"
	}

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         dir,
			"filename":     filename,
			"content":      base64Content,
			"action":       "upload",
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		return nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

func downloadContainerFileViaWebSocket(serverID uint, containerID, path string) ([]byte, error) {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_download_%d", time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         path,
			"action":       "download",
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return nil, fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		contentData, ok := resp["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("无效的响应格式")
		}
		content, ok := contentData["content"].(string)
		if !ok {
			return nil, fmt.Errorf("无效的文件内容格式")
		}
		fileData, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("解码文件内容失败: %v", err)
		}
		return fileData, nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return nil, fmt.Errorf("请求超时")
	}
}

func genericContainerFileContentAction(serverID uint, containerID, path, action, content string) error {
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return fmt.Errorf("服务器连接类型错误")
	}

	requestID := fmt.Sprintf("docker_file_%s_%d", action, time.Now().UnixNano())
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 【安全修复】注册待处理请求，以便在Agent断开时能快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	request := map[string]interface{}{
		"type":       "docker_file",
		"request_id": requestID,
		"payload": map[string]interface{}{
			"container_id": containerID,
			"path":         path,
			"action":       action,
			"content":      content,
		},
	}

	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("发送请求失败: %v", err)
	}

	select {
	case resp := <-respChan:
		if resp["type"] == "error" {
			return fmt.Errorf("Agent返回错误: %v", resp["error"])
		}
		return nil
	case <-time.After(fileRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()

		return fmt.Errorf("请求超时")
	}
}

// isValidFilePath 验证文件路径是否合法
// 安全检查：在Clean之前先检查原始路径中的traversal段，防止目录穿越攻击
func isValidFilePath(path string) bool {
	if path == "" {
		return false
	}

	// 【关键】在Clean之前检查原始路径中的traversal段
	// 因为Clean会将 "../" 规范化，导致后续检查失效
	// 例如："/www/../etc/passwd" 会被Clean为 "/etc/passwd"
	normalizedSlash := filepath.ToSlash(path)
	for _, seg := range strings.Split(normalizedSlash, "/") {
		if seg == ".." {
			return false
		}
	}

	// 清理路径
	cleanPath := filepath.Clean(path)

	// 再次检查Clean后的路径（双重保险）
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return false
	}

	// 禁止访问的敏感路径列表
	bannedPaths := []string{
		"/etc/shadow",
		"/etc/passwd",
		"/etc/sudoers",
		"/etc/gshadow",
		"/etc/security",
		"/root/.ssh",
		"/home/.ssh",
		"/proc",
		"/sys",
	}

	// 检查是否在禁止访问的目录中
	for _, banned := range bannedPaths {
		if cleanPath == banned || strings.HasPrefix(cleanPath, banned+"/") {
			return false
		}
	}

	return true
}

// validatePathInBaseDir 验证用户路径是否在指定的基础目录内
// 用于需要限制访问范围的场景（如容器文件操作）
func validatePathInBaseDir(baseDir, userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("路径不能为空")
	}

	// 先检查原始路径中的traversal段
	normalizedSlash := filepath.ToSlash(userPath)
	for _, seg := range strings.Split(normalizedSlash, "/") {
		if seg == ".." {
			return "", fmt.Errorf("路径包含非法字符")
		}
	}

	// 构造完整路径
	var fullPath string
	if filepath.IsAbs(userPath) {
		fullPath = filepath.Clean(userPath)
	} else {
		fullPath = filepath.Clean(filepath.Join(baseDir, userPath))
	}

	// 获取基础目录的绝对路径
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("无效的基础目录")
	}

	// 获取目标路径的绝对路径
	absTarget, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("无效的目标路径")
	}

	// 验证目标路径是否在基础目录内
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return "", fmt.Errorf("路径验证失败")
	}

	// 如果相对路径以..开头，说明目标路径不在基础目录内
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界")
	}

	return absTarget, nil
}
