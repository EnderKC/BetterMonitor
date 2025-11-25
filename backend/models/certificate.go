package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CertificateAccount 保存DNS/ACME账号信息
type CertificateAccount struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ServerID  uint      `json:"server_id" gorm:"index"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Config    string    `json:"config"` // JSON字符串，包含provider需要的字段
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ManagedCertificate 记录已申请的证书
type ManagedCertificate struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	ServerID        uint      `json:"server_id" gorm:"index"`
	PrimaryDomain   string    `json:"primary_domain"`
	Domains         string    `json:"-"` // 逗号分隔
	Provider        string    `json:"provider"`
	AccountID       *uint     `json:"account_id"`
	Status          string    `json:"status"`
	CertificatePath string    `json:"certificate_path"`
	KeyPath         string    `json:"key_path"`
	Expiry          time.Time `json:"expiry"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (m *ManagedCertificate) DomainList() []string {
	if m.Domains == "" {
		return []string{}
	}
	parts := strings.Split(m.Domains, ",")
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return cleaned
}

func CreateCertificateAccount(account *CertificateAccount) error {
	return DB.Create(account).Error
}

func GetCertificateAccount(serverID uint, id uint) (*CertificateAccount, error) {
	var account CertificateAccount
	if err := DB.Where("server_id = ? AND id = ?", serverID, id).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func ListCertificateAccounts(serverID uint) ([]CertificateAccount, error) {
	var accounts []CertificateAccount
	if err := DB.Where("server_id = ?", serverID).Order("id DESC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func DeleteCertificateAccount(serverID uint, id uint) error {
	return DB.Where("server_id = ? AND id = ?", serverID, id).Delete(&CertificateAccount{}).Error
}

func CreateManagedCertificate(cert *ManagedCertificate) error {
	return DB.Create(cert).Error
}

func ListManagedCertificates(serverID uint) ([]ManagedCertificate, error) {
	var certs []ManagedCertificate
	if err := DB.Where("server_id = ?", serverID).Order("id DESC").Find(&certs).Error; err != nil {
		return nil, err
	}
	return certs, nil
}

func GetManagedCertificate(serverID uint, id uint) (*ManagedCertificate, error) {
	var cert ManagedCertificate
	if err := DB.Where("server_id = ? AND id = ?", serverID, id).First(&cert).Error; err != nil {
		return nil, err
	}
	return &cert, nil
}

func DeleteManagedCertificate(serverID uint, id uint) error {
	return DB.Where("server_id = ? AND id = ?", serverID, id).Delete(&ManagedCertificate{}).Error
}

func ParseAccountConfig(account *CertificateAccount) (map[string]string, error) {
	result := make(map[string]string)
	if account == nil || account.Config == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(account.Config), &result); err != nil {
		return nil, fmt.Errorf("解析账号配置失败: %w", err)
	}
	return result, nil
}
