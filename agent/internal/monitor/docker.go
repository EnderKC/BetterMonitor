//go:build !monitor_only

package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/user/server-ops-agent/pkg/logger"
)

// Compose 配置相关错误类型，用于区分不同的错误场景
var (
	// ErrComposeConfigUnknownPath 表示 Compose 项目存在但无法从 docker compose ls 或容器 labels 获取配置路径
	ErrComposeConfigUnknownPath = errors.New("无法获取Compose配置路径")
	// ErrComposeConfigInaccessible 表示配置路径已发现但无法访问（权限等问题）
	ErrComposeConfigInaccessible = errors.New("Compose配置路径不可访问")
	// ErrComposeConfigFileNotFound 表示配置文件路径已确定但文件不存在于磁盘
	ErrComposeConfigFileNotFound = errors.New("Compose配置文件不存在")
)

func sanitizeComposeProjectName(projectName string) (string, error) {
	name := strings.TrimSpace(projectName)
	if name == "" {
		return "", fmt.Errorf("Compose项目名不能为空")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("Compose项目名无效")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("Compose项目名不能包含路径分隔符")
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("Compose项目名不能包含 ..")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return "", fmt.Errorf("Compose项目名包含非法字符: %q", r)
		}
	}
	return name, nil
}

// ContainerInfo 容器信息
type ContainerInfo struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Status     string   `json:"status"`
	State      string   `json:"state"`
	Created    string   `json:"created"`
	Ports      []string `json:"ports"`
	Command    string   `json:"command"`
	Mounts     []string `json:"mounts"`
	Size       string   `json:"size"`
	SizeRw     int64    `json:"size_rw"`
	SizeRootFs int64    `json:"size_root_fs"`
}

// ImageInfo 镜像信息
type ImageInfo struct {
	ID         string            `json:"id"`
	Repository string            `json:"repository"`
	Tag        string            `json:"tag"`
	Created    string            `json:"created"`
	Size       int64             `json:"size"`
	Labels     map[string]string `json:"labels"`
}

// ComposeInfo Compose项目信息
type ComposeInfo struct {
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	ContainerCount int      `json:"container_count"`
	ConfigFiles    []string `json:"config_files,omitempty"`
	WorkingDir     string   `json:"working_dir,omitempty"`
	UpdatedAt      string   `json:"updated_at"`
}

// dockerComposeLsItem 用于解析 docker compose ls --format json 的原始输出
// Docker CLI 返回的字段名首字母大写，与 ComposeInfo 的 JSON tag 不匹配
// ConfigFiles 可能是逗号分隔的字符串或数组（取决于 Docker Compose 版本）
type dockerComposeLsItem struct {
	Name        string             `json:"Name"`
	Status      string             `json:"Status"`
	ConfigFiles composeConfigFiles `json:"ConfigFiles"`
	ProjectDir  string             `json:"ProjectDir"`
	WorkingDir  string             `json:"WorkingDir"`
}

// composeConfigFiles 自定义类型，支持从 JSON 字符串（逗号分隔）或数组反序列化
type composeConfigFiles []string

func (c *composeConfigFiles) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*c = nil
		return nil
	}
	// 如果是字符串形式（如 "/path/a.yml,/path/b.yml"）
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*c = splitConfigFilesString(s)
		return nil
	}
	// 如果是数组形式
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	*c = result
	return nil
}

// composeStatusRe 用于解析 docker compose ls 的 Status 字段
// 使用非锚定模式以支持组合状态如 "running(1), exited(2)"
var composeStatusRe = regexp.MustCompile(`(?i)([a-zA-Z_-]+)\s*\(\s*(\d+)\s*\)`)

// statusPriority 定义容器状态优先级，用于从组合状态中选择主要状态
// running 优先级最高，表示有容器在运行
var statusPriority = map[string]int{
	"running":    100,
	"restarting": 80,
	"paused":     60,
	"created":    40,
	"exited":     20,
	"dead":       10,
}

