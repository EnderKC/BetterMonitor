//go:build !windows && !monitor_only

package nginx

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/user/server-ops-agent/pkg/logger"
	"golang.org/x/sys/unix"
)

// SiteConfig 声明式站点配置
type SiteConfig struct {
	PrimaryDomain     string            `json:"primary_domain"`
	ExtraDomains      []string          `json:"extra_domains"`
	RootDir           string            `json:"root_dir"`
	Index             []string          `json:"index"`
	PHPVersion        string            `json:"php_version"`
	Proxy             ProxyConfig       `json:"proxy"`
	EnableHTTPS       bool              `json:"enable_https"`
	ForceSSL          bool              `json:"force_ssl"`
	SSL               SSLPaths          `json:"ssl"`
	HTTPChallengeDir  string            `json:"http_challenge_dir"`
	ClientMaxBodySize string            `json:"client_max_body_size"` // 文件上传大小限制，如"10m", "100m"
	Labels            map[string]string `json:"labels,omitempty"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// SSLPaths 存储证书路径
type SSLPaths struct {
	Certificate    string `json:"certificate"`
	CertificateKey string `json:"certificate_key"`
}

// CertificateInfo 证书状态
type CertificateInfo struct {
	Valid    bool   `json:"valid"`
	Expiry   string `json:"expiry"`
	Issuer   string `json:"issuer"`
	DaysLeft int    `json:"days_left"`
	Path     string `json:"path"`
}

// SiteSummary 对外输出的网站结构
type SiteSummary struct {
	SiteConfig  SiteConfig       `json:"site"`
	Type        string           `json:"type"`
	Certificate *CertificateInfo `json:"certificate,omitempty"`
	HostRootDir string           `json:"host_root_dir,omitempty"`
}

// HostPaths 宿主机路径布局
type HostPaths struct {
	Base  string
	Conf  string
	ConfD string
	Vhost string
	Meta  string
	Logs  string
	WWW   string
	SSL   string
}

// ContainerPaths 容器内部路径布局
type ContainerPaths struct {
	Conf string
	Logs string
	WWW  string
	SSL  string
}

// NginxClient 负责OpenResty容器与配置生命周期
type NginxClient struct {
	ctx            context.Context
	docker         *client.Client
	log            *logger.Logger
	containerID    string
	containerName  string
	image          string
	hostPaths      HostPaths
	containerPaths ContainerPaths
	mu             sync.Mutex
}

// ClientOption 定制NginxClient
type ClientOption func(*NginxClient)

// WithContainerName 设置容器名称
func WithContainerName(name string) ClientOption {
	return func(c *NginxClient) {
		if name != "" {
			c.containerName = name
		}
	}
}

// WithHostBaseDir 调整宿主机根目录
func WithHostBaseDir(dir string) ClientOption {
	return func(c *NginxClient) {
		if dir != "" {
			c.hostPaths.Base = dir
			c.rebuildHostPaths()
		}
	}
}

// WithImage 指定OpenResty镜像
func WithImage(image string) ClientOption {
	return func(c *NginxClient) {
		if image != "" {
			c.image = image
		}
	}
}

// NewNginxClient 创建新的客户端
func NewNginxClient(log *logger.Logger, opts ...ClientOption) (*NginxClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("初始化Docker客户端失败: %w", err)
	}

	if log == nil {
		log, _ = logger.New("", "info")
	}

	c := &NginxClient{
		ctx:           context.Background(),
		docker:        cli,
		log:           log,
		containerName: "openresty",
		image:         "openresty/openresty:latest",
		hostPaths: HostPaths{
			Base: "/opt/node/openresty",
		},
		containerPaths: ContainerPaths{
			Conf: "/usr/local/openresty/nginx/conf",
			Logs: "/usr/local/openresty/nginx/logs",
			WWW:  "/www",
			SSL:  "/usr/local/openresty/nginx/conf/ssl",
		},
	}
	c.rebuildHostPaths()

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Close 释放Docker客户端
func (c *NginxClient) Close() error {
	if c.docker != nil {
		return c.docker.Close()
	}
	return nil
}

func (c *NginxClient) rebuildHostPaths() {
	base := c.hostPaths.Base
	c.hostPaths.Conf = filepath.Join(base, "conf")
	c.hostPaths.ConfD = filepath.Join(c.hostPaths.Conf, "conf.d")
	c.hostPaths.Vhost = filepath.Join(c.hostPaths.Conf, "vhost")
	c.hostPaths.Meta = filepath.Join(c.hostPaths.Conf, "sites")
	c.hostPaths.Logs = filepath.Join(base, "logs")
	c.hostPaths.WWW = filepath.Join(base, "www")
	c.hostPaths.SSL = filepath.Join(base, "ssl")
}

// AllDomains 返回所有域名
func (s *SiteConfig) AllDomains() []string {
	names := []string{}
	if s.PrimaryDomain != "" {
		names = append(names, s.PrimaryDomain)
	}
	for _, d := range s.ExtraDomains {
		if d == "" {
			continue
		}
		names = append(names, d)
	}
	return names
}

// Validate 校验配置
func (s *SiteConfig) Validate() error {
	if s.PrimaryDomain == "" {
		return errors.New("缺少主域名")
	}
	if s.RootDir == "" {
		return errors.New("缺少站点根目录")
	}
	return nil
}

// CreateWebsite 根据配置渲染并应用站点
func (c *NginxClient) CreateWebsite(config SiteConfig) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureContainer(); err != nil {
		return "", err
	}

	normalized, err := c.normalizeSiteConfig(config)
	if err != nil {
		return "", err
	}

	return c.applySiteConfig(normalized)
}

// IssueCertificate 调用ACME流程并写入证书
func (c *NginxClient) IssueCertificate(req CertificateRequest) (*CertificateResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureContainer(); err != nil {
		return nil, err
	}

	if req.Webroot == "" {
		req.Webroot = filepath.Join(c.hostPaths.WWW, "common")
	}

	result, err := ObtainCertificate(req)
	if err != nil {
		return nil, err
	}

	targetDomain := req.PrimaryDomain()
	if targetDomain == "" {
		return result, nil
	}

	sslDir := filepath.Join(c.hostPaths.SSL, sanitizeName(targetDomain))
	if err := os.MkdirAll(sslDir, 0755); err != nil {
		return nil, fmt.Errorf("创建证书目录失败: %w", err)
	}

	certPath := filepath.Join(sslDir, "fullchain.pem")
	keyPath := filepath.Join(sslDir, "privkey.pem")

	if err := os.WriteFile(certPath, result.CertificatePEM, 0600); err != nil {
		return nil, fmt.Errorf("写入证书失败: %w", err)
	}
	if err := os.WriteFile(keyPath, result.PrivateKeyPEM, 0600); err != nil {
		return nil, fmt.Errorf("写入私钥失败: %w", err)
	}

	result.CertificatePath = c.containerPathFromHost(certPath)
	result.KeyPath = c.containerPathFromHost(keyPath)

	return result, nil
}

// ListSites 返回当前所有站点的描述信息
func (c *NginxClient) ListSites() ([]SiteSummary, error) {
	if err := c.ensureDirectories(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(c.hostPaths.Meta)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	sites := make([]SiteSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(c.hostPaths.Meta, entry.Name()))
		if err != nil {
			continue
		}

		var site SiteConfig
		if err := json.Unmarshal(data, &site); err != nil {
			continue
		}

		hostDir := site.RootDir
		if converted, convErr := c.hostPathFromContainer(site.RootDir); convErr == nil && converted != "" {
			hostDir = converted
		}

		summary := SiteSummary{
			SiteConfig:  site,
			Type:        determineSiteType(site),
			Certificate: c.buildCertificateInfo(&site),
		}
		summary.HostRootDir = hostDir
		sites = append(sites, summary)
	}

	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].SiteConfig.PrimaryDomain < sites[j].SiteConfig.PrimaryDomain
	})

	return sites, nil
}

// GetSiteDetail 获取单个站点的详细配置
func (c *NginxClient) GetSiteDetail(domain string) (*SiteSummary, error) {
	normalized, err := NormalizeDomain(domain, false)
	if err != nil {
		return nil, err
	}
	domain = normalized

	if err := c.ensureDirectories(); err != nil {
		return nil, err
	}

	// 读取站点元数据JSON文件
	metaPath := c.siteMetadataPath(domain)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("站点 %s 不存在", domain)
		}
		return nil, fmt.Errorf("读取站点配置失败: %w", err)
	}

	var site SiteConfig
	if err := json.Unmarshal(data, &site); err != nil {
		return nil, fmt.Errorf("解析站点配置失败: %w", err)
	}

	// 确保必要字段有默认值
	if len(site.Index) == 0 {
		site.Index = []string{"index.php", "index.html"}
	}
	// 确保SSL字段至少为空字符串(避免前端显示undefined)
	if site.SSL.Certificate == "" {
		site.SSL.Certificate = ""
	}
	if site.SSL.CertificateKey == "" {
		site.SSL.CertificateKey = ""
	}

	// 转换容器路径为宿主机路径
	hostDir := site.RootDir
	if converted, convErr := c.hostPathFromContainer(site.RootDir); convErr == nil && converted != "" {
		hostDir = converted
	}

	summary := &SiteSummary{
		SiteConfig:  site,
		Type:        determineSiteType(site),
		Certificate: c.buildCertificateInfo(&site),
		HostRootDir: hostDir,
	}

	return summary, nil
}

// GetRuntimeState 返回OpenResty容器是否存在以及运行状态
func (c *NginxClient) GetRuntimeState() (bool, bool, error) {
	filtersArgs := filters.NewArgs()
	filtersArgs.Add("name", c.containerName)
	containers, err := c.docker.ContainerList(c.ctx, container.ListOptions{
		All:     true,
		Filters: filtersArgs,
	})
	if err != nil {
		return false, false, err
	}

	if len(containers) == 0 {
		return false, false, nil
	}

	c.containerID = containers[0].ID
	running := containers[0].State == "running"
	return true, running, nil
}

// InstallOpenResty 安装并启动OpenResty容器
func (c *NginxClient) InstallOpenResty() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureContainer()
}

// InstallOpenRestyWithLogger 带日志输出的安装
func (c *NginxClient) InstallOpenRestyWithLogger(logFunc func(string)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ensureContainerWithLogger(logFunc)
}

func (c *NginxClient) ensureContainer() error {
	if err := c.ensureDirectories(); err != nil {
		return err
	}
	if err := c.ensureBaseConfig(); err != nil {
		return err
	}

	if c.containerID != "" {
		inspect, err := c.docker.ContainerInspect(c.ctx, c.containerID)
		if err == nil && inspect.State != nil && inspect.State.Running {
			return nil
		}
	}

	filtersArgs := filters.NewArgs()
	filtersArgs.Add("name", c.containerName)
	containers, err := c.docker.ContainerList(c.ctx, container.ListOptions{
		All:     true,
		Filters: filtersArgs,
	})
	if err != nil {
		return fmt.Errorf("列举OpenResty容器失败: %w", err)
	}

	if len(containers) == 0 {
		if err := c.ensureImage(); err != nil {
			return err
		}
		resp, err := c.docker.ContainerCreate(
			c.ctx,
			&container.Config{
				Image: c.image,
			},
			&container.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:%s", c.hostPaths.Conf, c.containerPaths.Conf),
					fmt.Sprintf("%s:%s", c.hostPaths.Logs, c.containerPaths.Logs),
					fmt.Sprintf("%s:%s", c.hostPaths.WWW, c.containerPaths.WWW),
					fmt.Sprintf("%s:%s", c.hostPaths.SSL, c.containerPaths.SSL),
				},
				NetworkMode: "host",
				RestartPolicy: container.RestartPolicy{
					Name: "always",
				},
			},
			nil,
			nil,
			c.containerName,
		)
		if err != nil {
			return fmt.Errorf("创建OpenResty容器失败: %w", err)
		}
		c.containerID = resp.ID
	} else {
		c.containerID = containers[0].ID
	}

	inspect, err := c.docker.ContainerInspect(c.ctx, c.containerID)
	if err != nil {
		return fmt.Errorf("Inspect容器失败: %w", err)
	}

	if !inspect.State.Running {
		if err := c.docker.ContainerStart(c.ctx, c.containerID, container.StartOptions{}); err != nil {
			return fmt.Errorf("启动OpenResty容器失败: %w", err)
		}
	}

	return nil
}

func (c *NginxClient) ensureContainerWithLogger(logFunc func(string)) error {
	logFunc("[1/6] 检查并创建目录...")
	if err := c.ensureDirectories(); err != nil {
		return err
	}
	logFunc("✓ 目录创建完成")

	logFunc("[2/6] 生成Nginx基础配置...")
	if err := c.ensureBaseConfig(); err != nil {
		return err
	}
	logFunc("✓ 配置文件生成完成")

	if c.containerID != "" {
		inspect, err := c.docker.ContainerInspect(c.ctx, c.containerID)
		if err == nil && inspect.State != nil && inspect.State.Running {
			logFunc("✓ OpenResty容器已在运行")
			return nil
		}
	}

	logFunc("[3/6] 检查现有容器...")
	filtersArgs := filters.NewArgs()
	filtersArgs.Add("name", c.containerName)
	containers, err := c.docker.ContainerList(c.ctx, container.ListOptions{
		All:     true,
		Filters: filtersArgs,
	})
	if err != nil {
		return fmt.Errorf("列举OpenResty容器失败: %w", err)
	}

	if len(containers) == 0 {
		logFunc("[4/6] 拉取OpenResty镜像 (这可能需要几分钟)...")
		if err := c.ensureImageWithLogger(logFunc); err != nil {
			return err
		}
		logFunc("✓ 镜像拉取完成")

		logFunc("[5/6] 创建OpenResty容器...")
		resp, err := c.docker.ContainerCreate(
			c.ctx,
			&container.Config{
				Image: c.image,
			},
			&container.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:%s", c.hostPaths.Conf, c.containerPaths.Conf),
					fmt.Sprintf("%s:%s", c.hostPaths.Logs, c.containerPaths.Logs),
					fmt.Sprintf("%s:%s", c.hostPaths.WWW, c.containerPaths.WWW),
					fmt.Sprintf("%s:%s", c.hostPaths.SSL, c.containerPaths.SSL),
				},
				NetworkMode: "host",
				RestartPolicy: container.RestartPolicy{
					Name: "always",
				},
			},
			nil,
			nil,
			c.containerName,
		)
		if err != nil {
			return fmt.Errorf("创建OpenResty容器失败: %w", err)
		}
		c.containerID = resp.ID
		logFunc("✓ 容器创建完成")
	} else {
		c.containerID = containers[0].ID
		logFunc("✓ 找到已存在的容器")
	}

	logFunc("[6/6] 启动OpenResty容器...")
	inspect, err := c.docker.ContainerInspect(c.ctx, c.containerID)
	if err != nil {
		return fmt.Errorf("Inspect容器失败: %w", err)
	}

	if !inspect.State.Running {
		if err := c.docker.ContainerStart(c.ctx, c.containerID, container.StartOptions{}); err != nil {
			return fmt.Errorf("启动OpenResty容器失败: %w", err)
		}
		logFunc("✓ 容器启动成功")
	} else {
		logFunc("✓ 容器已在运行")
	}

	logFunc("")
	logFunc("🎉 OpenResty 安装完成！")
	return nil
}

func (c *NginxClient) ensureImage() error {
	reader, err := c.docker.ImagePull(c.ctx, c.image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("拉取OpenResty镜像失败: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (c *NginxClient) ensureImageWithLogger(logFunc func(string)) error {
	reader, err := c.docker.ImagePull(c.ctx, c.image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("拉取OpenResty镜像失败: %w", err)
	}
	defer reader.Close()

	// 解析Docker pull的JSON进度输出
	decoder := json.NewDecoder(reader)
	layerStatus := make(map[string]string)

	for {
		var progress struct {
			Status         string `json:"status"`
			ID             string `json:"id"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
		}

		if err := decoder.Decode(&progress); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		// 只显示关键状态变化
		if progress.ID != "" {
			key := progress.ID
			if progress.Status != layerStatus[key] {
				layerStatus[key] = progress.Status
				if progress.Status == "Pulling fs layer" {
					logFunc(fmt.Sprintf("  下载镜像层: %s", progress.ID))
				} else if progress.Status == "Download complete" {
					logFunc(fmt.Sprintf("  ✓ 完成: %s", progress.ID))
				}
			}
		} else if progress.Status != "" && !strings.Contains(progress.Status, "Pulling") {
			logFunc(fmt.Sprintf("  %s", progress.Status))
		}
	}

	return nil
}

