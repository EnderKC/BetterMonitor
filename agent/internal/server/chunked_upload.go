//go:build !monitor_only

package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/user/server-ops-agent/pkg/logger"
)

const (
	chunkedUploadTTL     = 24 * time.Hour
	chunkedCleanupPeriod = 1 * time.Hour
)

// ChunkedUploadSession 分片上传会话
type ChunkedUploadSession struct {
	UploadID    string
	Path        string // 目标目录
	Filename    string // 目标文件名
	TotalSize   int64
	TotalChunks int
	ChunkSize   int64
	TempDir     string       // 临时分片存储目录
	Received    map[int]bool // 已接收分片索引
	ContainerID string       // 非空则为容器上传
	CreatedAt   time.Time
	completing  bool         // 标记是否正在合并，阻止新分片写入
	mu          sync.Mutex   // 保护 Received 和 completing 字段
}

// ChunkedUploadManager 管理多个分片上传会话
type ChunkedUploadManager struct {
	mu       sync.RWMutex
	sessions map[string]*ChunkedUploadSession
	log      *logger.Logger
	stopCh   chan struct{}
	once     sync.Once
}

// NewChunkedUploadManager 创建分片上传管理器
func NewChunkedUploadManager(log *logger.Logger) *ChunkedUploadManager {
	return &ChunkedUploadManager{
		sessions: make(map[string]*ChunkedUploadSession),
		log:      log,
		stopCh:   make(chan struct{}),
	}
}

// Init 初始化一个分片上传会话，创建临时目录
func (m *ChunkedUploadManager) Init(uploadID, path, filename string, totalSize, chunkSize int64, totalChunks int, containerID string) error {
	if uploadID == "" || path == "" || filename == "" {
		return fmt.Errorf("uploadID, path, filename 不能为空")
	}
	if totalSize <= 0 || chunkSize <= 0 || totalChunks <= 0 {
		return fmt.Errorf("totalSize, chunkSize, totalChunks 必须大于0")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[uploadID]; exists {
		return fmt.Errorf("上传会话已存在: %s", uploadID)
	}

	tempDir, err := os.MkdirTemp("", "bm_chunk_"+uploadID+"_")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	m.sessions[uploadID] = &ChunkedUploadSession{
		UploadID:    uploadID,
		Path:        path,
		Filename:    filename,
		TotalSize:   totalSize,
		TotalChunks: totalChunks,
		ChunkSize:   chunkSize,
		TempDir:     tempDir,
		Received:    make(map[int]bool, totalChunks),
		ContainerID: containerID,
		CreatedAt:   time.Now(),
	}

	m.log.Info("分片上传会话已初始化: id=%s, file=%s, chunks=%d", uploadID, filename, totalChunks)
	return nil
}

// SaveChunk 保存一个分片到临时文件，并校验 SHA-256 哈希
// 当 compressed=true 时，哈希校验针对传输数据（压缩后），然后解压再存储
func (m *ChunkedUploadManager) SaveChunk(uploadID string, index int, data []byte, hash string, compressed bool) error {
	session, err := m.getSession(uploadID)
	if err != nil {
		return err
	}

	// 检查会话是否正在合并
	session.mu.Lock()
	if session.completing {
		session.mu.Unlock()
		return fmt.Errorf("上传会话正在合并中，无法接收新分片")
	}
	session.mu.Unlock()

	if index < 0 || index >= session.TotalChunks {
		return fmt.Errorf("分片索引越界: %d (总共 %d)", index, session.TotalChunks)
	}

	if len(data) == 0 {
		return fmt.Errorf("分片数据为空")
	}

	// 校验传输数据的哈希（压缩场景下为压缩后的字节，保证传输完整性）
	if hash != "" {
		sum := sha256.Sum256(data)
		actual := hex.EncodeToString(sum[:])
		if actual != hash {
			return fmt.Errorf("分片哈希校验失败: index=%d", index)
		}
	}

	// 如果分片经过 gzip 压缩，先解压再存储
	writeData := data
	if compressed {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("创建 gzip reader 失败: index=%d: %w", index, err)
		}

		// 防止 gzip bomb：解压后大小不应超过原始分片大小上限
		// 最后一片可能小于 ChunkSize，但解压后不应超过 ChunkSize
		maxDecompressed := session.ChunkSize + 1
		limited := io.LimitReader(gr, maxDecompressed)
		decompressed, err := io.ReadAll(limited)
		gr.Close()
		if err != nil {
			return fmt.Errorf("解压分片失败: index=%d: %w", index, err)
		}
		if int64(len(decompressed)) >= maxDecompressed {
			return fmt.Errorf("解压后分片过大，疑似异常数据: index=%d", index)
		}
		writeData = decompressed
	}

	// 幂等：如果已存在且重复写入，直接覆盖
	chunkPath := filepath.Join(session.TempDir, fmt.Sprintf("%06d.part", index))
	if err := os.WriteFile(chunkPath, writeData, 0644); err != nil {
		return fmt.Errorf("写入分片失败: %w", err)
	}

	session.mu.Lock()
	session.Received[index] = true
	session.mu.Unlock()

	return nil
}

