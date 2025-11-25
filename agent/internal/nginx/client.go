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
)

// SiteConfig 声明式站点配置
type SiteConfig struct {
	PrimaryDomain    string            `json:"primary_domain"`
	ExtraDomains     []string          `json:"extra_domains"`
	RootDir          string            `json:"root_dir"`
	Index            []string          `json:"index"`
	PHPVersion       string            `json:"php_version"`
	Proxy            ProxyConfig       `json:"proxy"`
	EnableHTTPS      bool              `json:"enable_https"`
	ForceSSL         bool              `json:"force_ssl"`
	SSL              SSLPaths          `json:"ssl"`
	HTTPChallengeDir string            `json:"http_challenge_dir"`
	Labels           map[string]string `json:"labels,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at"`
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

func (c *NginxClient) ensureImage() error {
	reader, err := c.docker.ImagePull(c.ctx, c.image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("拉取OpenResty镜像失败: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
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
	site.PrimaryDomain = strings.TrimSpace(strings.ToLower(site.PrimaryDomain))

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

	return &ServerBlock{
		Listen:        listen,
		ServerNames:   site.AllDomains(),
		Root:          site.RootDir,
		Index:         site.Index,
		AccessLog:     filepath.Join(paths.Logs, fmt.Sprintf("%s.access.log", sanitizeName(site.PrimaryDomain))),
		ErrorLog:      filepath.Join(paths.Logs, fmt.Sprintf("%s.error.log", sanitizeName(site.PrimaryDomain))),
		Proxy:         proxyBlock,
		PHP:           phpBlock,
		SSL:           sslBlock,
		ForceSSL:      site.ForceSSL && sslBlock != nil,
		ChallengeRoot: site.HTTPChallengeDir,
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