func (c *NginxClient) ensureDirectories() error {
	dirs := []string{
		c.hostPaths.Base,
		c.hostPaths.Conf,
		c.hostPaths.ConfD,
		c.hostPaths.Vhost,
		c.hostPaths.Meta,
		c.hostPaths.Logs,
		c.hostPaths.WWW,
		filepath.Join(c.hostPaths.WWW, "common", ".well-known", "acme-challenge"),
		c.hostPaths.SSL,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录%s失败: %w", dir, err)
		}
	}
	return nil
}

// GetVersion 返回容器内nginx版本
func (c *NginxClient) GetVersion() (string, error) {
	if err := c.ensureContainer(); err != nil {
		return "", err
	}

	_, stderr, err := c.runInContainer([]string{"nginx", "-v"})
	if err != nil {
		// 即便命令退出码非0，也可能输出版本，继续尝试解析
		if stderr == "" {
			return "", err
		}
	}

	out := strings.TrimSpace(stderr)
	if idx := strings.Index(out, "nginx/"); idx >= 0 {
		return strings.TrimSpace(out[idx+len("nginx/"):]), nil
	}
	return out, nil
}

func (c *NginxClient) ensureBaseConfig() error {
	files := map[string]string{
		filepath.Join(c.hostPaths.Conf, "nginx.conf"):     defaultNginxConf,
		filepath.Join(c.hostPaths.Conf, "mime.types"):     defaultMimeTypes,
		filepath.Join(c.hostPaths.Conf, "fastcgi_params"): defaultFastCGIParams,
		filepath.Join(c.hostPaths.ConfD, "http01.conf"):   defaultHTTP01Server,
	}

	for path, content := range files {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("写入默认配置%s失败: %w", path, err)
			}
		}
	}
	return nil
}