// parseComposeStatus 解析 docker compose ls 的 Status 字段
// 支持单一状态如 "running(1)" 和组合状态如 "running(1), exited(2)"
// 返回: 主要状态(按优先级选择，仅考虑count>0的状态), 总容器数(所有状态容器数之和)
// 特殊情况: 当所有状态的容器数都为0时，返回原始状态字符串
func parseComposeStatus(raw string) (status string, containerCount int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}

	// 查找所有匹配的状态片段
	matches := composeStatusRe.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		// 无法解析，返回原始状态
		return raw, 0
	}

	var totalCount int
	var primaryStatus string
	highestPriority := -1

	for _, m := range matches {
		if len(m) != 3 {
			continue
		}
		stateName := strings.ToLower(strings.TrimSpace(m[1]))
		count, _ := strconv.Atoi(m[2])
		totalCount += count

		// 仅当 count > 0 时才参与主状态竞争
		// 避免 "running(0), exited(2)" 错误地返回 "running"
		if count == 0 {
			continue
		}

		// 根据优先级选择主要状态
		priority, known := statusPriority[stateName]
		if !known {
			priority = 1 // 未知状态给予最低优先级
		}
		if priority > highestPriority {
			highestPriority = priority
			primaryStatus = stateName
		}
	}

	// 如果没有选出主状态（所有状态count都为0或解析失败），返回原始状态
	if primaryStatus == "" {
		return raw, totalCount
	}
	return primaryStatus, totalCount
}

// DockerManager Docker管理器
type DockerManager struct {
	client     *client.Client
	log        *logger.Logger
	ctx        context.Context
	composeDir string
	execConns  map[string]types.HijackedResponse
}

// NewDockerManager 创建Docker管理器
func NewDockerManager(log *logger.Logger) (*DockerManager, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}

	// 创建Compose项目目录
	composeDir := "/tmp/docker-compose"
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		log.Warn("创建Compose目录失败: %v", err)
		composeDir = os.TempDir() + "/docker-compose"
		if err := os.MkdirAll(composeDir, 0755); err != nil {
			log.Warn("创建备用Compose目录失败: %v", err)
		}
	}

	return &DockerManager{
		client:     cli,
		log:        log,
		ctx:        ctx,
		composeDir: composeDir,
		execConns:  make(map[string]types.HijackedResponse),
	}, nil
}

// Close 关闭Docker客户端
func (dm *DockerManager) Close() error {
	if dm.client != nil {
		// 关闭所有 exec 连接
		for id, conn := range dm.execConns {
			conn.Close()
			delete(dm.execConns, id)
		}
		return dm.client.Close()
	}
	return nil
}

// CopyFromContainer 复制容器内文件（包装原生接口，便于 agent 使用）
func (dm *DockerManager) CopyFromContainer(containerID, srcPath string) (io.ReadCloser, container.PathStat, error) {
	return dm.client.CopyFromContainer(dm.ctx, containerID, srcPath)
}

// CopyToContainer 写入文件到容器
func (dm *DockerManager) CopyToContainer(containerID, dstPath string, content io.Reader, options container.CopyToContainerOptions) error {
	return dm.client.CopyToContainer(dm.ctx, containerID, dstPath, content, options)
}

// StatContainerPath 查看容器内路径信息
func (dm *DockerManager) StatContainerPath(containerID, path string) (*container.PathStat, error) {
	stat, err := dm.client.ContainerStatPath(dm.ctx, containerID, path)
	if err != nil {
		return nil, err
	}
	return &stat, nil
}

