//go:build windows && !monitor_only

package nginx

import (
	"errors"
	"time"

	"github.com/user/server-ops-agent/pkg/logger"
)

var (
	// ErrNotSupported Windows 不支持 Nginx 操作
	ErrNotSupported = errors.New("Nginx operations are not supported on Windows")
)

// NginxClient Windows stub - 不支持实际操作
type NginxClient struct {
	log *logger.Logger
}

// ClientOption 定制NginxClient
type ClientOption func(*NginxClient)

// WithContainerName 设置容器名称（Windows stub）
func WithContainerName(name string) ClientOption {
	return func(c *NginxClient) {}
}

// WithHostBaseDir 调整宿主机根目录（Windows stub）
func WithHostBaseDir(dir string) ClientOption {
	return func(c *NginxClient) {}
}

// WithImage 指定OpenResty镜像（Windows stub）
func WithImage(image string) ClientOption {
	return func(c *NginxClient) {}
}

// NewNginxClient 在 Windows 上返回不支持错误
func NewNginxClient(log *logger.Logger, opts ...ClientOption) (*NginxClient, error) {
	return nil, ErrNotSupported
}

// 以下是 NginxClient 的方法 stubs，所有方法都返回不支持错误

func (c *NginxClient) Close() error {
	return ErrNotSupported
}

func (c *NginxClient) CreateWebsite(config SiteConfig) (string, error) {
	return "", ErrNotSupported
}

func (c *NginxClient) GetRawConfig(domain string) (string, error) {
	return "", ErrNotSupported
}

func (c *NginxClient) GetRuntimeState() (bool, bool, error) {
	return false, false, ErrNotSupported
}

func (c *NginxClient) GetSiteDetail(domain string) (*SiteSummary, error) {
	return nil, ErrNotSupported
}

func (c *NginxClient) GetVersion() (string, error) {
	return "", ErrNotSupported
}

func (c *NginxClient) IssueCertificate(req CertificateRequest) (*CertificateResult, error) {
	return nil, ErrNotSupported
}

func (c *NginxClient) ListSites() ([]SiteSummary, error) {
	return nil, ErrNotSupported
}

func (c *NginxClient) ReloadNginx() error {
	return ErrNotSupported
}

func (c *NginxClient) ResolveHostPath(containerPath string) (string, error) {
	return "", ErrNotSupported
}

func (c *NginxClient) SaveRawConfig(domain, content string) error {
	return ErrNotSupported
}

func (c *NginxClient) TestConfig() error {
	return ErrNotSupported
}

func (c *NginxClient) InstallOpenResty() error {
	return ErrNotSupported
}

func (c *NginxClient) InstallOpenRestyWithLogger(logFunc func(string)) error {
	return ErrNotSupported
}

// HostPaths Windows stub
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

// ContainerPaths Windows stub
type ContainerPaths struct {
	Conf string
	Logs string
	WWW  string
	SSL  string
}

// SSLPaths Windows stub (可能在其他文件已定义，这里为了安全重新声明)
type SSLPaths struct {
	Certificate    string `json:"certificate"`
	CertificateKey string `json:"certificate_key"`
}

// CertificateInfo Windows stub
type CertificateInfo struct {
	Valid    bool   `json:"valid"`
	Expiry   string `json:"expiry"`
	Issuer   string `json:"issuer"`
	DaysLeft int    `json:"days_left"`
	Path     string `json:"path"`
}

// SiteConfig Windows stub
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
	ClientMaxBodySize string            `json:"client_max_body_size"`
	Labels            map[string]string `json:"labels,omitempty"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// SiteSummary Windows stub
type SiteSummary struct {
	SiteConfig  SiteConfig       `json:"site"`
	Type        string           `json:"type"`
	Certificate *CertificateInfo `json:"certificate,omitempty"`
	HostRootDir string           `json:"host_root_dir,omitempty"`
}