func (c *NginxClient) applySiteConfig(site *SiteConfig) (string, error) {
	if err := site.Validate(); err != nil {
		return "", err
	}

	hostRoot, err := c.hostPathFromContainer(site.RootDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(hostRoot, 0755); err != nil {
		return "", fmt.Errorf("创建站点根目录失败: %w", err)
	}

	siteCfg := &NginxConfig{
		FilePath: c.siteConfigPath(site.PrimaryDomain),
		Servers: []*ServerBlock{
			site.toServerBlock(c.containerPaths),
		},
	}

	rendered, err := siteCfg.Render()
	if err != nil {
		return "", err
	}

	configPath := siteCfg.FilePath
	metaPath := c.siteMetadataPath(site.PrimaryDomain)
	backup, hasBackup := c.loadBackup(configPath)
	metaBackup, hasMeta := c.loadBackup(metaPath)

	site.UpdatedAt = time.Now()
	if err := os.WriteFile(configPath, []byte(rendered), 0644); err != nil {
		return "", fmt.Errorf("写入站点配置失败: %w", err)
	}

	metaBytes, err := json.MarshalIndent(site, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化站点元数据失败: %w", err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
		return "", fmt.Errorf("写入站点元数据失败: %w", err)
	}

	if err := c.TestConfig(); err != nil {
		c.restoreBackup(configPath, backup, hasBackup)
		c.restoreBackup(metaPath, metaBackup, hasMeta)
		return "", err
	}

	if err := c.ReloadNginx(); err != nil {
		c.restoreBackup(configPath, backup, hasBackup)
		c.restoreBackup(metaPath, metaBackup, hasMeta)
		return "", err
	}

	return configPath, nil
}

// TestConfig 调用nginx -t
func (c *NginxClient) TestConfig() error {
	if err := c.ensureContainer(); err != nil {
		return err
	}

	_, stderr, err := c.runInContainer([]string{"nginx", "-t"})
	if err != nil {
		return fmt.Errorf("nginx配置校验失败: %s", stderr)
	}

	return nil
}

// ReloadNginx 执行nginx -s reload
func (c *NginxClient) ReloadNginx() error {
	if err := c.ensureContainer(); err != nil {
		return err
	}

	_, stderr, err := c.runInContainer([]string{"nginx", "-s", "reload"})
	if err != nil {
		return fmt.Errorf("重载Nginx失败: %s", stderr)
	}
	return nil
}

func (c *NginxClient) runInContainer(cmd []string) (string, string, error) {
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	target := c.containerID
	if target == "" {
		target = c.containerName
	}
	resp, err := c.docker.ContainerExecCreate(c.ctx, target, execConfig)
	if err != nil {
		return "", "", err
	}

	attach, err := c.docker.ContainerExecAttach(c.ctx, resp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", err
	}
	defer attach.Close()

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if _, err := stdcopy.StdCopy(stdout, stderr, attach.Reader); err != nil {
		return "", "", err
	}

	inspect, err := c.docker.ContainerExecInspect(c.ctx, resp.ID)
	if err != nil {
		return stdout.String(), stderr.String(), err
	}

	if inspect.ExitCode != 0 {
		return stdout.String(), stderr.String(), fmt.Errorf("命令失败，退出码%d", inspect.ExitCode)
	}

	return stdout.String(), stderr.String(), nil
}

func (c *NginxClient) normalizeSiteConfig(config SiteConfig) (*SiteConfig, error) {
	site := config
	primaryDomain, err := NormalizeDomain(site.PrimaryDomain, false)
	if err != nil {
		return nil, err
	}
	site.PrimaryDomain = primaryDomain
	if len(site.ExtraDomains) > 0 {
		extraDomains, err := NormalizeDomains(site.ExtraDomains, true)
		if err != nil {
			return nil, err
		}
		site.ExtraDomains = extraDomains
	}

	if site.RootDir == "" {
		site.RootDir = filepath.Join(c.containerPaths.WWW, "sites", sanitizeName(site.PrimaryDomain))
	}

	if !strings.HasPrefix(site.RootDir, "/") {
		site.RootDir = filepath.Join(c.containerPaths.WWW, site.RootDir)
	}

	if len(site.Index) == 0 {
		site.Index = []string{"index.php", "index.html", "index.htm"}
	}

	if site.HTTPChallengeDir == "" {
		site.HTTPChallengeDir = filepath.Join(c.containerPaths.WWW, "common")
	}

	if site.Proxy.Enable && site.Proxy.Pass == "" {
		return nil, fmt.Errorf("启用代理时必须提供proxy.pass地址")
	}

	return &site, nil
}

func (c *NginxClient) hostPathFromContainer(containerPath string) (string, error) {
	path := filepath.Clean(containerPath)
	switch {
	case strings.HasPrefix(path, c.containerPaths.WWW):
		return filepath.Join(c.hostPaths.WWW, strings.TrimPrefix(path, c.containerPaths.WWW)), nil
	case strings.HasPrefix(path, c.containerPaths.Conf):
		return filepath.Join(c.hostPaths.Conf, strings.TrimPrefix(path, c.containerPaths.Conf)), nil
	case strings.HasPrefix(path, c.containerPaths.Logs):
		return filepath.Join(c.hostPaths.Logs, strings.TrimPrefix(path, c.containerPaths.Logs)), nil
	case strings.HasPrefix(path, c.containerPaths.SSL):
		return filepath.Join(c.hostPaths.SSL, strings.TrimPrefix(path, c.containerPaths.SSL)), nil
	default:
		return "", fmt.Errorf("未知的容器路径: %s", containerPath)
	}
}

func (c *NginxClient) containerPathFromHost(hostPath string) string {
	path := filepath.Clean(hostPath)
	switch {
	case strings.HasPrefix(path, c.hostPaths.WWW):
		return filepath.Join(c.containerPaths.WWW, strings.TrimPrefix(path, c.hostPaths.WWW))
	case strings.HasPrefix(path, c.hostPaths.Conf):
		return filepath.Join(c.containerPaths.Conf, strings.TrimPrefix(path, c.hostPaths.Conf))
	case strings.HasPrefix(path, c.hostPaths.Logs):
		return filepath.Join(c.containerPaths.Logs, strings.TrimPrefix(path, c.hostPaths.Logs))
	case strings.HasPrefix(path, c.hostPaths.SSL):
		return filepath.Join(c.containerPaths.SSL, strings.TrimPrefix(path, c.hostPaths.SSL))
	default:
		return path
	}
}

func (c *NginxClient) siteConfigPath(domain string) string {
	return filepath.Join(c.hostPaths.Vhost, fmt.Sprintf("%s.conf", sanitizeName(domain)))
}

func (c *NginxClient) siteMetadataPath(domain string) string {
	return filepath.Join(c.hostPaths.Meta, fmt.Sprintf("%s.json", sanitizeName(domain)))
}

// GetRawConfig 获取网站的原始nginx配置文件内容
func (c *NginxClient) GetRawConfig(domain string) (string, error) {
	normalized, err := NormalizeDomain(domain, false)
	if err != nil {
		return "", err
	}
	domain = normalized

	// 获取配置文件路径
	configPath := c.siteConfigPath(domain)

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("站点 %s 的配置文件不存在", domain)
		}
		return "", fmt.Errorf("读取配置文件失败: %w", err)
	}

	return string(data), nil
}

