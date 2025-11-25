package server

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/user/server-ops-agent/internal/monitor"
	"github.com/user/server-ops-agent/pkg/logger"
)

// ContainerFileManager 负责容器内部的文件操作。
// 它混合使用 Docker SDK API（CopyFrom/CopyTo/Stat）和容器内命令执行（Exec）：
// - 文件读写使用 SDK 的 tar 流传输，保证二进制安全
// - 目录列举、创建、删除使用容器内命令，更高效直观
// - 列目录优先使用 stat 命令，失败时降级到 tar 方案以兼容精简镜像
type ContainerFileManager struct {
	log         *logger.Logger
	docker      *monitor.DockerManager
	containerID string
}

// NewContainerFileManager 创建容器文件管理器
func NewContainerFileManager(log *logger.Logger, containerID string) (*ContainerFileManager, error) {
	if containerID == "" {
		return nil, fmt.Errorf("容器ID不能为空")
	}

	manager, err := monitor.NewDockerManager(log)
	if err != nil {
		return nil, fmt.Errorf("创建Docker管理器失败: %w", err)
	}

	return &ContainerFileManager{
		log:         log,
		docker:      manager,
		containerID: containerID,
	}, nil
}

// Close 释放资源
func (cfm *ContainerFileManager) Close() {
	if cfm.docker != nil {
		_ = cfm.docker.Close()
	}
}

// ListFiles 列出容器内目录文件。
// 首选方案是调用容器里的 stat 命令，因为可以一次性拿到权限/时间等信息；
// 如果容器镜像过于精简（busybox 等）导致命令不可用，则回退到 tar 方案。
func (cfm *ContainerFileManager) ListFiles(path string) ([]*FileInfo, error) {
	if path == "" {
		path = "/"
	}

	output, err := cfm.runListCommand(path)
	if err != nil {
		cfm.log.Warn("容器stat列目录失败，尝试tar列目录: %v", err)
		return cfm.listFilesViaTar(path)
	}

	files, err := cfm.parseStatOutput(output)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		cfm.log.Debug("stat输出为空，尝试tar列目录: %s", path)
		return cfm.listFilesViaTar(path)
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

// GetFileContent 读取文件内容
func (cfm *ContainerFileManager) GetFileContent(path string) (string, error) {
	data, err := cfm.readFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SaveFileContent 保存文件内容。
// 如果目标文件已存在，会尝试复用原文件的权限位，避免破坏容器内既有权限设定。
func (cfm *ContainerFileManager) SaveFileContent(path, content string) error {
	data := []byte(content)
	mode := os.FileMode(0644)

	if stat, err := cfm.statPath(path); err == nil {
		mode = stat.Mode
	}

	return cfm.writeFile(path, data, mode)
}

// CreateFile 创建新文件（若已存在则报错）。
// 由于容器端不存在“触摸文件”接口，所以直接复用 writeFile 逻辑。
func (cfm *ContainerFileManager) CreateFile(path, content string) error {
	if path == "" {
		return fmt.Errorf("文件路径不能为空")
	}

	if _, err := cfm.statPath(path); err == nil {
		return fmt.Errorf("文件已存在: %s", path)
	} else if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("检查文件失败: %w", err)
	}

	data := []byte(content)
	return cfm.writeFile(path, data, 0644)
}

// CreateDirectory 创建目录，通过在容器内执行 mkdir -p 完成。
func (cfm *ContainerFileManager) CreateDirectory(path string) error {
	if path == "" || path == "/" {
		return fmt.Errorf("目录路径无效")
	}

	script := fmt.Sprintf("set -e\nmkdir -p %s", shellEscape(path))
	_, err := cfm.runShell(script)
	if err != nil {
		return fmt.Errorf("创建容器目录失败: %w", err)
	}
	return nil
}

// UploadFile 上传文件，前端的内容以 Base64 方式发送，解析后交给 writeFile。
func (cfm *ContainerFileManager) UploadFile(path, filename, content string) error {
	if filename == "" {
		return fmt.Errorf("文件名不能为空")
	}

	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return fmt.Errorf("解码文件内容失败: %w", err)
	}

	targetPath := filepath.Join(path, filename)
	return cfm.writeFile(targetPath, data, 0644)
}

// DownloadFile 下载文件
func (cfm *ContainerFileManager) DownloadFile(path string) ([]byte, error) {
	return cfm.readFile(path)
}