// RunCommand 在容器内执行命令并返回 stdout/stderr
func (dm *DockerManager) RunCommand(containerID string, cmd []string, env []string) (string, string, error) {
	execIDResp, err := dm.client.ContainerExecCreate(dm.ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Env:          env,
	})
	if err != nil {
		return "", "", fmt.Errorf("创建 exec 失败: %w", err)
	}

	attachResp, err := dm.client.ContainerExecAttach(dm.ctx, execIDResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", fmt.Errorf("附加 exec 失败: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf strings.Builder
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader); err != nil {
		return "", "", fmt.Errorf("读取 exec 输出失败: %w", err)
	}

	inspect, err := dm.client.ContainerExecInspect(dm.ctx, execIDResp.ID)
	if err != nil {
		return "", "", fmt.Errorf("inspect exec 失败: %w", err)
	}
	if inspect.ExitCode != 0 {
		stderrBuf.WriteString(stdoutBuf.String())
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("命令退出码 %d", inspect.ExitCode)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// GetContainers 获取容器列表
func (dm *DockerManager) GetContainers(all bool) ([]ContainerInfo, error) {
	options := container.ListOptions{
		All: all,
	}
	containers, err := dm.client.ContainerList(dm.ctx, options)
	if err != nil {
		return nil, fmt.Errorf("获取容器列表失败: %v", err)
	}

	var containerInfos []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			// 容器名称通常以/开头，需要去除
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		// 格式化端口信息
		var ports []string
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				portStr := fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, p.Type)
				ports = append(ports, portStr)
			} else if p.PrivatePort > 0 {
				portStr := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
				ports = append(ports, portStr)
			}
		}

		// 获取容器详情
		containerDetails, err := dm.client.ContainerInspect(dm.ctx, c.ID)
		if err != nil {
			dm.log.Warn("获取容器 %s 详情失败: %v", c.ID, err)
		}

		// 格式化挂载点信息
		var mounts []string
		for _, m := range containerDetails.Mounts {
			mountStr := fmt.Sprintf("%s:%s:%s", m.Source, m.Destination, m.Mode)
			mounts = append(mounts, mountStr)
		}

		containerInfo := ContainerInfo{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Created: time.Unix(c.Created, 0).Format(time.RFC3339),
			Ports:   ports,
			Command: c.Command,
			Mounts:  mounts,
		}

		containerInfos = append(containerInfos, containerInfo)
	}

	return containerInfos, nil
}

// GetContainerLogs 获取容器日志
func (dm *DockerManager) GetContainerLogs(containerID string, tail int) (string, error) {
	tailStr := "all"
	if tail > 0 {
		tailStr = fmt.Sprintf("%d", tail)
	}

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tailStr,
		Timestamps: true,
	}

	logs, err := dm.client.ContainerLogs(dm.ctx, containerID, options)
	if err != nil {
		return "", fmt.Errorf("获取容器日志失败: %v", err)
	}
	defer logs.Close()

	// 读取日志内容
	logBytes, err := io.ReadAll(logs)
	if err != nil {
		return "", fmt.Errorf("读取容器日志失败: %v", err)
	}

	return string(logBytes), nil
}

// StartContainer 启动容器
func (dm *DockerManager) StartContainer(containerID string) error {
	if err := dm.client.ContainerStart(dm.ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("启动容器失败: %v", err)
	}
	return nil
}

// ExecSession 表示容器内的交互式会话
type ExecSession struct {
	ExecID string `json:"exec_id"`
}

