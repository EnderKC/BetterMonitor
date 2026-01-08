package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/services"
	"github.com/user/server-ops-backend/utils"
)

type certificateAccountRequest struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config"`
}

// ListCertificateAccounts 获取DNS证书账号
func ListCertificateAccounts(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	accounts, err := models.ListCertificateAccounts(uint(serverID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取账号失败: %v", err)})
		return
	}

	var response []map[string]interface{}
	for _, acc := range accounts {
		cfg, _ := models.ParseAccountConfig(&acc)
		maskConfig(cfg)
		response = append(response, map[string]interface{}{
			"id":         acc.ID,
			"name":       acc.Name,
			"provider":   acc.Provider,
			"config":     cfg,
			"created_at": acc.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, response)
}

func maskConfig(cfg map[string]string) {
	for key, val := range cfg {
		if val == "" {
			continue
		}
		if len(val) <= 4 {
			cfg[key] = "****"
		} else {
			cfg[key] = val[:2] + "****" + val[len(val)-2:]
		}
	}
}

// CreateCertificateAccount 新增DNS账号
func CreateCertificateAccount(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var req certificateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求参数无效: %v", err)})
		return
	}

	if req.Name == "" || req.Provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和提供商是必须的"})
		return
	}

	if req.Config == nil {
		req.Config = map[string]string{}
	}

	switch strings.ToLower(req.Provider) {
	case "alidns", "aliyun":
		if req.Config["access_key_id"] == "" || req.Config["access_key_secret"] == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "阿里云账号需要提供access_key_id和access_key_secret"})
			return
		}
	case "cloudflare", "cf":
		apiToken := strings.TrimSpace(req.Config["api_token"])
		apiEmail := strings.TrimSpace(req.Config["api_email"])
		apiKey := strings.TrimSpace(req.Config["api_key"])
		zoneToken := strings.TrimSpace(req.Config["zone_token"])

		if apiToken == "" && (apiEmail == "" || apiKey == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cloudflare账号需要提供 API Token 或 Email+API Key"})
			return
		}

		// 规范化存储，避免无意义的空字符串
		if apiToken != "" {
			req.Config["api_token"] = apiToken
		}
		if apiEmail != "" {
			req.Config["api_email"] = apiEmail
		}
		if apiKey != "" {
			req.Config["api_key"] = apiKey
		}
		if zoneToken != "" {
			req.Config["zone_token"] = zoneToken
		} else {
			delete(req.Config, "zone_token")
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "暂不支持该DNS提供商"})
		return
	}

	configBytes, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存配置失败: %v", err)})
		return
	}

	account := models.CertificateAccount{
		ServerID: uint(serverID),
		Name:     req.Name,
		Provider: strings.ToLower(req.Provider),
		Config:   string(configBytes),
	}
	if err := models.CreateCertificateAccount(&account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存账号失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "id": account.ID})
}

// DeleteCertificateAccount 删除账号
func DeleteCertificateAccount(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}
	accountID, err := strconv.Atoi(c.Param("account_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的账号ID"})
		return
	}

	if err := models.DeleteCertificateAccount(uint(serverID), uint(accountID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除账号失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListManagedCertificates 列出历史证书
func ListManagedCertificates(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	certs, err := models.ListManagedCertificates(uint(serverID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取证书失败: %v", err)})
		return
	}

	var response []map[string]interface{}
	for _, cert := range certs {
		response = append(response, map[string]interface{}{
			"id":               cert.ID,
			"primary_domain":   cert.PrimaryDomain,
			"domains":          cert.DomainList(),
			"provider":         cert.Provider,
			"account_id":       cert.AccountID,
			"status":           cert.Status,
			"certificate_path": cert.CertificatePath,
			"key_path":         cert.KeyPath,
			"expiry":           cert.Expiry,
			"created_at":       cert.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, response)
}

// DeleteManagedCertificate 删除证书记录
func DeleteManagedCertificate(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}
	certID, err := strconv.Atoi(c.Param("cert_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的证书ID"})
		return
	}

	if err := models.DeleteManagedCertificate(uint(serverID), uint(certID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除证书失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetCertificateContent 返回证书/私钥文件内容
func GetCertificateContent(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	certID, err := strconv.Atoi(c.Param("cert_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的证书ID"})
		return
	}

	cert, err := models.GetManagedCertificate(server.ID, uint(certID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "证书记录不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	if cert.CertificatePath == "" && cert.KeyPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "证书记录没有文件路径"})
		return
	}

	payload := map[string]interface{}{
		"action":           "certificate_content",
		"certificate_path": cert.CertificatePath,
		"key_path":         cert.KeyPath,
	}

	message := map[string]interface{}{
		"type":    "nginx_command",
		"payload": payload,
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// RenewCertificate 手动触发证书续期
func RenewCertificate(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	certID, err := strconv.Atoi(c.Param("cert_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的证书ID"})
		return
	}

	// 调用续期服务
	renewalService := services.GetCertificateRenewalService()
	if err := renewalService.RenewCertificateManually(uint(serverID), uint(certID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("证书续期失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "证书续期成功"})
}

