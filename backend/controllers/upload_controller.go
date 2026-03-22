package controllers

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ─── 分片上传常量 ────────────────────────────────────────────────────────────────

const (
	chunkedUploadSessionTTL      = 24 * time.Hour
	chunkedUploadCleanupInterval = 1 * time.Hour
	chunkedUploadRequestTimeout  = 120 * time.Second // 合并大文件可能耗时较长
	maxChunkSizeBytes            = 5 * 1024 * 1024   // 5MB/片
)

// ─── 分片上传会话 ────────────────────────────────────────────────────────────────

// chunkedUploadSession 后端侧的分片上传会话，跟踪状态和已收分片
type chunkedUploadSession struct {
	UploadID    string
	ServerID    uint
	Path        string
	Filename    string
	TotalSize   int64
	ChunkSize   int64
	TotalChunks int
	ContainerID string
	Status      string // initializing | ready | uploading | completing | completed | failed | cancelled
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	mu          sync.Mutex
	receivedSet map[int]struct{}
}

func (s *chunkedUploadSession) markReceived(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receivedSet[index] = struct{}{}
	s.Status = "uploading"
	s.UpdatedAt = time.Now()
}

func (s *chunkedUploadSession) setStatus(status, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	s.LastError = lastErr
	s.UpdatedAt = time.Now()
}