// StartExecSession 启动容器内的交互式终端（tty）
func (dm *DockerManager) StartExecSession(containerID string, cmd []string) (*ExecSession, error) {
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	config := container.ExecOptions{
		User:         "",
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	execResp, err := dm.client.ContainerExecCreate(dm.ctx, containerID, config)
	if err != nil {
		return nil, fmt.Errorf("创建 exec 会话失败: %w", err)
	}

	attachResp, err := dm.client.ContainerExecAttach(dm.ctx, execResp.ID, container.ExecAttachOptions{
		Tty: true,
	})
	if err != nil {
		return nil, fmt.Errorf("附加 exec 会话失败: %w", err)
	}

	dm.execConns[execResp.ID] = attachResp
	return &ExecSession{ExecID: execResp.ID}, nil
}

// WriteExec 写入容器 exec 输入
func (dm *DockerManager) WriteExec(execID string, data string) error {
	conn, ok := dm.execConns[execID]
	if !ok {
		return fmt.Errorf("exec 会话不存在")
	}
	_, err := io.WriteString(conn.Conn, data)
	return err
}

// ResizeExec 调整容器 exec 终端大小
func (dm *DockerManager) ResizeExec(execID string, cols, rows uint) error {
	return dm.client.ContainerExecResize(dm.ctx, execID, container.ResizeOptions{
		Height: uint(rows),
		Width:  uint(cols),
	})
}

// CloseExec 关闭容器 exec 会话
func (dm *DockerManager) CloseExec(execID string) error {
	if conn, ok := dm.execConns[execID]; ok {
		conn.Close()
		delete(dm.execConns, execID)
	}
	return nil
}

// ExecOutput 返回容器 exec 的输出读取器
func (dm *DockerManager) ExecOutput(execID string) (io.Reader, error) {
	conn, ok := dm.execConns[execID]
	if !ok {
		return nil, fmt.Errorf("exec 会话不存在")
	}
	return conn.Reader, nil
}

// ListExec 是否退出
func (dm *DockerManager) IsExecRunning(execID string) bool {
	inspect, err := dm.client.ContainerExecInspect(dm.ctx, execID)
	if err != nil {
		return false
	}
	return inspect.Running
}

// StopContainer 停止容器
func (dm *DockerManager) StopContainer(containerID string, timeout int) error {
	stopOpts := container.StopOptions{}
	if timeout > 0 {
		stopOpts.Timeout = &timeout
	}

	if err := dm.client.ContainerStop(dm.ctx, containerID, stopOpts); err != nil {
		return fmt.Errorf("停止容器失败: %v", err)
	}
	return nil
}

// RestartContainer 重启容器
func (dm *DockerManager) RestartContainer(containerID string, timeout int) error {
	stopOpts := container.StopOptions{}
	if timeout > 0 {
		stopOpts.Timeout = &timeout
	}

	if err := dm.client.ContainerRestart(dm.ctx, containerID, stopOpts); err != nil {
		return fmt.Errorf("重启容器失败: %v", err)
	}
	return nil
}

// RemoveContainer 删除容器
func (dm *DockerManager) RemoveContainer(containerID string, force bool) error {
	// 如果不是强制删除，先检查容器状态
	if !force {
		containerJSON, err := dm.client.ContainerInspect(dm.ctx, containerID)
		if err != nil {
			return fmt.Errorf("检查容器状态失败: %v", err)
		}

		// 如果容器正在运行，拒绝删除
		if containerJSON.State.Running {
			return fmt.Errorf("容器正在运行中，无法删除。请先停止容器或使用强制删除")
		}
	}

	options := container.RemoveOptions{
		Force:         force,
		RemoveVolumes: false,
	}

	if err := dm.client.ContainerRemove(dm.ctx, containerID, options); err != nil {
		return fmt.Errorf("删除容器失败: %v", err)
	}
	return nil
}

// GetImages 获取镜像列表
func (dm *DockerManager) GetImages() ([]ImageInfo, error) {
	// 使用命令行方式获取镜像列表，避免API版本兼容性问题
	cmd := exec.Command("docker", "images", "--format", "{{json .}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取镜像列表失败: %v", err)
	}

	// 解析JSON输出
	lines := strings.Split(string(output), "\n")
	var imageInfos []ImageInfo

	for _, line := range lines {
		if line == "" {
			continue
		}

		// 解析每一行的JSON
		var imgData struct {
			Repository string
			Tag        string
			ID         string
			CreatedAt  string
			Size       string
		}

		if err := json.Unmarshal([]byte(line), &imgData); err != nil {
			dm.log.Warn("解析镜像信息失败: %v", err)
			continue
		}

		// 转换大小为数字
		// docker images 输出格式如 "2.1GB"、"99.4MB"、"512kB"、"1.2TB"
		size := parseImageSize(imgData.Size)

		imageInfos = append(imageInfos, ImageInfo{
			ID:         imgData.ID,
			Repository: imgData.Repository,
			Tag:        imgData.Tag,
			Created:    imgData.CreatedAt,
			Size:       size,
			Labels:     make(map[string]string),
		})
	}

	return imageInfos, nil
}

// PullImage 拉取镜像
func (dm *DockerManager) PullImage(imageRef string) error {
	// 使用命令行方式拉取镜像，避免认证问题
	cmd := exec.Command("docker", "pull", imageRef)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("拉取镜像失败: %v, 输出: %s", err, string(output))
	}
	return nil
}