const allowWidePermEnv = "NGINX_ALLOW_WIDE_PERMISSIONS"

// validateExistingFilePerms 校验已存在文件的权限：禁止组/其他可写(0022)；可用环境变量放宽
func (c *NginxClient) validateExistingFilePerms(path string) error {
	// 使用Lstat而非Stat，显式拒绝symlink
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("获取文件信息失败 %s: %w", path, err)
	}

	// 显式拒绝symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("配置路径不安全: %s 是符号链接", path)
	}

	// 仅对常规文件做权限判断；拒绝目录/设备等特殊文件
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("配置路径不是常规文件: %s (mode=%s)", path, fi.Mode().String())
	}

	perm := fi.Mode().Perm()
	if perm&0022 == 0 {
		return nil
	}

	if os.Getenv(allowWidePermEnv) == "1" {
		if c.log != nil {
			c.log.Warn("检测到过宽权限，仍继续（%s=1）：path=%s perm=%04o", allowWidePermEnv, path, uint32(perm))
		}
		return nil
	}

	return fmt.Errorf("不安全的文件权限：%s perm=%04o（group/other 可写）；请修复权限或设置 %s=1 放宽", path, uint32(perm), allowWidePermEnv)
}

// fsyncDir 对目录执行fsync，失败时返回错误
func (c *NginxClient) fsyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("fsync目录失败（打开目录失败）：dir=%s err=%w", dir, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && c.log != nil {
			c.log.Warn("关闭目录fd失败：dir=%s err=%v", dir, closeErr)
		}
	}()

	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync目录失败：dir=%s err=%w", dir, err)
	}
	return nil
}