// DeleteFiles 删除文件或目录，直接在容器内执行 rm -rf。
// 这里不做更多的安全检查（除了禁止删除根），以保持与宿主机文件管理器一致。
func (cfm *ContainerFileManager) DeleteFiles(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	var parts []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || p == "/" {
			return fmt.Errorf("不允许删除根目录或空路径")
		}
		parts = append(parts, shellEscape(p))
	}

	script := fmt.Sprintf("set -e\nrm -rf %s", strings.Join(parts, " "))
	_, err := cfm.runShell(script)
	if err != nil {
		return fmt.Errorf("删除容器文件失败: %w", err)
	}
	return nil
}

// GetDirectoryTree 获取目录树，用 ListFiles 做一层层递归。
// 由于容器端目录结构可能很深，因此在递归过程中需要捕获错误并记录日志，但不中断其它节点。
// GetDirectoryTree 获取目录树
// 优化：尝试使用 find 命令一次性获取所有文件信息，避免递归调用 docker exec
func (cfm *ContainerFileManager) GetDirectoryTree(path string, depth int) ([]*FileInfo, error) {
	if depth <= 0 {
		return []*FileInfo{}, nil
	}

	// 尝试使用 find 命令批量获取
	// 只有当深度大于1时才使用 find，因为对于单层目录（depth=1），ListFiles (ls) 更可靠且已在文件列表中验证过
	if depth > 1 {
		output, err := cfm.runRecursiveListCommand(path, depth)
		if err == nil {
			// 解析输出
			files, err := cfm.parseRecursiveStatOutput(path, output)
			if err == nil && len(files) > 0 {
				return cfm.buildTree(files, path), nil
			}
			if err != nil {
				cfm.log.Warn("解析递归stat输出失败: %v，回退到递归方案", err)
			} else if len(files) == 0 {
				cfm.log.Warn("递归列目录返回为空，可能是 find/stat 命令兼容性问题，回退到递归方案")
			}
		} else {
			cfm.log.Warn("容器递归列目录失败: %v，回退到递归方案", err)
		}
	}

	// 回退到原来的递归方案
	entries, err := cfm.ListFiles(path)
	if err != nil {
		return nil, err
	}

	result := make([]*FileInfo, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, ".") {
			continue
		}

		item := &FileInfo{
			Name:    entry.Name,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			IsDir:   entry.IsDir,
			Mode:    entry.Mode,
		}

		if entry.IsDir && depth > 1 {
			childPath := joinContainerPath(path, entry.Name)
			children, childErr := cfm.GetDirectoryTree(childPath, depth-1)
			if childErr != nil {
				cfm.log.Warn("读取容器目录失败: %v", childErr)
			} else {
				item.Children = children
			}
		}

		result = append(result, item)
	}

	return result, nil
}

// buildTree 将扁平的文件列表构建为树形结构
func (cfm *ContainerFileManager) buildTree(files []*FileInfo, rootPath string) []*FileInfo {
	// 建立映射 map[path]*FileInfo
	// pathKey 是相对于 rootPath 的路径，例如 "foo/bar"
	fileMap := make(map[string]*FileInfo)

	// 根目录下的直接子节点
	var rootChildren []*FileInfo

	// 规范化 rootPath
	rootPath = strings.TrimRight(rootPath, "/")
	if rootPath == "" {
		rootPath = "/"
	}

	// 第一次遍历：创建所有节点并存入 map
	for _, f := range files {
		// f.Name 已经是相对于 rootPath 的路径（例如 "dir1/file1"）
		// 或者如果是根目录本身，则是 "."
		if f.Name == "." || f.Name == "" {
			continue
		}
		fileMap[f.Name] = f
	}

	// 第二次遍历：构建树形关系
	for _, f := range files {
		if f.Name == "." || f.Name == "" {
			continue
		}

		// 获取父目录路径
		dir := filepath.Dir(f.Name)

		// 如果父目录是 "." 或 "/" (取决于 filepath.Dir 的行为)，说明是根节点的直接子节点
		if dir == "." || dir == "/" || dir == "" {
			rootChildren = append(rootChildren, f)
			continue
		}

		// 尝试找到父节点
		if parent, ok := fileMap[dir]; ok {
			if parent.Children == nil {
				parent.Children = make([]*FileInfo, 0)
			}
			parent.Children = append(parent.Children, f)
		} else {
			// 如果找不到父节点（可能是 find depth 限制导致中间节点缺失，或者逻辑错误）
			// 这种情况下将其作为根节点的子节点，或者忽略
			// 这里选择作为根节点的子节点，防止丢失
			rootChildren = append(rootChildren, f)
		}
	}

	// 修正 Name 字段：前端期望 Name 是文件名，而不是相对路径
	// 我们在构建完树之后再修改 Name，或者在添加到 Children 之前修改
	// 但是 map key 依赖于路径。所以我们在最后统一修改。

	// 递归修改 Name
	var fixNames func(nodes []*FileInfo)
	fixNames = func(nodes []*FileInfo) {
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].IsDir != nodes[j].IsDir {
				return nodes[i].IsDir
			}
			return strings.ToLower(filepath.Base(nodes[i].Name)) < strings.ToLower(filepath.Base(nodes[j].Name))
		})

		for _, node := range nodes {
			node.Name = filepath.Base(node.Name)
			if len(node.Children) > 0 {
				fixNames(node.Children)
			}
		}
	}

	fixNames(rootChildren)
	return rootChildren
}