// RemoveImage 删除镜像
func (dm *DockerManager) RemoveImage(imageID string, force bool) error {
	// 规范化镜像引用，解决 "sha256:<短hex>" 被误解析为 "name:tag" 导致 404 的问题
	imageRef := normalizeImageRef(imageID)
	if imageRef == "" {
		return fmt.Errorf("删除镜像失败: 空的镜像ID")
	}

	// 使用命令行方式删除镜像，避免API版本兼容性问题
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, imageRef)

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("删除镜像失败: %v, 输出: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// normalizeImageRef 规范化镜像引用
// 处理 "sha256:<短hex>" 格式，避免被 Docker 误解析为 "repository:tag"
// 例如 "sha256:abc123" 会被剥离前缀变成 "abc123"
func normalizeImageRef(imageID string) string {
	ref := strings.TrimSpace(imageID)
	if ref == "" {
		return ""
	}

	// 如果是 sha256 前缀格式
	if strings.HasPrefix(ref, "sha256:") {
		hexPart := strings.TrimPrefix(ref, "sha256:")
		// 如果是完整的 64 字符 hex，保留原格式（Docker 可以正确处理）
		// 如果是截断的短 hex（< 64 字符），剥离前缀避免被误解析为 name:tag
		if len(hexPart) > 0 && len(hexPart) < 64 && isHexString(hexPart) {
			return hexPart
		}
	}

	return ref
}

// isHexString 检查字符串是否为有效的十六进制字符串
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetComposes 获取Docker Compose项目列表
func (dm *DockerManager) GetComposes() ([]ComposeInfo, error) {
	// 确保Compose目录存在
	if err := os.MkdirAll(dm.composeDir, 0755); err != nil {
		return nil, fmt.Errorf("创建Compose目录失败: %v", err)
	}

	// 使用 docker compose ls 获取项目列表
	cmd := exec.Command("docker", "compose", "ls", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// docker compose ls 失败，回退到读取托管目录
		return dm.getComposesFromManagedDir()
	}

	// 解析JSON输出到中间结构体（Docker CLI 返回的字段名首字母大写）
	var rawItems []dockerComposeLsItem
	if err := json.Unmarshal(output, &rawItems); err != nil {
		return nil, fmt.Errorf("解析Docker Compose项目列表失败: %v", err)
	}

	// 预先通过容器 labels 获取补充信息（用于 ls 输出缺少 WorkingDir 的情况）
	labelIndex := dm.buildComposeLabelIndex()

	// 转换为 ComposeInfo，正确映射字段
	composes := make([]ComposeInfo, 0, len(rawItems))
	for _, item := range rawItems {
		status, containerCount := parseComposeStatus(item.Status)

		workingDir := firstNonEmpty(item.WorkingDir, item.ProjectDir)
		configFiles := []string(item.ConfigFiles)

		// 如果 ls 输出缺少关键字段，尝试从容器 labels 补充
		if meta, ok := labelIndex[item.Name]; ok {
			if workingDir == "" {
				workingDir = meta.workingDir
			}
			if len(configFiles) == 0 && len(meta.configFiles) > 0 {
				configFiles = meta.configFiles
			}
		}

		composes = append(composes, ComposeInfo{
			Name:           item.Name,
			Status:         status,
			ContainerCount: containerCount,
			ConfigFiles:    configFiles,
			WorkingDir:     workingDir,
			UpdatedAt:      time.Now().Format(time.RFC3339),
		})
	}

	return composes, nil
}

// getComposesFromManagedDir 从托管目录读取 Compose 项目列表（docker compose ls 不可用时的回退）
func (dm *DockerManager) getComposesFromManagedDir() ([]ComposeInfo, error) {
	entries, err := os.ReadDir(dm.composeDir)
	if err != nil {
		return nil, fmt.Errorf("读取Compose目录失败: %v", err)
	}

	var composes []ComposeInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectName := entry.Name()
		projectPath := filepath.Join(dm.composeDir, projectName)

		configFile := findComposeFile(projectPath)
		if configFile == "" {
			continue
		}

		composes = append(composes, ComposeInfo{
			Name:           projectName,
			Status:         "unknown",
			ContainerCount: 0,
			ConfigFiles:    []string{configFile},
			WorkingDir:     projectPath,
			UpdatedAt:      time.Now().Format(time.RFC3339),
		})
	}

	return composes, nil
}