// renameAndFsync 执行原子重命名并fsync目录
func (c *NginxClient) renameAndFsync(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	// Fsync目标目录（新目录项创建处）
	if err := c.fsyncDir(filepath.Dir(newPath)); err != nil {
		return err
	}
	// 如果源和目标在不同目录，也fsync源目录（旧目录项删除处）
	oldDir := filepath.Dir(oldPath)
	newDir := filepath.Dir(newPath)
	if oldDir != newDir {
		if err := c.fsyncDir(oldDir); err != nil {
			return err
		}
	}
	return nil
}

// SaveRawConfig 保存网站的nginx配置文件(包含备份、校验和reload)
// 采用"替换→测试→回滚"模式，确保测试阶段使用的是新配置
func (c *NginxClient) SaveRawConfig(domain, content string) error {
	if content == "" {
		return fmt.Errorf("配置内容不能为空")
	}
	if len(content) > 1<<20 {
		return fmt.Errorf("配置内容超过1 MiB限制")
	}

	normalized, err := NormalizeDomain(domain, false)
	if err != nil {
		return err
	}
	domain = normalized

	configPath := c.siteConfigPath(domain)

	// 并发保护：使用文件锁防止并发编辑冲突
	lockPath := configPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("创建锁文件失败: %w", err)
	}
	defer lockFile.Close()

	// 使用 EINTR 重试机制获取文件锁
	for {
		err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX)
		if err == nil {
			break
		}
		if err != unix.EINTR {
			return fmt.Errorf("获取文件锁失败: %w", err)
		}
		// 如果是 EINTR，重试
	}
	defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)

	// symlink安全检查：防止通过符号链接写入任意位置
	if fi, err := os.Lstat(configPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("配置路径不安全: %s 是符号链接", configPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查配置文件失败: %w", err)
	}

	// 获取原文件的权限信息，用于新文件继承
	originalPerm := os.FileMode(0644)
	originalUID, originalGID := -1, -1
	originalExists := false

	var statBuf unix.Stat_t
	if err := unix.Stat(configPath, &statBuf); err == nil {
		originalExists = true
		originalPerm = os.FileMode(statBuf.Mode).Perm()
		originalUID = int(statBuf.Uid)
		originalGID = int(statBuf.Gid)
		// 【修改点2】权限检查：禁止过宽权限（group/other可写）
		if err := c.validateExistingFilePerms(configPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取配置文件信息失败: %w", err)
	}

	dir := filepath.Dir(configPath)
	basename := filepath.Base(configPath)

	// 创建临时文件（在同一目录下，确保Rename是原子操作）
	tempFile, err := os.CreateTemp(dir, basename+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置文件失败: %w", err)
	}
	tempPath := tempFile.Name()
	tempClosed := false

	// 确保临时文件在所有失败路径都被清理
	defer func() {
		if !tempClosed {
			tempFile.Close()
		}
		os.Remove(tempPath)
	}()

	// 写入新内容到临时文件
	if _, err := io.WriteString(tempFile, content); err != nil {
		return fmt.Errorf("写入临时配置文件失败: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("同步临时配置文件失败: %w", err)
	}

	// 设置临时文件的权限和属主（在Close之前，使用fd级别操作）
	if err := unix.Fchmod(int(tempFile.Fd()), uint32(originalPerm)); err != nil {
		return fmt.Errorf("设置临时配置文件权限失败: %w", err)
	}
	if originalUID >= 0 && originalGID >= 0 {
		if err := unix.Fchown(int(tempFile.Fd()), originalUID, originalGID); err != nil {
			return fmt.Errorf("设置临时配置文件属主失败: %w", err)
		}
	}

	// 元数据变更后再次Sync，确保权限落盘
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("同步临时配置文件元数据失败: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("关闭临时配置文件失败: %w", err)
	}
	tempClosed = true

	// 创建备份（用于回滚）
	backupPath := configPath + ".backup"
	backupCreated := false

	if originalExists {
		// 检查备份路径是否安全
		if fi, err := os.Lstat(backupPath); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("备份路径不安全: %s 是符号链接", backupPath)
			}
			// 【修改点3】如果备份文件已存在，检查其权限
			if err := c.validateExistingFilePerms(backupPath); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("检查备份文件失败: %w", err)
		}

		originalData, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("读取原配置失败: %w", err)
		}

		// 备份也使用临时文件+原子替换
		backupTempFile, err := os.CreateTemp(dir, basename+".*.backup.tmp")
		if err != nil {
			return fmt.Errorf("创建备份临时文件失败: %w", err)
		}
		backupTempPath := backupTempFile.Name()
		backupTempClosed := false

		defer func() {
			if !backupTempClosed {
				backupTempFile.Close()
			}
			os.Remove(backupTempPath)
		}()

		if _, err := backupTempFile.Write(originalData); err != nil {
			return fmt.Errorf("写入备份临时文件失败: %w", err)
		}
		if err := backupTempFile.Sync(); err != nil {
			return fmt.Errorf("同步备份临时文件失败: %w", err)
		}

		// 设置备份文件的权限和属主（在Close之前，使用fd级别操作）
		if err := unix.Fchmod(int(backupTempFile.Fd()), uint32(originalPerm)); err != nil {
			return fmt.Errorf("设置备份文件权限失败: %w", err)
		}
		if originalUID >= 0 && originalGID >= 0 {
			if err := unix.Fchown(int(backupTempFile.Fd()), originalUID, originalGID); err != nil {
				return fmt.Errorf("设置备份文件属主失败: %w", err)
			}
		}

		// 元数据变更后再次Sync，确保权限落盘
		if err := backupTempFile.Sync(); err != nil {
			return fmt.Errorf("同步备份文件元数据失败: %w", err)
		}

		if err := backupTempFile.Close(); err != nil {
			return fmt.Errorf("关闭备份临时文件失败: %w", err)
		}
		backupTempClosed = true

		// 【修改点6】原子重命名备份文件并fsync目录
		if err := c.renameAndFsync(backupTempPath, backupPath); err != nil {
			return fmt.Errorf("创建备份文件失败: %w", err)
		}
		backupCreated = true
	}

	// 回滚函数：从备份恢复配置文件
	restoreFromBackup := func() error {
		if !backupCreated {
			return fmt.Errorf("无可用备份")
		}

		backupData, err := os.ReadFile(backupPath)
		if err != nil {
			return fmt.Errorf("读取备份失败: %w", err)
		}

		restoreTempFile, err := os.CreateTemp(dir, basename+".*.restore.tmp")
		if err != nil {
			return fmt.Errorf("创建回滚临时文件失败: %w", err)
		}
		restoreTempPath := restoreTempFile.Name()
		restoreTempClosed := false

		defer func() {
			if !restoreTempClosed {
				restoreTempFile.Close()
			}
			os.Remove(restoreTempPath)
		}()

		if _, err := restoreTempFile.Write(backupData); err != nil {
			return fmt.Errorf("写入回滚临时文件失败: %w", err)
		}
		if err := restoreTempFile.Sync(); err != nil {
			return fmt.Errorf("同步回滚临时文件失败: %w", err)
		}

		// 设置回滚文件的权限和属主（在Close之前，使用fd级别操作）
		if err := unix.Fchmod(int(restoreTempFile.Fd()), uint32(originalPerm)); err != nil {
			return fmt.Errorf("设置回滚文件权限失败: %w", err)
		}
		if originalUID >= 0 && originalGID >= 0 {
			if err := unix.Fchown(int(restoreTempFile.Fd()), originalUID, originalGID); err != nil {
				return fmt.Errorf("设置回滚文件属主失败: %w", err)
			}
		}

		// 元数据变更后再次Sync，确保权限落盘
		if err := restoreTempFile.Sync(); err != nil {
			return fmt.Errorf("同步回滚文件元数据失败: %w", err)
		}

		if err := restoreTempFile.Close(); err != nil {
			return fmt.Errorf("关闭回滚临时文件失败: %w", err)
		}
		restoreTempClosed = true

		// 【修改点7】原子重命名回滚文件并fsync目录
		if err := c.renameAndFsync(restoreTempPath, configPath); err != nil {
			return fmt.Errorf("回滚替换配置文件失败: %w", err)
		}
		return nil
	}

	// 【关键】先替换再测试，确保nginx -t测试的是新配置
	// 【修改点5】使用renameAndFsync提高崩溃一致性
	if err := c.renameAndFsync(tempPath, configPath); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}

	// 测试nginx配置（此时configPath已经是新配置）
	if err := c.TestConfig(); err != nil {
		// 测试失败，立即回滚
		if backupCreated {
			if rbErr := restoreFromBackup(); rbErr != nil {
				return fmt.Errorf("nginx配置测试失败: %v (回滚失败: %w)", err, rbErr)
			}
		} else {
			// 如果原本不存在配置文件，删除新创建的
			os.Remove(configPath)
		}
		return fmt.Errorf("nginx配置测试失败: %w", err)
	}

	// Reload nginx（失败也必须回滚，保持磁盘与运行态一致）
	if err := c.ReloadNginx(); err != nil {
		if backupCreated {
			if rbErr := restoreFromBackup(); rbErr != nil {
				return fmt.Errorf("重载nginx失败: %v (回滚失败: %w)", err, rbErr)
			}
			// 回滚后再次尝试reload（best-effort）
			c.ReloadNginx()
		} else {
			os.Remove(configPath)
		}
		return fmt.Errorf("重载nginx失败: %w", err)
	}

	// 成功后保留备份文件（用于审计和手动恢复）
	return nil
}