// runListCommand 列出容器目录。
func (cfm *ContainerFileManager) runListCommand(path string) (string, error) {
	script := fmt.Sprintf(`set -e
TARGET=%s
if ! command -v stat >/dev/null 2>&1; then
  echo "stat command not found" >&2
  exit 91
fi
if [ ! -e "$TARGET" ]; then
  echo "DEBUG: TARGET=$TARGET does not exist" >&2
  ls -la "$TARGET" >&2 || true
  exit 90
fi

output_entry() {
  item="$1"
  if result=$(stat -c 'ENTRY|%%n|%%f|%%s|%%Y|%%F' -- "$item" 2>/dev/null); then
    printf "%%s\n" "$result"
  fi
}

if [ -d "$TARGET" ]; then
  cd "$TARGET" || exit 92
  IFS=$(printf '\n')
  for entry in $(ls -A1); do
    [ -z "$entry" ] && continue
    output_entry "./$entry"
  done
else
  output_entry "$TARGET"
fi
`, shellEscape(path))

	cfm.log.Debug("执行脚本:\n%s", script)
	stdout, stderr, err := cfm.docker.RunCommand(cfm.containerID, []string{"/bin/sh", "-c", script}, []string{"LC_ALL=C"})
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf(strings.TrimSpace(stderr))
		}
		return "", fmt.Errorf("执行容器命令失败: %w", err)
	}

	return stdout, nil
}

// runRecursiveListCommand 递归列出容器目录
func (cfm *ContainerFileManager) runRecursiveListCommand(path string, depth int) (string, error) {
	script := fmt.Sprintf(`set -e
TARGET=%s
if ! command -v find >/dev/null 2>&1; then
  echo "find command not found" >&2
  exit 91
fi
if ! command -v stat >/dev/null 2>&1; then
  echo "stat command not found" >&2
  exit 91
fi

if [ -d "$TARGET" ]; then
  cd "$TARGET" || exit 92
  find . -maxdepth %d -name "." -o -exec stat -c 'ENTRY|%%n|%%f|%%s|%%Y|%%F' -- {} + 2>/dev/null || \
  find . -maxdepth %d -name "." -o -exec stat -c 'ENTRY|%%n|%%f|%%s|%%Y|%%F' -- {} \; 2>/dev/null
else
  stat -c 'ENTRY|%%n|%%f|%%s|%%Y|%%F' -- "$TARGET"
fi
`, shellEscape(path), depth, depth)

	stdout, stderr, err := cfm.docker.RunCommand(cfm.containerID, []string{"/bin/sh", "-c", script}, []string{"LC_ALL=C"})
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf(strings.TrimSpace(stderr))
		}
		return "", fmt.Errorf("执行容器命令失败: %w", err)
	}

	return stdout, nil
}