// Complete 合并所有分片为最终文件
func (m *ChunkedUploadManager) Complete(uploadID, fileHash string) error {
	session, err := m.getSession(uploadID)
	if err != nil {
		return err
	}

	// 标记为合并中，阻止新分片写入
	session.mu.Lock()
	if session.completing {
		session.mu.Unlock()
		return fmt.Errorf("上传会话已在合并中")
	}
	session.completing = true
	// 校验所有分片已接收
	for i := 0; i < session.TotalChunks; i++ {
		if !session.Received[i] {
			session.completing = false
			session.mu.Unlock()
			return fmt.Errorf("缺少分片 index=%d (已收到 %d/%d)", i, len(session.Received), session.TotalChunks)
		}
	}
	session.mu.Unlock()

	// 合并分片到临时文件
	mergedPath := filepath.Join(session.TempDir, "merged_"+session.Filename)
	if err := m.mergeChunks(session, mergedPath); err != nil {
		return err
	}

	// 校验最终文件哈希
	if fileHash != "" {
		ok, err := verifyFileHash(mergedPath, fileHash)
		if err != nil {
			return fmt.Errorf("校验文件哈希失败: %w", err)
		}
		if !ok {
			return fmt.Errorf("最终文件哈希不匹配")
		}
	}

	// 写入最终位置
	if session.ContainerID == "" {
		if err := m.completeToHost(session, mergedPath); err != nil {
			return err
		}
	} else {
		if err := m.completeToContainer(session, mergedPath); err != nil {
			return err
		}
	}

	m.log.Info("分片上传完成: id=%s, file=%s/%s", uploadID, session.Path, session.Filename)

	// 清理临时文件
	m.mu.Lock()
	delete(m.sessions, uploadID)
	m.mu.Unlock()
	_ = os.RemoveAll(session.TempDir)

	return nil
}

// Cancel 取消上传并清理临时文件
func (m *ChunkedUploadManager) Cancel(uploadID string) error {
	m.mu.Lock()
	session, ok := m.sessions[uploadID]
	if ok {
		delete(m.sessions, uploadID)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("上传会话不存在: %s", uploadID)
	}

	m.log.Info("分片上传已取消: id=%s", uploadID)
	return os.RemoveAll(session.TempDir)
}

// GetStatus 返回已接收的分片状态
func (m *ChunkedUploadManager) GetStatus(uploadID string) (map[int]bool, error) {
	session, err := m.getSession(uploadID)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[int]bool, len(session.Received))
	for k, v := range session.Received {
		result[k] = v
	}
	return result, nil
}

// StartCleanup 启动后台清理过期会话的 goroutine
func (m *ChunkedUploadManager) StartCleanup() {
	m.once.Do(func() {
		go func() {
			ticker := time.NewTicker(chunkedCleanupPeriod)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					m.cleanupExpired()
				case <-m.stopCh:
					return
				}
			}
		}()
	})
}

// StopCleanup 停止清理 goroutine（可安全多次调用）
func (m *ChunkedUploadManager) StopCleanup() {
	select {
	case <-m.stopCh:
		// 已关闭
	default:
		close(m.stopCh)
	}
}

func (m *ChunkedUploadManager) getSession(uploadID string) (*ChunkedUploadSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[uploadID]
	if !ok {
		return nil, fmt.Errorf("上传会话不存在: %s", uploadID)
	}
	return session, nil
}

func (m *ChunkedUploadManager) mergeChunks(session *ChunkedUploadSession, mergedPath string) error {
	target, err := os.Create(mergedPath)
	if err != nil {
		return fmt.Errorf("创建合并文件失败: %w", err)
	}
	defer target.Close()

	for i := 0; i < session.TotalChunks; i++ {
		chunkPath := filepath.Join(session.TempDir, fmt.Sprintf("%06d.part", i))
		partFile, err := os.Open(chunkPath)
		if err != nil {
			return fmt.Errorf("打开分片 %d 失败: %w", i, err)
		}
		if _, err := io.Copy(target, partFile); err != nil {
			partFile.Close()
			return fmt.Errorf("合并分片 %d 失败: %w", i, err)
		}
		partFile.Close()
	}

	// 确保数据落盘
	return target.Sync()
}

func (m *ChunkedUploadManager) completeToHost(session *ChunkedUploadSession, mergedPath string) error {
	// 确保目标目录存在
	if err := os.MkdirAll(session.Path, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	finalPath := filepath.Join(session.Path, session.Filename)

	// 尝试原子 rename（同一文件系统时高效）
	if err := os.Rename(mergedPath, finalPath); err != nil {
		// 跨文件系统时 fallback 到 copy
		return copyFile(mergedPath, finalPath)
	}
	return nil
}

func (m *ChunkedUploadManager) completeToContainer(session *ChunkedUploadSession, mergedPath string) error {
	// 读取合并后的文件内容
	content, err := os.ReadFile(mergedPath)
	if err != nil {
		return fmt.Errorf("读取合并文件失败: %w", err)
	}

	// 使用现有的 ContainerFileManager 写入容器
	cfm, err := NewContainerFileManager(m.log, session.ContainerID)
	if err != nil {
		return fmt.Errorf("创建容器文件管理器失败: %w", err)
	}
	defer cfm.Close()

	return cfm.WriteFileFromBytes(session.Path, session.Filename, content)
}

func (m *ChunkedUploadManager) cleanupExpired() {
	now := time.Now()
	var expired []string

	m.mu.Lock()
	for id, s := range m.sessions {
		if now.Sub(s.CreatedAt) > chunkedUploadTTL {
			expired = append(expired, id)
			_ = os.RemoveAll(s.TempDir)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	if len(expired) > 0 {
		m.log.Info("清理过期分片上传会话: %d 个", len(expired))
	}
}

func verifyFileHash(path, expectedHash string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	return actual == expectedHash, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}