// ResolveHostPath 将容器路径或宿主路径规范化为宿主机路径
func (c *NginxClient) ResolveHostPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	clean := filepath.Clean(path)
	resolved := clean
	var err error

	if !strings.HasPrefix(clean, c.hostPaths.Base) {
		resolved, err = c.hostPathFromContainer(clean)
		if err != nil {
			return "", err
		}
	}

	if _, statErr := os.Stat(resolved); statErr == nil {
		return resolved, nil
	}

	confSSL := filepath.Join(c.hostPaths.Base, "conf", "ssl")
	if strings.HasPrefix(resolved, confSSL) {
		trimmed := strings.TrimPrefix(resolved, confSSL)
		alt := filepath.Join(c.hostPaths.SSL, trimmed)
		if _, altErr := os.Stat(alt); altErr == nil {
			return filepath.Clean(alt), nil
		}
		return filepath.Clean(alt), nil
	}

	return resolved, nil
}

func (site *SiteConfig) toServerBlock(paths ContainerPaths) *ServerBlock {
	listen := []string{"80"}
	var sslBlock *SSLConfig
	if site.EnableHTTPS && site.SSL.Certificate != "" && site.SSL.CertificateKey != "" {
		listen = append(listen, "443 ssl http2")
		sslBlock = &SSLConfig{
			Enabled:        true,
			Certificate:    site.SSL.Certificate,
			CertificateKey: site.SSL.CertificateKey,
			SessionCache:   "shared:SSL:50m",
			Protocols:      []string{"TLSv1.2", "TLSv1.3"},
			Ciphers: []string{
				"HIGH:!aNULL:!MD5",
			},
		}
	}

	var phpBlock *PHPConfig
	if site.PHPVersion != "" {
		socket := fmt.Sprintf("unix:/run/php/php%s-fpm.sock", site.PHPVersion)
		if strings.Contains(site.PHPVersion, ":") {
			socket = site.PHPVersion
		}
		phpBlock = &PHPConfig{
			FastCGIPass: socket,
			Index:       []string{"index.php"},
		}
	}

	var proxyBlock *ProxyConfig
	if site.Proxy.Enable {
		proxyBlock = &ProxyConfig{
			Enable:       true,
			Pass:         site.Proxy.Pass,
			Websocket:    site.Proxy.Websocket,
			Headers:      site.Proxy.Headers,
			PreserveHost: site.Proxy.PreserveHost,
		}
	}

	// 文件上传大小限制: 校验并规范化格式
	clientMaxBodySize := strings.TrimSpace(site.ClientMaxBodySize)
	if clientMaxBodySize != "" {
		// 基础格式校验，防止注入和非法值
		// 合法格式: "0"(无限制) 或 "数字+可选单位(k/m/g)"
		// 例如: "0", "10m", "100m", "1g"
		clientMaxBodySize = strings.ToLower(clientMaxBodySize)
		valid := false

		if clientMaxBodySize == "0" {
			// 特殊值: 0表示不限制
			valid = true
		} else if len(clientMaxBodySize) > 0 && clientMaxBodySize[0] >= '1' && clientMaxBodySize[0] <= '9' {
			// 数字开头，检查后续格式
			i := 1
			for i < len(clientMaxBodySize) && clientMaxBodySize[i] >= '0' && clientMaxBodySize[i] <= '9' {
				i++
			}
			if i == len(clientMaxBodySize) {
				// 纯数字，合法
				valid = true
			} else if i == len(clientMaxBodySize)-1 {
				// 最后一位是单位
				unit := clientMaxBodySize[i]
				if unit == 'k' || unit == 'm' || unit == 'g' {
					valid = true
				}
			}
		}

		// 非法值时清空，不输出该指令
		if !valid {
			clientMaxBodySize = ""
		}
	}

	return &ServerBlock{
		Listen:            listen,
		ServerNames:       site.AllDomains(),
		Root:              site.RootDir,
		Index:             site.Index,
		AccessLog:         filepath.Join(paths.Logs, fmt.Sprintf("%s.access.log", sanitizeName(site.PrimaryDomain))),
		ErrorLog:          filepath.Join(paths.Logs, fmt.Sprintf("%s.error.log", sanitizeName(site.PrimaryDomain))),
		ClientMaxBodySize: clientMaxBodySize,
		Proxy:             proxyBlock,
		PHP:               phpBlock,
		SSL:               sslBlock,
		ForceSSL:          site.ForceSSL && sslBlock != nil,
		ChallengeRoot:     site.HTTPChallengeDir,
	}
}

