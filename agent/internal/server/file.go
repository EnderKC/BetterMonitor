package server

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/user/server-ops-agent/pkg/logger"
)

// FileInfo 文件信息
type FileInfo struct {
	Name     string      `json:"name"`               // 文件名
	Size     int64       `json:"size"`               // 文件大小
	ModTime  string      `json:"mod_time"`           // 修改时间
	IsDir    bool        `json:"is_dir"`             // 是否是目录
	Mode     string      `json:"mode"`               // 文件权限
	Children []*FileInfo `json:"children,omitempty"` // 子文件（目录树使用）
}

// FileManager 文件管理器
type FileManager struct {
	log *logger.Logger
}

// NewFileManager 创建新的文件管理器
func NewFileManager(log *logger.Logger) *FileManager {
	return &FileManager{
		log: log,
	}
}

// ListFiles 列出指定目录下的文件
func (fm *FileManager) ListFiles(path string) ([]*FileInfo, error) {
	fm.log.Debug("获取目录列表: %s", path)

	// 处理空路径和根路径
	if path == "" {
		path = "/"
	}

	// 打开目录
	dir, err := os.Open(path)
	if err != nil {
		fm.log.Error("打开目录失败: %v", err)
		return nil, fmt.Errorf("打开目录失败: %v", err)
	}
	defer dir.Close()

	// 读取目录内容
	entries, err := dir.Readdir(-1)
	if err != nil {
		fm.log.Error("读取目录内容失败: %v", err)
		return nil, fmt.Errorf("读取目录内容失败: %v", err)
	}

	// 转换为FileInfo结构
	files := make([]*FileInfo, 0, len(entries))
	for _, entry := range entries {
		files = append(files, &FileInfo{
			Name:    entry.Name(),
			Size:    entry.Size(),
			ModTime: entry.ModTime().Format(time.RFC3339),
			IsDir:   entry.IsDir(),
			Mode:    entry.Mode().String(),
		})
	}

	// 排序：目录在前，文件在后，然后按名称排序
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

// GetFileContent 获取文件内容
func (fm *FileManager) GetFileContent(path string) (string, error) {
	fm.log.Debug("获取文件内容: %s", path)

	// 检查文件大小
	fileInfo, err := os.Stat(path)
	if err != nil {
		fm.log.Error("检查文件大小失败: %v", err)
		return "", fmt.Errorf("检查文件大小失败: %v", err)
	}

	// 限制文件大小
	if fileInfo.Size() > 10*1024*1024 { // 10MB
		fm.log.Error("文件过大: %d bytes", fileInfo.Size())
		return "", fmt.Errorf("文件过大，不能读取超过10MB的文本文件")
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		fm.log.Error("读取文件内容失败: %v", err)
		return "", fmt.Errorf("读取文件内容失败: %v", err)
	}

	return string(content), nil
}

// SaveFileContent 保存文件内容
func (fm *FileManager) SaveFileContent(path, content string) error {
	fm.log.Debug("保存文件内容: %s", path)

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fm.log.Error("创建目录失败: %v", err)
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 添加更健壮的错误处理和恢复机制
	var tempPath string
	var originalExists bool
	var originalContent []byte
	var originalMode os.FileMode = 0644

	// 检查原始文件是否存在
	fileInfo, err := os.Stat(path)
	if err == nil {
		// 文件存在，获取原始权限和内容
		originalExists = true
		originalMode = fileInfo.Mode()
		
		// 读取原始内容用于恢复
		originalContent, err = os.ReadFile(path)
		if err != nil {
			fm.log.Warn("读取原始文件内容失败，无法创建备份: %v", err)
			// 继续操作，但无法恢复
		}
	}

	// 创建临时文件，使用随机后缀防止冲突
	tempPath = path + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	
	// 首先写入临时文件
	if err := os.WriteFile(tempPath, []byte(content), originalMode); err != nil {
		fm.log.Error("写入临时文件失败: %v", err)
		// 清理临时文件
		os.Remove(tempPath)
		return fmt.Errorf("写入临时文件失败: %v", err)
	}

	// 确保临时文件被写入磁盘
	if err := syncFile(tempPath); err != nil {
		fm.log.Error("同步临时文件到磁盘失败: %v", err)
		// 清理临时文件
		os.Remove(tempPath)
		return fmt.Errorf("同步临时文件到磁盘失败: %v", err)
	}

	// 重命名临时文件为目标文件
	if err := os.Rename(tempPath, path); err != nil {
		fm.log.Error("重命名文件失败: %v", err)
		
		// 清理临时文件
		os.Remove(tempPath)
		
		// 如果原始文件存在，尝试恢复
		if originalExists && len(originalContent) > 0 {
			fm.log.Info("尝试恢复原始文件内容...")
			if err := os.WriteFile(path, originalContent, originalMode); err != nil {
				fm.log.Error("恢复原始文件失败: %v", err)
				return fmt.Errorf("重命名文件失败且无法恢复原始文件: %v", err)
			}
			fm.log.Info("成功恢复原始文件内容")
		}
		
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	fm.log.Debug("文件保存成功: %s", path)
	return nil
}

// syncFile 确保文件被写入磁盘
func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	
	// 同步文件到磁盘
	return file.Sync()
}

// CreateFile 创建文件
func (fm *FileManager) CreateFile(path, content string) error {
	fm.log.Debug("创建文件: %s", path)

	// 检查文件是否已存在
	if _, err := os.Stat(path); err == nil {
		fm.log.Error("文件已存在: %s", path)
		return fmt.Errorf("文件已存在: %s", path)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fm.log.Error("创建目录失败: %v", err)
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 写入文件内容
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fm.log.Error("写入文件内容失败: %v", err)
		return fmt.Errorf("写入文件内容失败: %v", err)
	}

	return nil
}

// CreateDirectory 创建目录
func (fm *FileManager) CreateDirectory(path string) error {
	fm.log.Debug("创建目录: %s", path)

	// 检查目录是否已存在
	if _, err := os.Stat(path); err == nil {
		fm.log.Error("目录已存在: %s", path)
		return fmt.Errorf("目录已存在: %s", path)
	}

	// 创建目录
	if err := os.MkdirAll(path, 0755); err != nil {
		fm.log.Error("创建目录失败: %v", err)
		return fmt.Errorf("创建目录失败: %v", err)
	}

	return nil
}

// UploadFile 上传文件
func (fm *FileManager) UploadFile(path, filename, content string) error {
	fm.log.Debug("上传文件: %s/%s", path, filename)

	// 确保目录存在
	if err := os.MkdirAll(path, 0755); err != nil {
		fm.log.Error("创建目录失败: %v", err)
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 构造完整的文件路径
	fullPath := filepath.Join(path, filename)

	// 解码Base64内容
	fileContent, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		fm.log.Error("解码文件内容失败: %v", err)
		return fmt.Errorf("解码文件内容失败: %v", err)
	}

	// 创建临时文件
	tempPath := fullPath + ".tmp"
	if err := os.WriteFile(tempPath, fileContent, 0644); err != nil {
		fm.log.Error("写入临时文件失败: %v", err)
		return fmt.Errorf("写入临时文件失败: %v", err)
	}

	// 重命名临时文件为目标文件
	if err := os.Rename(tempPath, fullPath); err != nil {
		os.Remove(tempPath) // 清理临时文件
		fm.log.Error("重命名文件失败: %v", err)
		return fmt.Errorf("重命名文件失败: %v", err)
	}

	return nil
}

// DownloadFile 获取文件内容用于下载
func (fm *FileManager) DownloadFile(path string) ([]byte, error) {
	fm.log.Debug("下载文件: %s", path)

	// 检查文件大小
	fileInfo, err := os.Stat(path)
	if err != nil {
		fm.log.Error("检查文件失败: %v", err)
		return nil, fmt.Errorf("检查文件失败: %v", err)
	}

	// 检查是否是目录
	if fileInfo.IsDir() {
		fm.log.Error("不能下载目录: %s", path)
		return nil, fmt.Errorf("不能下载目录")
	}

	// 限制文件大小
	if fileInfo.Size() > 100*1024*1024 { // 100MB
		fm.log.Error("文件过大: %d bytes", fileInfo.Size())
		return nil, fmt.Errorf("文件过大，不能下载超过100MB的文件")
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		fm.log.Error("读取文件内容失败: %v", err)
		return nil, fmt.Errorf("读取文件内容失败: %v", err)
	}

	return content, nil
}

// DeleteFiles 删除文件或目录
func (fm *FileManager) DeleteFiles(paths []string) error {
	for _, path := range paths {
		fm.log.Debug("删除文件或目录: %s", path)

		// 检查文件是否存在
		fileInfo, err := os.Stat(path)
		if err != nil {
			fm.log.Error("检查文件失败: %v", err)
			return fmt.Errorf("检查文件失败: %v", err)
		}

		// 根据类型删除文件或目录
		if fileInfo.IsDir() {
			// 删除目录及其内容
			if err := os.RemoveAll(path); err != nil {
				fm.log.Error("删除目录失败: %v", err)
				return fmt.Errorf("删除目录失败: %v", err)
			}
		} else {
			// 删除文件
			if err := os.Remove(path); err != nil {
				fm.log.Error("删除文件失败: %v", err)
				return fmt.Errorf("删除文件失败: %v", err)
			}
		}
	}

	return nil
}

// GetDirectoryTree 获取目录树
func (fm *FileManager) GetDirectoryTree(path string, depth int) ([]*FileInfo, error) {
	fm.log.Debug("获取目录树: %s (深度: %d)", path, depth)

	// 处理根路径
	if path == "" {
		path = "/"
	}

	// 检查路径是否存在
	fileInfo, err := os.Stat(path)
	if err != nil {
		fm.log.Error("检查路径失败: %v", err)
		return nil, fmt.Errorf("检查路径失败: %v", err)
	}

	// 检查是否是目录
	if !fileInfo.IsDir() {
		fm.log.Error("路径不是目录: %s", path)
		return nil, fmt.Errorf("路径不是目录: %s", path)
	}

	// 递归构建目录树
	result, err := fm.buildDirectoryTree(path, "", depth)
	if err != nil {
		fm.log.Error("构建目录树失败: %v", err)
		return nil, fmt.Errorf("构建目录树失败: %v", err)
	}

	return result, nil
}

// 递归构建目录树
func (fm *FileManager) buildDirectoryTree(rootPath, currentPath string, depth int) ([]*FileInfo, error) {
	// 如果深度为0，则不再递归
	if depth <= 0 {
		return []*FileInfo{}, nil
	}

	// 构建完整路径
	fullPath := rootPath
	if currentPath != "" {
		fullPath = filepath.Join(rootPath, currentPath)
	}

	// 打开目录
	dir, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	// 读取目录内容
	entries, err := dir.Readdir(-1)
	if err != nil {
		return nil, err
	}

	// 创建结果数组
	result := make([]*FileInfo, 0, len(entries))

	// 添加目录项
	for _, entry := range entries {
		// 排除隐藏文件
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info := &FileInfo{
			Name:    entry.Name(),
			Size:    entry.Size(),
			ModTime: entry.ModTime().Format(time.RFC3339),
			IsDir:   entry.IsDir(),
			Mode:    entry.Mode().String(),
		}

		// 如果是目录且深度大于1，则递归获取子目录
		if entry.IsDir() && depth > 1 {
			entryPath := filepath.Join(currentPath, entry.Name())
			children, err := fm.buildDirectoryTree(rootPath, entryPath, depth-1)
			if err != nil {
				continue // 忽略无法访问的子目录
			}
			info.Children = children
		}

		result = append(result, info)
	}

	// 排序：目录在前，文件在后，然后按名称排序
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}