func (s *chunkedUploadSession) receivedChunks() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, 0, len(s.receivedSet))
	for idx := range s.receivedSet {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func (s *chunkedUploadSession) toStatusResponse() gin.H {
	s.mu.Lock()
	defer s.mu.Unlock()

	received := make([]int, 0, len(s.receivedSet))
	for idx := range s.receivedSet {
		received = append(received, idx)
	}
	sort.Ints(received)

	return gin.H{
		"upload_id":       s.UploadID,
		"path":            s.Path,
		"filename":        s.Filename,
		"total_size":      s.TotalSize,
		"chunk_size":      s.ChunkSize,
		"total_chunks":    s.TotalChunks,
		"container_id":    s.ContainerID,
		"status":          s.Status,
		"last_error":      s.LastError,
		"received_chunks": received,
		"created_at":      s.CreatedAt.Unix(),
		"updated_at":      s.UpdatedAt.Unix(),
	}
}

// ─── 全局会话存储 ────────────────────────────────────────────────────────────────

var (
	chunkedUploadSessions    sync.Map // uploadID → *chunkedUploadSession
	chunkedUploadCleanupOnce sync.Once
)

// ─── HTTP Handlers ──────────────────────────────────────────────────────────────

// InitUpload 初始化分片上传会话
func InitUpload(c *gin.Context) {
	startChunkedUploadCleaner()

	serverID, err := parseServerIDFromParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Path        string `json:"path" binding:"required"`
		Filename    string `json:"filename" binding:"required"`
		TotalSize   int64  `json:"total_size" binding:"required"`
		ChunkSize   int64  `json:"chunk_size" binding:"required"`
		TotalChunks int    `json:"total_chunks" binding:"required"`
		ContainerID string `json:"container_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if req.TotalSize <= 0 || req.ChunkSize <= 0 || req.TotalChunks <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "total_size、chunk_size、total_chunks 必须大于0"})
		return
	}
	if req.ChunkSize > int64(maxChunkSizeBytes) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("chunk_size 不能超过 %dMB", maxChunkSizeBytes/1024/1024)})
		return
	}

	uploadID := fmt.Sprintf("chunked_%d_%d", serverID, time.Now().UnixNano())
	now := time.Now()

	session := &chunkedUploadSession{
		UploadID:    uploadID,
		ServerID:    serverID,
		Path:        req.Path,
		Filename:    req.Filename,
		TotalSize:   req.TotalSize,
		ChunkSize:   req.ChunkSize,
		TotalChunks: req.TotalChunks,
		ContainerID: req.ContainerID,
		Status:      "initializing",
		CreatedAt:   now,
		UpdatedAt:   now,
		receivedSet: make(map[int]struct{}),
	}
	chunkedUploadSessions.Store(uploadID, session)

	// 向 Agent 发送初始化请求
	payload := map[string]interface{}{
		"upload_id":    uploadID,
		"path":         req.Path,
		"filename":     req.Filename,
		"total_size":   req.TotalSize,
		"chunk_size":   req.ChunkSize,
		"total_chunks": req.TotalChunks,
		"container_id": req.ContainerID,
	}

	resp, err := sendChunkedRequest(serverID, "chunked_upload_init", payload)
	if err != nil {
		session.setStatus("failed", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if ok, errMsg := checkAgentAck(resp); !ok {
		session.setStatus("failed", errMsg)
		c.JSON(http.StatusBadGateway, gin.H{"error": errMsg})
		return
	}

	session.setStatus("ready", "")
	c.JSON(http.StatusOK, gin.H{
		"upload_id": uploadID,
		"status":    "ready",
	})
}

// UploadChunk 上传单个分片（二进制 body）
func UploadChunk(c *gin.Context) {
	serverID, err := parseServerIDFromParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uploadID := c.Param("upload_id")
	indexStr := c.Param("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分片索引"})
		return
	}

	session, err := loadChunkedSession(uploadID, serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if index >= session.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片索引越界"})
		return
	}

	// 从 header 读取分片哈希（可选）和压缩标志
	chunkHash := strings.TrimSpace(c.GetHeader("X-Chunk-Hash"))
	compressedHeader := strings.TrimSpace(c.GetHeader("X-Chunk-Compressed"))
	chunkCompressed := false
	if compressedHeader != "" {
		if !strings.EqualFold(compressedHeader, "gzip") {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的压缩算法: %s，仅支持 gzip", compressedHeader)})
			return
		}
		chunkCompressed = true
	}

	// 读取二进制 body
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, int64(maxChunkSizeBytes)+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取分片数据失败"})
		return
	}
	if len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片数据为空"})
		return
	}
	if len(body) > maxChunkSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片大小超过限制"})
		return
	}

	// Base64 编码后发送给 Agent（Agent 端通过 JSON 解析）
	payload := map[string]interface{}{
		"upload_id":   uploadID,
		"chunk_index": index,
		"chunk_hash":  chunkHash,
		"compressed":  chunkCompressed,
		"content":     base64.StdEncoding.EncodeToString(body),
	}

	resp, err := sendChunkedRequest(serverID, "chunked_upload_chunk", payload)
	if err != nil {
		session.setStatus("failed", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if ok, errMsg := checkAgentAck(resp); !ok {
		session.setStatus("failed", errMsg)
		c.JSON(http.StatusBadGateway, gin.H{"error": errMsg})
		return
	}

	session.markReceived(index)
	c.JSON(http.StatusOK, gin.H{
		"upload_id":   uploadID,
		"chunk_index": index,
		"success":     true,
	})
}

// GetUploadStatus 查询分片上传状态
func GetUploadStatus(c *gin.Context) {
	serverID, err := parseServerIDFromParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uploadID := c.Param("upload_id")
	session, err := loadChunkedSession(uploadID, serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, session.toStatusResponse())
}

// CompleteUpload 请求合并所有分片
func CompleteUpload(c *gin.Context) {
	serverID, err := parseServerIDFromParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uploadID := c.Param("upload_id")
	session, err := loadChunkedSession(uploadID, serverID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		FileHash string `json:"file_hash"`
	}
	_ = c.ShouldBindJSON(&req) // file_hash 可选

	session.setStatus("completing", "")

	payload := map[string]interface{}{
		"upload_id": uploadID,
		"file_hash": strings.TrimSpace(req.FileHash),
	}

	resp, err := sendChunkedRequest(serverID, "chunked_upload_complete", payload)
	if err != nil {
		session.setStatus("failed", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if ok, errMsg := checkAgentAck(resp); !ok {
		session.setStatus("failed", errMsg)
		c.JSON(http.StatusBadGateway, gin.H{"error": errMsg})
		return
	}

	session.setStatus("completed", "")
	c.JSON(http.StatusOK, gin.H{
		"upload_id": uploadID,
		"status":    "completed",
	})
}

// CancelUpload 取消分片上传
func CancelUpload(c *gin.Context) {
	serverID, err := parseServerIDFromParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uploadID := c.Param("upload_id")
	session, err := loadChunkedSession(uploadID, serverID)
	if err != nil {
		// 幂等取消：会话已不存在（已完成/已取消/已过期）时直接返回成功
		c.JSON(http.StatusOK, gin.H{
			"upload_id": uploadID,
			"status":    "cancelled",
		})
		return
	}

	payload := map[string]interface{}{
		"upload_id": uploadID,
	}

	resp, err := sendChunkedRequest(serverID, "chunked_upload_cancel", payload)
	if err != nil {
		session.setStatus("failed", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	if ok, errMsg := checkAgentAck(resp); !ok {
		session.setStatus("failed", errMsg)
		c.JSON(http.StatusBadGateway, gin.H{"error": errMsg})
		return
	}

	session.setStatus("cancelled", "")
	chunkedUploadSessions.Delete(uploadID)
	c.JSON(http.StatusOK, gin.H{
		"upload_id": uploadID,
		"status":    "cancelled",
	})
}

// ─── 内部辅助函数 ────────────────────────────────────────────────────────────────

// parseServerIDFromParam 从路由参数 :id 解析 serverID
func parseServerIDFromParam(c *gin.Context) (uint, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效的服务器ID")
	}
	return uint(id), nil
}

// loadChunkedSession 加载并校验分片上传会话
func loadChunkedSession(uploadID string, serverID uint) (*chunkedUploadSession, error) {
	if uploadID == "" {
		return nil, fmt.Errorf("缺少 upload_id")
	}
	val, ok := chunkedUploadSessions.Load(uploadID)
	if !ok {
		return nil, fmt.Errorf("上传会话不存在: %s", uploadID)
	}
	session, ok := val.(*chunkedUploadSession)
	if !ok || session == nil {
		chunkedUploadSessions.Delete(uploadID)
		return nil, fmt.Errorf("上传会话无效")
	}
	if session.ServerID != serverID {
		return nil, fmt.Errorf("上传会话不属于当前服务器")
	}
	// 过期检查
	if time.Since(session.CreatedAt) > chunkedUploadSessionTTL {
		chunkedUploadSessions.Delete(uploadID)
		return nil, fmt.Errorf("上传会话已过期")
	}
	return session, nil
}

// sendChunkedRequest 向 Agent 发送分片上传相关的 WebSocket 消息并等待 ACK
func sendChunkedRequest(serverID uint, msgType string, payload map[string]interface{}) (map[string]interface{}, error) {
	// 获取 Agent 连接
	agentConnVal, ok := ActiveAgentConnections.Load(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器Agent未连接")
	}
	agentConn, ok := agentConnVal.(*SafeConn)
	if !ok {
		return nil, fmt.Errorf("服务器连接类型错误")
	}

	// 创建请求ID
	requestID := fmt.Sprintf("%s_%d", msgType, time.Now().UnixNano())

	// 注册响应通道
	respChan := make(chan map[string]interface{}, 1)
	fileRequestMutex.Lock()
	fileRequestMap[requestID] = respChan
	fileRequestMutex.Unlock()

	// 注册待处理请求，Agent 断连时可快速失败
	registerPendingRequest(serverID, requestID)
	defer unregisterPendingRequest(serverID, requestID)

	// 构造消息
	request := map[string]interface{}{
		"type":       msgType,
		"request_id": requestID,
		"payload":    payload,
	}

	// 发送
	if err := agentConn.WriteJSON(request); err != nil {
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}

	// 等待 Agent ACK 或超时
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(chunkedUploadRequestTimeout):
		fileRequestMutex.Lock()
		delete(fileRequestMap, requestID)
		fileRequestMutex.Unlock()
		return nil, fmt.Errorf("请求超时")
	}
}

// checkAgentAck 校验 Agent ACK 响应是否成功
func checkAgentAck(resp map[string]interface{}) (bool, string) {
	// HandleFileResponse 传入的格式: {"type": "xxx_ack", "data": {...}}
	respType, _ := resp["type"].(string)
	if respType == "error" {
		errMsg, _ := resp["error"].(string)
		if errMsg == "" {
			errMsg = "Agent 返回错误"
		}
		return false, errMsg
	}

	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		return false, "Agent 返回无效的 ACK 数据"
	}

	success, _ := data["success"].(bool)
	if !success {
		errMsg, _ := data["error"].(string)
		if errMsg == "" {
			errMsg = "Agent 返回失败"
		}
		return false, errMsg
	}

	return true, ""
}

// startChunkedUploadCleaner 启动后台过期会话清理（仅执行一次）
func startChunkedUploadCleaner() {
	chunkedUploadCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(chunkedUploadCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				cleanupExpiredChunkedSessions()
			}
		}()
	})
}

// cleanupExpiredChunkedSessions 清理过期的分片上传会话
func cleanupExpiredChunkedSessions() {
	now := time.Now()
	chunkedUploadSessions.Range(func(key, value interface{}) bool {
		session, ok := value.(*chunkedUploadSession)
		if !ok || session == nil || now.Sub(session.CreatedAt) > chunkedUploadSessionTTL {
			chunkedUploadSessions.Delete(key)
		}
		return true
	})
}