// GetComposeConfig 获取Compose项目配置内容
// 优先通过 docker compose config 获取渲染后的配置（处理多文件合并、变量替换等）
// 按以下顺序尝试发现配置：1) docker compose ls 2) 容器 labels 3) 托管目录
func (dm *DockerManager) GetComposeConfig(projectName string) (string, error) {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return "", err
	}

	// 尝试发现项目的配置元数据
	meta, discoverErr := dm.discoverComposeProjectMeta(projectName)
	if discoverErr == nil && meta != nil && len(meta.configFiles) > 0 {
		// 解析配置文件路径（处理相对路径）
		absFiles, err := resolveConfigFilePaths(meta.workingDir, meta.configFiles)
		if err != nil {
			return "", err
		}

		// 检查文件是否都可访问
		if err := checkFilesAccessible(absFiles); err != nil {
			return "", err
		}

		// 使用 docker compose config 获取渲染后的配置
		return dm.runComposeConfig(projectName, meta.workingDir, absFiles)
	}

	// 回退：尝试托管目录 /tmp/docker-compose/{projectName}
	projectPath := filepath.Join(dm.composeDir, projectName)
	configFile := findComposeFile(projectPath)
	if configFile == "" {
		// 如果之前有发现错误，返回该错误
		if discoverErr != nil {
			return "", fmt.Errorf("%w: %s", ErrComposeConfigUnknownPath, projectName)
		}
		return "", fmt.Errorf("%w: %s", ErrComposeConfigFileNotFound, projectPath)
	}

	// 检查配置文件是否可访问
	if err := checkFilesAccessible([]string{configFile}); err != nil {
		return "", err
	}

	return dm.runComposeConfig(projectName, projectPath, []string{configFile})
}

// ComposeUp 启动Compose项目
func (dm *DockerManager) ComposeUp(projectName string) error {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return err
	}

	// 检查配置文件
	projectPath := filepath.Join(dm.composeDir, projectName)
	configFile := filepath.Join(projectPath, "docker-compose.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 尝试检查docker-compose.yaml
		configFile = filepath.Join(projectPath, "docker-compose.yaml")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return fmt.Errorf("Compose配置文件不存在")
		}
	}

	// 执行docker-compose up命令
	cmd := exec.Command("docker", "compose", "-f", configFile, "-p", projectName, "up", "-d")
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("启动Compose项目失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// ComposeDown 停止Compose项目
func (dm *DockerManager) ComposeDown(projectName string) error {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return err
	}

	// 检查配置文件
	projectPath := filepath.Join(dm.composeDir, projectName)
	configFile := filepath.Join(projectPath, "docker-compose.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 尝试检查docker-compose.yaml
		configFile = filepath.Join(projectPath, "docker-compose.yaml")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return fmt.Errorf("Compose配置文件不存在")
		}
	}

	// 执行docker-compose down命令
	cmd := exec.Command("docker", "compose", "-f", configFile, "-p", projectName, "down")
	cmd.Dir = projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止Compose项目失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// RemoveCompose 删除Compose项目
func (dm *DockerManager) RemoveCompose(projectName string) error {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return err
	}

	projectPath := filepath.Join(dm.composeDir, projectName)

	// 首先停止项目
	if err := dm.ComposeDown(projectName); err != nil {
		dm.log.Warn("停止Compose项目 %s 失败: %v", projectName, err)
		// 继续删除，不返回错误
	}

	// 删除项目目录
	if err := os.RemoveAll(projectPath); err != nil {
		return fmt.Errorf("删除Compose项目目录失败: %v", err)
	}

	return nil
}

// CreateCompose 创建Compose项目
func (dm *DockerManager) CreateCompose(projectName string, content string) error {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return err
	}

	// 创建项目目录
	projectPath := filepath.Join(dm.composeDir, projectName)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return fmt.Errorf("创建项目目录失败: %v", err)
	}

	// 创建docker-compose.yml文件
	configFile := filepath.Join(projectPath, "docker-compose.yml")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}

	return nil
}

// CreateContainer 创建容器
func (dm *DockerManager) CreateContainer(name string, image string, ports []string, volumes []string,
	env map[string]string, cmd string, restart string, network string) (string, error) {

	dm.log.Info("创建容器: 名称=%s, 镜像=%s", name, image)

	// 使用命令行方式创建容器，避免API版本兼容性问题
	args := []string{"run", "-d", "--name", name}

	// 添加重启策略
	if restart != "" {
		args = append(args, "--restart", restart)
	}

	// 添加网络模式
	if network != "" {
		args = append(args, "--network", network)
	}

	// 添加端口映射
	for _, port := range ports {
		if port != "" {
			args = append(args, "-p", port)
		}
	}

	// 添加卷映射
	for _, volume := range volumes {
		if volume != "" {
			args = append(args, "-v", volume)
		}
	}

	// 添加环境变量
	for k, v := range env {
		if k != "" {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
		}
	}

	// 添加镜像名称
	args = append(args, image)

	// 添加命令
	if cmd != "" {
		cmdParts := strings.Fields(cmd)
		args = append(args, cmdParts...)
	}

	dm.log.Info("执行Docker命令: docker %s", strings.Join(args, " "))

	// 执行docker run命令
	command := exec.Command("docker", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("创建容器失败: %v, 输出: %s", err, string(output))
	}

	// 输出通常是新创建的容器ID
	containerID := strings.TrimSpace(string(output))
	dm.log.Info("容器创建成功，ID: %s", containerID)

	return containerID, nil
}

