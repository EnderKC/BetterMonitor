package services

import (
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"sync"
)

// UploadTarget 上传目标类型
type UploadTarget int

const (
	// TargetHost 主机文件系统
	TargetHost UploadTarget = iota
	// TargetContainer Docker 容器
	TargetContainer
)

// 上传大小限制
const (
	MaxHostUploadSize      int64 = 50 * 1024 * 1024  // 50MB
	MaxContainerUploadSize int64 = 100 * 1024 * 1024 // 100MB
)

// UploadRequest 统一上传请求
type UploadRequest struct {
	Target      UploadTarget
	ServerID    uint
	ContainerID string // 仅容器上传时有值
	Path        string // 目标目录路径
	Filename    string // 文件名（sanitize 后填充）
	Content     []byte // 文件内容
}

// AgentSender 定义向 Agent 发送上传请求的函数签名，由 controllers 注入
type AgentSender struct {
	// SendHostFile 向主机文件系统上传
	SendHostFile func(serverID uint, targetPath string, content []byte) error
	// SendContainerFile 向容器上传
	SendContainerFile func(serverID uint, containerID, targetPath string, content []byte) error
	// ValidateFilePath 校验文件路径
	ValidateFilePath func(path string) bool
}

// UploadService 统一文件上传服务
type UploadService struct {
	sender AgentSender
}

var (
	uploadServiceInstance *UploadService
	uploadServiceOnce     sync.Once
)

// InitUploadService 初始化上传服务（由 controllers 在启动时调用）
func InitUploadService(sender AgentSender) {
	uploadServiceOnce.Do(func() {
		uploadServiceInstance = &UploadService{
			sender: sender,
		}
	})
}

// GetUploadService 获取上传服务单例
func GetUploadService() *UploadService {
	if uploadServiceInstance == nil {
		panic("UploadService not initialized, call InitUploadService first")
	}
	return uploadServiceInstance
}

// Upload 统一上传入口
func (s *UploadService) Upload(req *UploadRequest) error {
	if err := s.validateRequest(req); err != nil {
		return err
	}
	return s.sendToAgent(req)
}

// UploadFromMultipart 从 multipart 文件构建请求并上传
func (s *UploadService) UploadFromMultipart(target UploadTarget, serverID uint, containerID, path string, file multipart.File, header *multipart.FileHeader) error {
	// 确定大小限制
	maxSize := MaxHostUploadSize
	if target == TargetContainer {
		maxSize = MaxContainerUploadSize
	}

	// 校验文件大小
	if header.Size <= 0 {
		return fmt.Errorf("文件内容为空")
	}
	if header.Size > maxSize {
		return fmt.Errorf("文件太大，最大允许%dMB", maxSize/1024/1024)
	}

	// 读取文件内容（使用 LimitReader 做额外保护）
	limitedReader := io.LimitReader(file, maxSize+1)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("读取上传文件失败: %w", err)
	}
	if int64(len(content)) > maxSize {
		return fmt.Errorf("文件太大，最大允许%dMB", maxSize/1024/1024)
	}

	// 清洁文件名
	filename, err := s.SanitizeFilename(header.Filename)
	if err != nil {
		return err
	}

	// 构造目标路径
	targetPath := filepath.Join(path, filename)
	targetPath = filepath.Clean(targetPath)

	req := &UploadRequest{
		Target:      target,
		ServerID:    serverID,
		ContainerID: containerID,
		Path:        targetPath,
		Content:     content,
	}

	return s.Upload(req)
}

// validateRequest 校验上传请求
func (s *UploadService) validateRequest(req *UploadRequest) error {
	if req == nil {
		return fmt.Errorf("上传请求为空")
	}
	if req.ServerID == 0 {
		return fmt.Errorf("无效的服务器ID")
	}
	if len(req.Content) == 0 {
		return fmt.Errorf("文件内容为空")
	}
	if req.Path == "" {
		return fmt.Errorf("目标路径不能为空")
	}

	// 路径安全校验
	if s.sender.ValidateFilePath != nil && !s.sender.ValidateFilePath(req.Path) {
		return fmt.Errorf("无效的文件路径")
	}

	if req.Target == TargetContainer && strings.TrimSpace(req.ContainerID) == "" {
		return fmt.Errorf("容器ID不能为空")
	}

	return nil
}

// SanitizeFilename 清洁文件名，去除路径穿越和特殊字符
func (s *UploadService) SanitizeFilename(name string) (string, error) {
	// 统一反斜杠
	name = strings.ReplaceAll(name, "\\", "/")
	// 去除空白
	name = strings.TrimSpace(name)
	// 取基础文件名（去除目录部分）
	name = filepath.Base(name)

	if name == "" || name == "." || name == "/" || name == ".." {
		return "", fmt.Errorf("无效的文件名")
	}

	return name, nil
}

// sendToAgent 向 Agent 发送上传请求
func (s *UploadService) sendToAgent(req *UploadRequest) error {
	switch req.Target {
	case TargetHost:
		if s.sender.SendHostFile == nil {
			return fmt.Errorf("主机文件上传功能未注册")
		}
		return s.sender.SendHostFile(req.ServerID, req.Path, req.Content)

	case TargetContainer:
		if s.sender.SendContainerFile == nil {
			return fmt.Errorf("容器文件上传功能未注册")
		}
		return s.sender.SendContainerFile(req.ServerID, req.ContainerID, req.Path, req.Content)

	default:
		return fmt.Errorf("不支持的上传目标类型: %d", req.Target)
	}
}