// parseRecursiveStatOutput 解析递归stat输出
func (cfm *ContainerFileManager) parseRecursiveStatOutput(rootPath, data string) ([]*FileInfo, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return []*FileInfo{}, nil
	}

	lines := strings.Split(data, "\n")
	result := make([]*FileInfo, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "ENTRY|") {
			continue
		}
		parts := strings.SplitN(line[6:], "|", 5)
		if len(parts) < 5 {
			continue
		}

		// parts[0] 是路径，例如 "./dir/file"
		rawName := parts[0]
		// 去掉 "./" 前缀
		name := strings.TrimPrefix(rawName, "./")
		// 如果是 "." 本身，保留或忽略？通常忽略根目录本身，因为 buildTree 会处理
		if name == "." || name == "" {
			continue
		}

		modeHex := parts[1]
		sizeVal, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			sizeVal = 0
		}

		mtimeVal, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			mtimeVal = time.Now().Unix()
		}

		modeUint, err := strconv.ParseUint(modeHex, 16, 32)
		if err != nil {
			modeUint = 0
		}
		fileMode := os.FileMode(modeUint)

		info := &FileInfo{
			Name:    name, // 这里保留相对路径，例如 "dir/file"
			Size:    sizeVal,
			ModTime: time.Unix(mtimeVal, 0).Format(time.RFC3339),
			IsDir:   fileMode.IsDir(),
			Mode:    fileMode.String(),
		}

		result = append(result, info)
	}

	return result, nil
}

// parseStatOutput 将上述 shell 输出转换为 FileInfo 列表，方便直接给到前端。
func (cfm *ContainerFileManager) parseStatOutput(data string) ([]*FileInfo, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return []*FileInfo{}, nil
	}

	lines := strings.Split(data, "\n")
	result := make([]*FileInfo, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "ENTRY|") {
			continue
		}
		parts := strings.SplitN(line[6:], "|", 5)
		if len(parts) < 5 {
			continue
		}

		name := cfm.normalizeName(parts[0])
		if name == "" {
			continue
		}

		modeHex := parts[1]
		sizeVal, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			sizeVal = 0
		}

		mtimeVal, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			mtimeVal = time.Now().Unix()
		}

		modeUint, err := strconv.ParseUint(modeHex, 16, 32)
		if err != nil {
			modeUint = 0
		}
		fileMode := os.FileMode(modeUint)

		info := &FileInfo{
			Name:    name,
			Size:    sizeVal,
			ModTime: time.Unix(mtimeVal, 0).Format(time.RFC3339),
			IsDir:   fileMode.IsDir(),
			Mode:    fileMode.String(),
		}

		result = append(result, info)
	}

	return result, nil
}