// ============================================================================
// Compose 配置发现相关辅助函数和类型
// ============================================================================

// composeProjectMeta 存储 Compose 项目的配置元数据
type composeProjectMeta struct {
	workingDir  string
	configFiles []string
}

// composeLabelMeta 从容器 labels 解析出的 Compose 项目元数据
type composeLabelMeta struct {
	workingDir  string
	configFiles []string
}

// discoverComposeProjectMeta 发现指定 Compose 项目的配置元数据
// 优先从 docker compose ls 获取，失败则从容器 labels 兜底
func (dm *DockerManager) discoverComposeProjectMeta(projectName string) (*composeProjectMeta, error) {
	// 1) 尝试从 docker compose ls 获取
	if meta, err := dm.discoverMetaFromComposeLs(projectName); err == nil && meta != nil {
		if meta.workingDir != "" && len(meta.configFiles) > 0 {
			return meta, nil
		}
	}

	// 2) 回退到容器 labels
	if meta, err := dm.discoverMetaFromContainerLabels(projectName); err == nil && meta != nil {
		if meta.workingDir != "" && len(meta.configFiles) > 0 {
			return meta, nil
		}
	}

	return nil, fmt.Errorf("无法发现项目 %s 的配置信息", projectName)
}

// discoverMetaFromComposeLs 从 docker compose ls 输出中获取项目元数据
func (dm *DockerManager) discoverMetaFromComposeLs(projectName string) (*composeProjectMeta, error) {
	cmd := exec.Command("docker", "compose", "ls", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var items []dockerComposeLsItem
	if err := json.Unmarshal(output, &items); err != nil {
		return nil, err
	}

	for _, item := range items {
		if item.Name != projectName {
			continue
		}
		return &composeProjectMeta{
			workingDir:  firstNonEmpty(item.WorkingDir, item.ProjectDir),
			configFiles: []string(item.ConfigFiles),
		}, nil
	}

	return nil, fmt.Errorf("项目 %s 未在 compose ls 中找到", projectName)
}

// discoverMetaFromContainerLabels 从容器 labels 中获取指定项目的元数据
// 仅查询属于该项目的容器，避免全量 inspect 的性能开销
func (dm *DockerManager) discoverMetaFromContainerLabels(projectName string) (*composeProjectMeta, error) {
	// 只获取属于该项目的一个容器
	filterLabel := fmt.Sprintf("label=com.docker.compose.project=%s", projectName)
	cmd := exec.Command("docker", "ps", "-a", "--filter", filterLabel, "--format", "{{.ID}}", "-n", "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return nil, fmt.Errorf("项目 %s 未在容器 labels 中找到", projectName)
	}

	// inspect 单个容器
	cmd = exec.Command("docker", "inspect", containerID)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var inspected []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &inspected); err != nil {
		return nil, err
	}

	if len(inspected) == 0 || inspected[0].Config.Labels == nil {
		return nil, fmt.Errorf("项目 %s 的容器 labels 为空", projectName)
	}

	labels := inspected[0].Config.Labels
	return &composeProjectMeta{
		workingDir:  strings.TrimSpace(labels["com.docker.compose.project.working_dir"]),
		configFiles: splitConfigFilesString(labels["com.docker.compose.project.config_files"]),
	}, nil
}

