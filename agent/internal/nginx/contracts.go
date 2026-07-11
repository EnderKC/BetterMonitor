//go:build !monitor_only

package nginx

import (
	"fmt"
	"path/filepath"
	"strings"
)

func NormalizeDomain(domain string, allowWildcard bool) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || len(domain) > 253 {
		return "", fmt.Errorf("无效的域名")
	}
	wildcard := false
	if strings.HasPrefix(domain, "*.") {
		if !allowWildcard {
			return "", fmt.Errorf("不允许通配符域名")
		}
		wildcard = true
		domain = strings.TrimPrefix(domain, "*.")
	}
	if domain == "" || strings.Contains(domain, "*") || strings.ContainsAny(domain, "/\\") {
		return "", fmt.Errorf("无效的域名")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("域名至少需要两个标签")
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("无效的域名标签")
		}
		for _, char := range label {
			if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
				continue
			}
			return "", fmt.Errorf("域名包含非法字符")
		}
	}
	if wildcard {
		domain = "*." + domain
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("域名过长")
	}
	return domain, nil
}

func NormalizeDomains(domains []string, allowWildcard bool) ([]string, error) {
	if len(domains) == 0 || len(domains) > 100 {
		return nil, fmt.Errorf("域名数量无效")
	}
	result := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		normalized, err := NormalizeDomain(domain, allowWildcard)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func NormalizeProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "http01":
		return "http01", nil
	case "alidns", "aliyun":
		return "alidns", nil
	case "cloudflare", "cf":
		return "cloudflare", nil
	default:
		return "", fmt.Errorf("不支持的证书提供商")
	}
}

func ValidateDNSConfig(config map[string]string) error {
	if len(config) > 32 {
		return fmt.Errorf("DNS配置项过多")
	}
	for key, value := range config {
		if key == "" || len(key) > 64 || value == "" || len(value) > 4096 {
			return fmt.Errorf("DNS配置字段无效")
		}
		for _, char := range key {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
				continue
			}
			return fmt.Errorf("DNS配置键包含非法字符")
		}
		for _, char := range value {
			if char < 32 || char == 127 {
				return fmt.Errorf("DNS配置值包含控制字符")
			}
		}
	}
	return nil
}

func NormalizeCertificateRequest(request *CertificateRequest) error {
	if request == nil {
		return fmt.Errorf("证书请求为空")
	}
	domains, err := NormalizeDomains(request.Domains, true)
	if err != nil {
		return err
	}
	provider, err := NormalizeProvider(request.Provider)
	if err != nil {
		return err
	}
	request.Domains = domains
	request.Provider = provider
	request.Email = strings.TrimSpace(request.Email)
	if len(request.Email) > 254 || strings.ContainsAny(request.Email, "\r\n\x00") ||
		(request.Email != "" && (!strings.Contains(request.Email, "@") || strings.Contains(request.Email, " "))) {
		return fmt.Errorf("邮箱格式无效")
	}
	request.Webroot = strings.TrimSpace(request.Webroot)
	if provider == "http01" {
		if request.Webroot == "" || len(request.Webroot) > 4096 || !filepath.IsAbs(request.Webroot) {
			return fmt.Errorf("HTTP-01验证目录无效")
		}
		for _, char := range request.Webroot {
			if char < 32 || char == 127 {
				return fmt.Errorf("HTTP-01验证目录无效")
			}
		}
	} else {
		if len(request.DNSConfig) == 0 {
			return fmt.Errorf("DNS验证需要配置")
		}
		if err := ValidateDNSConfig(request.DNSConfig); err != nil {
			return err
		}
	}
	return nil
}
