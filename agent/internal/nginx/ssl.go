//go:build !monitor_only

package nginx

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

// CertificateRequest ACME申请参数
type CertificateRequest struct {
	Domains    []string          `json:"domains"`
	Email      string            `json:"email"`
	Webroot    string            `json:"webroot"`
	UseStaging bool              `json:"use_staging"`
	Provider   string            `json:"provider"`
	DNSConfig  map[string]string `json:"dns_config"`
}

// PrimaryDomain 获取主域名
func (r CertificateRequest) PrimaryDomain() string {
	if len(r.Domains) == 0 {
		return ""
	}
	return r.Domains[0]
}

// CertificateResult ACME申请结果
type CertificateResult struct {
	Domains         []string  `json:"domains"`
	Expiry          time.Time `json:"expiry"`
	CertificatePEM  []byte    `json:"-"`
	PrivateKeyPEM   []byte    `json:"-"`
	CertificatePath string    `json:"certificate_path"`
	KeyPath         string    `json:"key_path"`
}

type legoUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *legoUser) GetEmail() string {
	return u.email
}

func (u *legoUser) GetRegistration() *registration.Resource {
	return u.registration
}

func (u *legoUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// ObtainCertHTTP 兼容需求的简单HTTP-01入口
func ObtainCertHTTP(domain string, webroot string) (*CertificateResult, error) {
	req := CertificateRequest{
		Domains: []string{domain},
		Webroot: webroot,
	}
	return ObtainCertificate(req)
}

// ObtainCertificate 通过Lego执行HTTP-01流程
func ObtainCertificate(req CertificateRequest) (*CertificateResult, error) {
	if len(req.Domains) == 0 {
		return nil, fmt.Errorf("至少需要一个域名")
	}

	useHTTP := strings.EqualFold(req.Provider, "") || strings.EqualFold(req.Provider, "http01")
	if useHTTP && req.Webroot == "" {
		return nil, fmt.Errorf("必须提供HTTP-01验证目录")
	}

	email := req.Email
	if email == "" {
		email = fmt.Sprintf("admin@%s", strings.TrimPrefix(req.Domains[0], "*."))
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("生成账户私钥失败: %w", err)
	}

	user := &legoUser{
		email: email,
		key:   privateKey,
	}

	config := lego.NewConfig(user)
	if req.UseStaging {
		config.CADirURL = lego.LEDirectoryStaging
	} else {
		config.CADirURL = lego.LEDirectoryProduction
	}

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("创建ACME客户端失败: %w", err)
	}

	if useHTTP {
		provider := &webrootProvider{root: req.Webroot}
		if err := client.Challenge.SetHTTP01Provider(provider); err != nil {
			return nil, fmt.Errorf("设置HTTP-01提供器失败: %w", err)
		}
	} else {
		dnsProvider, err := buildDNSProvider(req.Provider, req.DNSConfig)
		if err != nil {
			return nil, err
		}
		if err := client.Challenge.SetDNS01Provider(dnsProvider); err != nil {
			return nil, fmt.Errorf("设置DNS-01提供器失败: %w", err)
		}
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already registered") {
		return nil, fmt.Errorf("注册ACME账号失败: %w", err)
	}
	if err == nil {
		user.registration = reg
	}

	request := certificate.ObtainRequest{
		Domains: req.Domains,
		Bundle:  true,
	}

	resp, err := client.Certificate.Obtain(request)
	if err != nil {
		return nil, wrapACMEProviderError(fmt.Errorf("申请证书失败: %w", err), req.Provider)
	}

	var expiry time.Time
	if parsed, parseErr := certcrypto.ParsePEMBundle(resp.Certificate); parseErr == nil && len(parsed) > 0 {
		expiry = parsed[0].NotAfter
	}

	return &CertificateResult{
		Domains:        req.Domains,
		Expiry:         expiry,
		CertificatePEM: resp.Certificate,
		PrivateKeyPEM:  resp.PrivateKey,
	}, nil
}

type webrootProvider struct {
	root string
}

func (p *webrootProvider) Present(domain, token, keyAuth string) error {
	if p.root == "" {
		return fmt.Errorf("webroot未设置")
	}
	challengeDir := filepath.Join(p.root, ".well-known", "acme-challenge")
	if err := os.MkdirAll(challengeDir, 0755); err != nil {
		return fmt.Errorf("创建挑战目录失败: %w", err)
	}

	target := filepath.Join(challengeDir, token)
	return os.WriteFile(target, []byte(keyAuth), 0644)
}

func (p *webrootProvider) CleanUp(domain, token, keyAuth string) error {
	if p.root == "" {
		return nil
	}
	target := filepath.Join(p.root, ".well-known", "acme-challenge", token)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func buildDNSProvider(name string, config map[string]string) (challenge.Provider, error) {
	switch strings.ToLower(name) {
	case "alidns", "aliyun":
		apiKey := config["access_key_id"]
		apiSecret := config["access_key_secret"]
		if apiKey == "" || apiSecret == "" {
			return nil, fmt.Errorf("阿里云DNS需要提供access_key_id和access_key_secret")
		}
		cfg := alidns.NewDefaultConfig()
		cfg.APIKey = apiKey
		cfg.SecretKey = apiSecret
		return alidns.NewDNSProviderConfig(cfg)
	case "cloudflare", "cf":
		cfg := cloudflare.NewDefaultConfig()
		token := config["api_token"]
		if token != "" {
			cfg.AuthToken = token
		} else {
			cfg.AuthEmail = config["api_email"]
			cfg.AuthKey = config["api_key"]
		}
		if zoneToken := config["zone_token"]; zoneToken != "" {
			cfg.ZoneToken = zoneToken
		}
		if cfg.AuthToken == "" && (cfg.AuthEmail == "" || cfg.AuthKey == "") {
			return nil, fmt.Errorf("Cloudflare需要提供api_token或api_email+api_key")
		}
		return cloudflare.NewDNSProviderConfig(cfg)
	default:
		return nil, fmt.Errorf("暂不支持的DNS提供商: %s", name)
	}
}

func wrapACMEProviderError(err error, provider string) error {
	if err == nil {
		return nil
	}

	if strings.EqualFold(provider, "cloudflare") || strings.EqualFold(provider, "cf") {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "failed to find zone") || strings.Contains(msg, "zone could not be found") {
			return fmt.Errorf("Cloudflare API 无法定位该域名，请确认令牌拥有 Zone:Read 和 DNS:Edit 权限，或在DNS账号中填写 Zone Token。%w", err)
		}
	}
	return err
}
