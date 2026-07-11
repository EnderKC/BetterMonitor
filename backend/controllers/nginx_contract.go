package controllers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var nginxManagedIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func ValidateNginxManagedID(id string) error {
	if !nginxManagedIDPattern.MatchString(id) {
		return fmt.Errorf("无效的托管文件ID")
	}
	return nil
}

func ValidateNginxConfigName(name string) error {
	if name == "" || name != strings.TrimSpace(name) || len(name) > 128 || name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("无效的Nginx配置名称")
	}
	for index, char := range name {
		if char > 127 || char < 32 || char == 127 || char == '/' || char == '\\' {
			return fmt.Errorf("无效的Nginx配置名称")
		}
		isAlphaNumeric := (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9')
		if index == 0 && !isAlphaNumeric {
			return fmt.Errorf("无效的Nginx配置名称")
		}
		if !isAlphaNumeric && char != '-' && char != '_' && char != '.' {
			return fmt.Errorf("无效的Nginx配置名称")
		}
	}
	return nil
}

func ValidateNginxConfigContent(content string) error {
	if len(content) > 1<<20 {
		return fmt.Errorf("Nginx配置内容超过1 MiB限制")
	}
	return nil
}

func NormalizeNginxDomain(domain string, allowWildcard bool) (string, error) {
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

func NormalizeCertificateProvider(provider string) (string, error) {
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

func ValidateCertificateDNSConfig(config map[string]string) error {
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

func NormalizeNginxDomains(domains []string, allowWildcard bool) ([]string, error) {
	if len(domains) == 0 || len(domains) > 100 {
		return nil, fmt.Errorf("域名数量无效")
	}
	result := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		normalized, err := NormalizeNginxDomain(domain, allowWildcard)
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

func ValidateCertificateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	if len(email) > 254 || !strings.Contains(email, "@") || strings.Contains(email, " ") || strings.ContainsAny(email, "\r\n\x00") {
		return fmt.Errorf("邮箱格式无效")
	}
	return nil
}

func ValidateCertificateWebroot(webroot string) error {
	webroot = strings.TrimSpace(webroot)
	if webroot == "" || len(webroot) > 4096 || !filepath.IsAbs(webroot) {
		return fmt.Errorf("HTTP验证目录无效")
	}
	for _, char := range webroot {
		if char < 32 || char == 127 {
			return fmt.Errorf("HTTP验证目录无效")
		}
	}
	return nil
}

func ValidateNginxJSONPayload(value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("Nginx请求内容无法序列化")
	}
	if len(encoded) > 1<<20 {
		return fmt.Errorf("Nginx请求内容超过1 MiB限制")
	}
	return nil
}

func ValidateNginxSessionID(sessionID string) error {
	if sessionID == "" || sessionID != strings.TrimSpace(sessionID) || len(sessionID) > 128 {
		return fmt.Errorf("无效的session_id")
	}
	for _, char := range sessionID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return fmt.Errorf("无效的session_id")
	}
	if strings.Contains(sessionID, "..") {
		return fmt.Errorf("无效的session_id")
	}
	return nil
}
