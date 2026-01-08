package services

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

// 全局CertificateRenewalService实例
var (
	globalRenewalService *CertificateRenewalService
	renewalServiceOnce   sync.Once
)

// CertificateRenewalService 证书自动续期服务
type CertificateRenewalService struct {
	stopChan chan struct{}
	mu       sync.Mutex
}

// NewCertificateRenewalService 创建证书续期服务
func NewCertificateRenewalService() *CertificateRenewalService {
	return &CertificateRenewalService{
		stopChan: make(chan struct{}),
	}
}

// GetCertificateRenewalService 获取全局证书续期服务实例
func GetCertificateRenewalService() *CertificateRenewalService {
	renewalServiceOnce.Do(func() {
		globalRenewalService = NewCertificateRenewalService()
	})
	return globalRenewalService
}

// Start 启动证书续期服务
func (s *CertificateRenewalService) Start() {
	// 每12小时检查一次证书到期情况
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	log.Println("证书自动续期服务已启动")

	// 启动时立即执行一次检查
	s.checkAndRenewCertificates()

	for {
		select {
		case <-ticker.C:
			s.checkAndRenewCertificates()
		case <-s.stopChan:
			log.Println("证书自动续期服务已停止")
			return
		}
	}
}

// Stop 停止证书续期服务
func (s *CertificateRenewalService) Stop() {
	close(s.stopChan)
}

// checkAndRenewCertificates 检查并续期即将到期的证书
func (s *CertificateRenewalService) checkAndRenewCertificates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("开始检查即将到期的证书...")

	// 获取30天内即将到期的证书
	certs, err := models.GetExpiringCertificates(30)
	if err != nil {
		log.Printf("获取即将到期的证书失败: %v", err)
		return
	}

	if len(certs) == 0 {
		log.Println("没有需要续期的证书")
		return
	}

	log.Printf("发现 %d 个证书需要续期", len(certs))

	for _, cert := range certs {
		daysLeft := int(time.Until(cert.Expiry).Hours() / 24)
		log.Printf("处理证书: %s (服务器ID: %d, 剩余天数: %d)", cert.PrimaryDomain, cert.ServerID, daysLeft)

		if err := s.renewCertificate(&cert); err != nil {
			log.Printf("证书续期失败 [%s]: %v", cert.PrimaryDomain, err)
		} else {
			log.Printf("证书续期成功 [%s]", cert.PrimaryDomain)
		}
	}

	log.Println("证书续期检查完成")
}

// renewCertificate 续期单个证书
func (s *CertificateRenewalService) renewCertificate(cert *models.ManagedCertificate) error {
	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, cert.ServerID).Error; err != nil {
		return fmt.Errorf("获取服务器信息失败: %w", err)
	}

	// 检查服务器是否在线
	models.CheckServerStatus(&server)
	if !server.Online {
		return fmt.Errorf("服务器离线，无法续期")
	}

	// 获取DNS账号配置
	var account *models.CertificateAccount
	if cert.AccountID != nil {
		acc, err := models.GetCertificateAccount(cert.ServerID, *cert.AccountID)
		if err != nil {
			return fmt.Errorf("获取DNS账号配置失败: %w", err)
		}
		account = acc
	}

	// 如果没有关联账号且使用DNS方式，无法续期
	if account == nil && (cert.Provider == "alidns" || cert.Provider == "aliyun" || cert.Provider == "cloudflare" || cert.Provider == "cf") {
		return fmt.Errorf("DNS验证方式需要配置DNS账号")
	}

	// 构建续期请求
	payload := map[string]interface{}{
		"action":  "issue_ssl",
		"domains": cert.DomainList(),
	}

	// 设置provider和DNS配置
	if account != nil {
		payload["provider"] = account.Provider
		dnsConfig, err := models.ParseAccountConfig(account)
		if err != nil {
			return fmt.Errorf("解析DNS配置失败: %w", err)
		}
		payload["dns_config"] = dnsConfig
	} else {
		// HTTP-01验证方式（需要Agent自动处理webroot）
		payload["provider"] = "http01"
	}

	// 发送续期命令到Agent
	message := map[string]interface{}{
		"type":    "nginx_command",
		"payload": payload,
	}

	log.Printf("发送证书续期请求: %s (服务器: %d)", cert.PrimaryDomain, cert.ServerID)

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		// 更新状态为续期失败
		models.UpdateCertificateRenewalStatus(cert.ServerID, cert.ID, "续期失败")
		return fmt.Errorf("发送续期命令失败: %w", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		models.UpdateCertificateRenewalStatus(cert.ServerID, cert.ID, "续期失败")
		return fmt.Errorf("解析续期响应失败: %w", err)
	}

	// 检查是否成功
	success, ok := result["success"].(bool)
	if !ok || !success {
		errMsg := "未知错误"
		if msg, exists := result["error"].(string); exists {
			errMsg = msg
		}
		models.UpdateCertificateRenewalStatus(cert.ServerID, cert.ID, "续期失败")
		return fmt.Errorf("续期失败: %s", errMsg)
	}

	// 获取新的证书信息
	certPath, _ := result["certificate_path"].(string)
	keyPath, _ := result["key_path"].(string)
	expiryStr, _ := result["expiry"].(string)

	var newExpiry time.Time
	if expiryStr != "" {
		if parsed, err := time.Parse(time.RFC3339, expiryStr); err == nil {
			newExpiry = parsed
		}
	}

	// 如果没有解析到到期时间，默认设置为90天后
	if newExpiry.IsZero() {
		newExpiry = time.Now().AddDate(0, 0, 90)
	}

	// 更新数据库中的证书信息
	if err := models.UpdateCertificateStatus(cert.ServerID, cert.ID, "有效", newExpiry, certPath, keyPath); err != nil {
		return fmt.Errorf("更新证书状态失败: %w", err)
	}

	return nil
}

// RenewCertificateManually 手动触发单个证书续期
func (s *CertificateRenewalService) RenewCertificateManually(serverID uint, certID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取证书信息
	cert, err := models.GetManagedCertificate(serverID, certID)
	if err != nil {
		return fmt.Errorf("证书不存在: %w", err)
	}

	log.Printf("手动触发证书续期: %s (服务器ID: %d)", cert.PrimaryDomain, serverID)

	return s.renewCertificate(cert)
}