// listFilesViaTar 使用 CopyFromContainer 构建目录列表。
// 这是在容器内 stat 命令缺失时的兜底方案，通过 docker cp 打包目录，
// 遍历 tar 首层条目即可得知当前目录下的子文件。
func (cfm *ContainerFileManager) listFilesViaTar(path string) ([]*FileInfo, error) {
	reader, stat, err := cfm.docker.CopyFromContainer(cfm.containerID, path)
	if err != nil {
		return nil, fmt.Errorf("容器目录读取失败: %w", err)
	}
	defer reader.Close()

	// 处理单文件场景
	if !stat.Mode.IsDir() {
		return []*FileInfo{{
			Name:    filepath.Base(path),
			Size:    stat.Size,
			ModTime: stat.Mtime.Format(time.RFC3339),
			IsDir:   false,
			Mode:    stat.Mode.String(),
		}}, nil
	}

	tr := tar.NewReader(reader)
	entries := make(map[string]*FileInfo)
	cleanPath := path
	if cleanPath == "" {
		cleanPath = "/"
	}
	cleanPath = filepath.Clean(cleanPath)
	rootPrefix := ""
	if cleanPath != "/" {
		rootPrefix = strings.Trim(filepath.Base(cleanPath), "/")
		if rootPrefix == "." {
			rootPrefix = ""
		}
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析容器目录tar失败: %w", err)
		}

		rawName := header.Name
		rawName = strings.TrimPrefix(rawName, "./")
		rawName = strings.TrimPrefix(rawName, "/")
		rawName = strings.TrimSuffix(rawName, "/")

		if rootPrefix != "" {
			if rawName == rootPrefix {
				continue
			}
			if strings.HasPrefix(rawName, rootPrefix+"/") {
				rawName = rawName[len(rootPrefix)+1:]
			}
		}

		rawName = strings.Trim(rawName, "/")
		if rawName == "" {
			continue
		}

		parts := strings.Split(rawName, "/")
		entryName := parts[0]
		if entryName == "" || entryName == "." {
			continue
		}
		if _, exists := entries[entryName]; exists {
			continue
		}

		info := header.FileInfo()
		isDir := info.IsDir()
		if len(parts) > 1 {
			isDir = true
		}
		size := info.Size()
		if isDir {
			size = 0
		}

		entries[entryName] = &FileInfo{
			Name:    entryName,
			Size:    size,
			ModTime: header.ModTime.Format(time.RFC3339),
			IsDir:   isDir,
			Mode:    info.Mode().String(),
		}
	}

	result := make([]*FileInfo, 0, len(entries))
	for _, item := range entries {
		result = append(result, item)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

// readFile 读取容器文件，先 stat 以确认不是目录并限制文件大小，再通过 tar 流取出内容。
func (cfm *ContainerFileManager) readFile(path string) ([]byte, error) {
	stat, err := cfm.statPath(path)
	if err != nil {
		return nil, fmt.Errorf("读取容器文件失败: %w", err)
	}
	if stat.Mode.IsDir() {
		return nil, fmt.Errorf("目标是目录，无法读取内容")
	}
	// 限制超大文件，避免 tar 生成耗时与内存占用
	const maxReadableSize = 100 * 1024 * 1024 // 100MB
	if stat.Size > maxReadableSize {
		return nil, fmt.Errorf("文件过大（>%dMB），请通过下载方式获取", maxReadableSize/1024/1024)
	}

	reader, _, err := cfm.docker.CopyFromContainer(cfm.containerID, path)
	if err != nil {
		return nil, fmt.Errorf("读取容器文件失败: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析容器文件失败: %w", err)
		}
		if header.FileInfo().IsDir() {
			continue
		}

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil {
			return nil, fmt.Errorf("读取容器文件内容失败: %w", err)
		}
		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("未找到文件: %s", path)
}

// writeFile 写入容器文件。
// Docker CopyToContainer 只接受 tar 流，这里会把单个文件打包成临时 tar，并允许覆盖已有文件。
func (cfm *ContainerFileManager) writeFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		dir = "/"
	}

	if err := cfm.ensureDirectory(dir); err != nil {
		return err
	}

	header := &tar.Header{
		Name:    filepath.Base(path),
		Mode:    int64(mode.Perm()),
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("写入文件头失败: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("写入文件内容失败: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("关闭tar写入器失败: %w", err)
	}

	reader := bytes.NewReader(buf.Bytes())
	if err := cfm.docker.CopyToContainer(cfm.containerID, dir, reader, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	}); err != nil {
		return fmt.Errorf("写入容器文件失败: %w", err)
	}

	return nil
}

// ensureDirectory 确保目录存在，用 mkdir -p 实现。
func (cfm *ContainerFileManager) ensureDirectory(path string) error {
	if path == "" {
		path = "/"
	}
	script := fmt.Sprintf("mkdir -p %s", shellEscape(path))
	_, err := cfm.runShell(script)
	if err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	return nil
}

// statPath 获取路径信息，直接调用 Docker API 返回 PathStat（包含 Mode/Size 等）。
func (cfm *ContainerFileManager) statPath(path string) (*container.PathStat, error) {
	if cfm.docker == nil {
		return nil, fmt.Errorf("Docker管理器未初始化")
	}
	return cfm.docker.StatContainerPath(cfm.containerID, path)
}

// runShell 在容器内执行 shell 脚本，统一封装错误输出，方便复用。
func (cfm *ContainerFileManager) runShell(script string) (string, error) {
	stdout, stderr, err := cfm.docker.RunCommand(cfm.containerID, []string{"/bin/sh", "-c", script}, []string{"LC_ALL=C"})
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return "", fmt.Errorf(strings.TrimSpace(stderr))
		}
		return "", err
	}
	return stdout, nil
}

// normalizeName 规范化 stat 输出的名称，移除 ./ 或 / 前缀，最终只保留文件名。
func (cfm *ContainerFileManager) normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimPrefix(name, "/")
	return filepath.Base(name)
}

// joinContainerPath 组合容器路径
func joinContainerPath(base, name string) string {
	if base == "" || base == "/" {
		return "/" + strings.TrimPrefix(name, "/")
	}
	base = strings.TrimRight(base, "/")
	return base + "/" + strings.TrimPrefix(name, "/")
}

// shellEscape 转义Shell字符串
func shellEscape(path string) string {
	if path == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