// buildComposeLabelIndex 构建所有 Compose 项目的 labels 索引
// 通过检查所有带有 com.docker.compose.project label 的容器
func (dm *DockerManager) buildComposeLabelIndex() map[string]composeLabelMeta {
	result := make(map[string]composeLabelMeta)

	// 获取所有属于 Compose 项目的容器 ID
	cmd := exec.Command("docker", "ps", "-a", "--filter", "label=com.docker.compose.project", "--format", "{{.ID}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return result
	}

	var containerIDs []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			containerIDs = append(containerIDs, line)
		}
	}

	if len(containerIDs) == 0 {
		return result
	}

	// 批量 inspect 所有容器以提高性能
	args := append([]string{"inspect"}, containerIDs...)
	cmd = exec.Command("docker", args...)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return result
	}

	var inspected []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(output, &inspected); err != nil {
		return result
	}

	for _, item := range inspected {
		labels := item.Config.Labels
		if labels == nil {
			continue
		}

		projectName := strings.TrimSpace(labels["com.docker.compose.project"])
		if projectName == "" {
			continue
		}

		// 每个项目只取第一个容器的信息（同一项目的容器 labels 应该相同）
		if _, exists := result[projectName]; exists {
			continue
		}

		result[projectName] = composeLabelMeta{
			workingDir:  strings.TrimSpace(labels["com.docker.compose.project.working_dir"]),
			configFiles: splitConfigFilesString(labels["com.docker.compose.project.config_files"]),
		}
	}

	return result
}

// runComposeConfig 执行 docker compose config 命令获取渲染后的配置
func (dm *DockerManager) runComposeConfig(projectName, workingDir string, configFiles []string) (string, error) {
	// 如果 workingDir 为空但有配置文件，使用第一个配置文件的目录作为工作目录
	// 这对于加载 .env 文件和解析相对 include 路径很重要
	if workingDir == "" && len(configFiles) > 0 {
		workingDir = filepath.Dir(configFiles[0])
	}

	args := []string{"compose"}
	if workingDir != "" {
		args = append(args, "--project-directory", workingDir)
	}
	args = append(args, "-p", projectName)
	for _, f := range configFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "config")

	cmd := exec.Command("docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose config 执行失败: %v, stderr: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// ============================================================================
// 通用辅助函数
// ============================================================================

// parseImageSize 解析 docker images 输出的人类可读大小字符串为字节数
// 支持格式: "2.1GB"、"99.4MB"、"512kB"、"1.2TB"、"800B"
func parseImageSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	// 按后缀从长到短匹配，避免 "B" 提前被匹配
	suffixes := []struct {
		suffix     string
		multiplier float64
	}{
		{"TB", 1024 * 1024 * 1024 * 1024},
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"kB", 1024},
		{"B", 1},
	}

	for _, s := range suffixes {
		if strings.HasSuffix(sizeStr, s.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(sizeStr, s.suffix))
			if val, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int64(val * s.multiplier)
			}
			return 0
		}
	}

	return 0
}

// splitConfigFilesString 将逗号分隔的配置文件字符串拆分为数组
func splitConfigFilesString(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// resolveConfigFilePaths 解析配置文件路径，将相对路径转换为绝对路径
func resolveConfigFilePaths(workingDir string, configFiles []string) ([]string, error) {
	if len(configFiles) == 0 {
		return nil, fmt.Errorf("%w: 配置文件列表为空", ErrComposeConfigUnknownPath)
	}

	result := make([]string, 0, len(configFiles))
	for _, f := range configFiles {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		if filepath.IsAbs(f) {
			result = append(result, f)
			continue
		}

		// 相对路径需要 workingDir 来解析
		if strings.TrimSpace(workingDir) == "" {
			return nil, fmt.Errorf("%w: 存在相对路径 %s 但缺少工作目录", ErrComposeConfigUnknownPath, f)
		}
		result = append(result, filepath.Join(workingDir, f))
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("%w: 解析后配置文件列表为空", ErrComposeConfigUnknownPath)
	}
	return result, nil
}

// checkFilesAccessible 检查所有文件是否可访问
func checkFilesAccessible(files []string) error {
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", ErrComposeConfigFileNotFound, f)
			}
			return fmt.Errorf("%w: %s (%v)", ErrComposeConfigInaccessible, f, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%w: %s 是目录而非文件", ErrComposeConfigInaccessible, f)
		}
	}
	return nil
}

// findComposeFile 在指定目录中查找 Compose 配置文件
// 按优先级检查: docker-compose.yml, docker-compose.yaml, compose.yml, compose.yaml
func findComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
