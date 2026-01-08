package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/user/server-ops-agent/pkg/logger"
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
	Name           string `json:"name"`
	Status         string `json:"status"`
	ContainerCount int    `json:"container_count"`
	UpdatedAt      string `json:"updated_at"`
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
		var size int64
		if imgData.Size != "" {
			sizeStr := strings.TrimSuffix(imgData.Size, "B")
			sizeStr = strings.TrimSpace(sizeStr)

			if strings.HasSuffix(sizeStr, "MB") {
				sizeVal := strings.TrimSuffix(sizeStr, "MB")
				if val, err := strconv.ParseFloat(sizeVal, 64); err == nil {
					size = int64(val * 1024 * 1024)
				}
			} else if strings.HasSuffix(sizeStr, "GB") {
				sizeVal := strings.TrimSuffix(sizeStr, "GB")
				if val, err := strconv.ParseFloat(sizeVal, 64); err == nil {
					size = int64(val * 1024 * 1024 * 1024)
				}
			} else if strings.HasSuffix(sizeStr, "kB") {
				sizeVal := strings.TrimSuffix(sizeStr, "kB")
				if val, err := strconv.ParseFloat(sizeVal, 64); err == nil {
					size = int64(val * 1024)
				}
			}
		}

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
	// 使用命令行方式删除镜像，避免API版本兼容性问题
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, imageID)

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("删除镜像失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// GetComposes 获取Docker Compose项目列表
func (dm *DockerManager) GetComposes() ([]ComposeInfo, error) {
	// 确保Compose目录存在
	if err := os.MkdirAll(dm.composeDir, 0755); err != nil {
		return nil, fmt.Errorf("创建Compose目录失败: %v", err)
	}

	// 使用docker-compose命令获取项目列表
	// 尝试使用docker compose (新版本)
	cmd := exec.Command("docker", "compose", "ls", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 读取Compose目录下的所有项目文件夹
		entries, err := os.ReadDir(dm.composeDir)
		if err != nil {
			return nil, fmt.Errorf("读取Compose目录失败: %v", err)
		}

		var composes []ComposeInfo
		for _, entry := range entries {
			if entry.IsDir() {
				projectName := entry.Name()
				projectPath := filepath.Join(dm.composeDir, projectName)

				// 检查docker-compose.yml文件是否存在
				configFile := filepath.Join(projectPath, "docker-compose.yml")
				if _, err := os.Stat(configFile); os.IsNotExist(err) {
					// 尝试检查docker-compose.yaml
					configFile = filepath.Join(projectPath, "docker-compose.yaml")
					if _, err := os.Stat(configFile); os.IsNotExist(err) {
						// 两种扩展名都不存在，跳过
						continue
					}
				}

				composes = append(composes, ComposeInfo{
					Name:           projectName,
					Status:         "unknown",
					ContainerCount: 0,
					UpdatedAt:      time.Now().Format(time.RFC3339),
				})
			}
		}

		return composes, nil
	}

	// 解析JSON输出
	var composes []ComposeInfo
	if err := json.Unmarshal(output, &composes); err != nil {
		return nil, fmt.Errorf("解析Docker Compose项目列表失败: %v", err)
	}

	return composes, nil
}

// GetComposeConfig 获取Compose项目配置内容
func (dm *DockerManager) GetComposeConfig(projectName string) (string, error) {
	projectName, err := sanitizeComposeProjectName(projectName)
	if err != nil {
		return "", err
	}

	// 检查配置文件
	projectPath := filepath.Join(dm.composeDir, projectName)
	configFile := filepath.Join(projectPath, "docker-compose.yml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// 尝试检查docker-compose.yaml
		configFile = filepath.Join(projectPath, "docker-compose.yaml")
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			return "", fmt.Errorf("Compose配置文件不存在")
		}
	}

	// 读取配置文件内容
	content, err := os.ReadFile(configFile)
	if err != nil {
		return "", fmt.Errorf("读取Compose配置文件失败: %v", err)
	}

	return string(content), nil
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