func (c *NginxClient) loadBackup(path string) ([]byte, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return content, true
}

func (c *NginxClient) restoreBackup(path string, data []byte, ok bool) {
	if !ok {
		_ = os.Remove(path)
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

func determineSiteType(site SiteConfig) string {
	if site.Proxy.Enable {
		return "proxy"
	}
	if strings.TrimSpace(site.PHPVersion) != "" {
		return "php"
	}
	return "static"
}

func (c *NginxClient) buildCertificateInfo(site *SiteConfig) *CertificateInfo {
	if site == nil || site.SSL.Certificate == "" {
		return nil
	}

	hostCertPath, err := c.hostPathFromContainer(site.SSL.Certificate)
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(hostCertPath)
	if err != nil {
		return nil
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}

	days := int(time.Until(cert.NotAfter).Hours() / 24)
	if days < 0 {
		days = 0
	}

	return &CertificateInfo{
		Valid:    time.Now().Before(cert.NotAfter),
		Expiry:   cert.NotAfter.Format(time.RFC3339),
		Issuer:   cert.Issuer.CommonName,
		DaysLeft: days,
		Path:     site.SSL.Certificate,
	}
}

func sanitizeName(domain string) string {
	d := strings.ToLower(domain)
	replacements := []struct {
		old string
		new string
	}{
		{"*", "wildcard"},
		{"/", "_"},
		{"\\", "_"},
		{":", "_"},
		{" ", "_"},
	}
	for _, r := range replacements {
		d = strings.ReplaceAll(d, r.old, r.new)
	}
	return d
}

const defaultNginxConf = `worker_processes  auto;
error_log  logs/error.log warn;
pid        logs/nginx.pid;

events {
	worker_connections  1024;
}

http {
	include       mime.types;
	default_type  application/octet-stream;

	log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
					  '$status $body_bytes_sent "$http_referer" '
					  '"$http_user_agent" "$http_x_forwarded_for"';

	access_log  logs/access.log  main;

	sendfile        on;
	keepalive_timeout  65;

	gzip on;
	gzip_types text/plain text/css application/javascript application/json application/xml;

	include conf.d/*.conf;
	include vhost/*.conf;
}
`

const defaultMimeTypes = `types {
	text/html                             html htm shtml;
	text/css                              css;
	text/xml                              xml;
	image/gif                             gif;
	image/jpeg                            jpeg jpg;
	application/javascript                js;
	application/atom+xml                  atom;
	application/rss+xml                   rss;

	text/mathml                           mml;
	text/plain                            txt;
	text/vnd.sun.j2me.app-descriptor      jad;
	text/vnd.wap.wml                      wml;
	text/x-component                      htc;

	image/png                             png;
	image/tiff                            tif tiff;
	image/vnd.wap.wbmp                    wbmp;
	image/x-icon                          ico;
	image/x-jng                           jng;
	image/x-ms-bmp                        bmp;
	image/svg+xml                         svg svgz;

	application/font-woff                 woff;
	application/java-archive              jar war ear;
	application/json                      json;
	application/mac-binhex40              hqx;
	application/msword                    doc;
	application/pdf                       pdf;
	application/postscript                ps eps ai;
	application/rtf                       rtf;
	application/vnd.apple.mpegurl         m3u8;
	application/vnd.ms-excel              xls;
	application/vnd.ms-fontobject         eot;
	application/vnd.ms-powerpoint         ppt;
	application/vnd.wap.wmlc              wmlc;
	application/vnd.google-earth.kml+xml  kml;
	application/vnd.google-earth.kmz      kmz;
	application/x-7z-compressed           7z;
	application/x-cocoa                   cco;
	application/x-java-archive-diff       jardiff;
	application/x-java-jnlp-file          jnlp;
	application/x-makeself                run;
	application/x-perl                    pl pm;
	application/x-pilot                   prc pdb;
	application/x-rar-compressed          rar;
	application/x-redhat-package-manager  rpm;
	application/x-sea                     sea;
	application/x-shockwave-flash         swf;
	application/x-stuffit                 sit;
	application/x-tcl                     tcl tk;
	application/x-x509-ca-cert            der pem crt;
	application/x-xpinstall               xpi;
	application/xhtml+xml                 xhtml;
	application/xspf+xml                  xspf;
	application/zip                       zip;

	audio/midi                            mid midi kar;
	audio/mpeg                            mp3;
	audio/ogg                             ogg;
	audio/x-m4a                           m4a;
	audio/x-realaudio                     ra;

	video/3gpp                            3gpp 3gp;
	video/mp2t                            ts;
	video/mp4                             mp4;
	video/mpeg                            mpeg mpg;
	video/quicktime                       mov;
	video/webm                            webm;
	video/x-flv                           flv;
	video/x-m4v                           m4v;
	video/x-mng                           mng;
	video/x-ms-asf                        asx asf;
	video/x-ms-wmv                        wmv;
	video/x-msvideo                       avi;
}
`

const defaultFastCGIParams = `fastcgi_param  QUERY_STRING        $query_string;
fastcgi_param  REQUEST_METHOD      $request_method;
fastcgi_param  CONTENT_TYPE        $content_type;
fastcgi_param  CONTENT_LENGTH      $content_length;

fastcgi_param  SCRIPT_FILENAME     $document_root$fastcgi_script_name;
fastcgi_param  SCRIPT_NAME         $fastcgi_script_name;
fastcgi_param  REQUEST_URI         $request_uri;
fastcgi_param  DOCUMENT_URI        $document_uri;
fastcgi_param  DOCUMENT_ROOT       $document_root;
fastcgi_param  SERVER_PROTOCOL     $server_protocol;

fastcgi_param  GATEWAY_INTERFACE   CGI/1.1;
fastcgi_param  SERVER_SOFTWARE     nginx/$nginx_version;

fastcgi_param  REMOTE_ADDR         $remote_addr;
fastcgi_param  REMOTE_PORT         $remote_port;
fastcgi_param  SERVER_ADDR         $server_addr;
fastcgi_param  SERVER_PORT         $server_port;
fastcgi_param  SERVER_NAME         $server_name;
`

const defaultHTTP01Server = `server {
	listen 80 default_server;
	server_name _;

	location ^~ /.well-known/acme-challenge/ {
		default_type "text/plain";
		root /www/common;
		try_files $uri =404;
	}
}
`
