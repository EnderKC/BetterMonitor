package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
)

type DeclarativeSiteRequest struct {
	Domain       string                 `json:"domain"`
	Domains      []string               `json:"domains"`
	ExtraDomains []string               `json:"extra_domains"`
	Config       map[string]interface{} `json:"config"`
}

type DeclarativeSSLRequest struct {
	Domain     string            `json:"domain"`
	Domains    []string          `json:"domains"`
	Provider   string            `json:"provider"`
	Email      string            `json:"email"`
	Webroot    string            `json:"webroot"`
	UseStaging bool              `json:"use_staging"`
	AccountID  *uint             `json:"account_id"`
	DNSConfig  map[string]string `json:"dns_config"`
}

func loadNginxServer(c *gin.Context) (*models.Server, bool) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || serverID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_server_id", "error": "无效的服务器ID"})
		return nil, false
	}
	var server models.Server
	if err := models.DB.First(&server, uint(serverID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "server_not_found", "error": "服务器不存在"})
		return nil, false
	}
	if !isServerOnline(&server) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "agent_offline", "error": "服务器当前离线，无法连接"})
		return nil, false
	}
	return &server, true
}

func sendNginxAgentCommand(
	ctx context.Context,
	serverID uint,
	payload map[string]interface{},
	timeout time.Duration,
) (json.RawMessage, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return utils.SendAgentCommand(requestCtx, serverID, "nginx_command", payload, "nginx_success")
}

func writeNginxAgentError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	code := "nginx_agent_failed"
	message := "Nginx 操作失败"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
		code = "nginx_agent_timeout"
		message = "Nginx 操作超时"
	case errors.Is(err, context.Canceled):
		status = http.StatusRequestTimeout
		code = "nginx_request_canceled"
		message = "Nginx 请求已取消"
	case errors.Is(err, utils.ErrAgentCommandSenderNotConfigured):
		status = http.StatusServiceUnavailable
		code = "agent_unavailable"
		message = "服务器 Agent 不可用"
	default:
		var commandErr *utils.AgentCommandError
		if errors.As(err, &commandErr) {
			code = commandErr.Code
		}
	}
	c.JSON(status, gin.H{"code": code, "error": message})
}

func decodeNginxObject(response json.RawMessage) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = map[string]interface{}{}
	}
	return result, nil
}

func bindStrictNginxJSON(c *gin.Context, target interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, (2<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("请求体包含多余JSON值")
	}
	return nil
}

func runNginxJSONCommand(
	c *gin.Context,
	payload map[string]interface{},
	timeout time.Duration,
	target interface{},
) bool {
	server, ok := loadNginxServer(c)
	if !ok {
		return false
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, payload, timeout)
	if err != nil {
		writeNginxAgentError(c, err)
		return false
	}
	if err := json.Unmarshal(response, target); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return false
	}
	return true
}

func respondNginxJSONCommand(c *gin.Context, payload map[string]interface{}, timeout time.Duration) {
	var result interface{}
	if runNginxJSONCommand(c, payload, timeout, &result) {
		c.JSON(http.StatusOK, result)
	}
}

// NginxConfigsList 获取Nginx配置文件列表
func NginxConfigsList(c *gin.Context) {
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action": "nginx_configs_list",
	}, TimeoutSimpleQuery)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// NginxConfigContent 获取Nginx配置文件内容
func NginxConfigContent(c *gin.Context) {
	configID := c.Param("config_id")
	if err := ValidateNginxManagedID(configID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_config_id", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action":    "nginx_config_content",
		"config_id": configID,
	}, TimeoutSimpleQuery)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	var content string
	if err := json.Unmarshal(response, &content); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.String(http.StatusOK, content)
}

// SaveNginxConfig 保存Nginx配置文件内容
func SaveNginxConfig(c *gin.Context) {
	configID := c.Param("config_id")
	if err := ValidateNginxManagedID(configID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_config_id", "error": err.Error()})
		return
	}
	var reqBody struct {
		Content string `json:"content"`
	}
	if err := bindStrictNginxJSON(c, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_request", "error": "无效的请求参数"})
		return
	}
	if err := ValidateNginxConfigContent(reqBody.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_content", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action":    "nginx_save_config",
		"config_id": configID,
		"content":   reqBody.Content,
	}, TimeoutLongOperation)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	result, err := decodeNginxObject(response)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateNginxConfig 创建Nginx配置文件
func CreateNginxConfig(c *gin.Context) {
	var reqBody struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := bindStrictNginxJSON(c, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_request", "error": "无效的请求参数"})
		return
	}
	if err := ValidateNginxConfigName(reqBody.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_config_name", "error": err.Error()})
		return
	}
	if err := ValidateNginxConfigContent(reqBody.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_content", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action":  "nginx_create_config",
		"name":    reqBody.Name,
		"content": reqBody.Content,
	}, TimeoutLongOperation)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	result, err := decodeNginxObject(response)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteNginxConfig 删除Nginx配置文件
func DeleteNginxConfig(c *gin.Context) {
	configID := c.Param("config_id")
	if err := ValidateNginxManagedID(configID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_config_id", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action":    "nginx_delete_config",
		"config_id": configID,
	}, TimeoutLongOperation)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	result, err := decodeNginxObject(response)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// NginxLogsList 获取Nginx日志文件列表
func NginxLogsList(c *gin.Context) {
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action": "nginx_logs_list",
	}, TimeoutSimpleQuery)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// NginxLogContent 获取Nginx日志文件内容
func NginxLogContent(c *gin.Context) {
	logID := c.Param("log_id")
	if err := ValidateNginxManagedID(logID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_log_id", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action": "nginx_log_content",
		"log_id": logID,
	}, TimeoutSimpleQuery)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	var content string
	if err := json.Unmarshal(response, &content); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.String(http.StatusOK, content)
}

// DownloadNginxLog 下载Nginx日志文件
func DownloadNginxLog(c *gin.Context) {
	logID := c.Param("log_id")
	if err := ValidateNginxManagedID(logID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_log_id", "error": err.Error()})
		return
	}
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}
	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, map[string]interface{}{
		"action": "nginx_log_download",
		"log_id": logID,
	}, TimeoutSimpleQuery)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}
	var respData struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(response, &respData); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", respData.Filename))
	c.Header("Content-Type", "application/octet-stream")
	c.String(http.StatusOK, respData.Content)
}

// RestartNginx 重启Nginx服务
func RestartNginx(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_restart"}, TimeoutLongOperation)
}

// StopNginx 停止Nginx服务
func StopNginx(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_stop"}, TimeoutLongOperation)
}

// StartNginx 启动Nginx服务
func StartNginx(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_start"}, TimeoutLongOperation)
}

// TestNginxConfig 测试Nginx配置
func TestNginxConfig(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_test_config"}, TimeoutLongOperation)
}

// GetNginxProcesses 获取Nginx相关进程
func GetNginxProcesses(c *gin.Context) {
	var result []map[string]interface{}
	if runNginxJSONCommand(c, map[string]interface{}{"action": "nginx_processes"}, TimeoutSimpleQuery, &result) {
		c.JSON(http.StatusOK, result)
	}
}

// GetNginxPorts 获取Nginx占用的端口
func GetNginxPorts(c *gin.Context) {
	var result []map[string]interface{}
	if runNginxJSONCommand(c, map[string]interface{}{"action": "nginx_ports"}, TimeoutSimpleQuery, &result) {
		c.JSON(http.StatusOK, result)
	}
}

// ListWebsites 获取网站列表
func ListWebsites(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_sites_list"}, TimeoutSimpleQuery)
}

// GetWebsiteDetail 获取单个网站的详细配置
func GetWebsiteDetail(c *gin.Context) {
	domain, err := NormalizeNginxDomain(c.Param("domain"), false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domain", "error": err.Error()})
		return
	}
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_site_detail", "domain": domain}, TimeoutSimpleQuery)
}

// GetWebsiteNginxConfig 获取网站的原始nginx配置文件内容
func GetWebsiteNginxConfig(c *gin.Context) {
	domain, err := NormalizeNginxDomain(c.Param("domain"), false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domain", "error": err.Error()})
		return
	}
	respondNginxJSONCommand(c, map[string]interface{}{"action": "nginx_get_raw_config", "domain": domain}, TimeoutSimpleQuery)
}

// SaveWebsiteNginxConfig 保存网站的nginx配置文件
func SaveWebsiteNginxConfig(c *gin.Context) {
	domain, err := NormalizeNginxDomain(c.Param("domain"), false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domain", "error": err.Error()})
		return
	}
	var reqBody struct {
		Content string `json:"content"`
	}
	if err := bindStrictNginxJSON(c, &reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_request", "error": "无效的请求参数"})
		return
	}
	if err := ValidateNginxConfigContent(reqBody.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_content", "error": err.Error()})
		return
	}
	respondNginxJSONCommand(c, map[string]interface{}{
		"action": "nginx_save_raw_config", "domain": domain, "content": reqBody.Content,
	}, TimeoutLongOperation)
}

// OpenRestyStatus 查看节点OpenResty安装状态
func OpenRestyStatus(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "openresty_status"}, TimeoutSimpleQuery)
}

// InstallOpenResty 一键安装OpenResty容器
func InstallOpenResty(c *gin.Context) {
	respondNginxJSONCommand(c, map[string]interface{}{"action": "openresty_install"}, TimeoutLongOperation)
}

// GetOpenRestyInstallLogs 获取OpenResty安装日志
func GetOpenRestyInstallLogs(c *gin.Context) {
	sessionID := c.Query("session_id")
	if err := ValidateNginxSessionID(sessionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_session_id", "error": err.Error()})
		return
	}
	var result map[string]interface{}
	if runNginxJSONCommand(c, map[string]interface{}{
		"action": "openresty_install_logs", "session_id": sessionID,
	}, TimeoutSimpleQuery, &result) {
		c.JSON(http.StatusOK, result)
	}
}

// ApplyWebsiteConfig 通过声明式配置应用站点
func ApplyWebsiteConfig(c *gin.Context) {
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}

	var req DeclarativeSiteRequest
	if err := bindStrictNginxJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_request", "error": "请求参数无效"})
		return
	}

	if req.Config == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "config字段是必须的"})
		return
	}

	if req.Domain == "" && len(req.Domains) > 0 {
		req.Domain = req.Domains[0]
	}
	if req.Domain == "" {
		if value, ok := req.Config["primary_domain"].(string); ok && value != "" {
			req.Domain = value
		}
	}
	var err error
	req.Domain, err = NormalizeNginxDomain(req.Domain, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domain", "error": err.Error()})
		return
	}
	if len(req.Domains) > 0 {
		req.Domains, err = NormalizeNginxDomains(req.Domains, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domains", "error": err.Error()})
			return
		}
	}
	if len(req.ExtraDomains) > 0 {
		req.ExtraDomains, err = NormalizeNginxDomains(req.ExtraDomains, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domains", "error": err.Error()})
			return
		}
	}
	if err := ValidateNginxJSONPayload(req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_config", "error": err.Error()})
		return
	}

	if certID := extractUint(req.Config["certificate_id"]); certID > 0 {
		cert, err := models.GetManagedCertificate(server.ID, certID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("选择的证书不存在: %v", err)})
			return
		}
		req.Config["enable_https"] = true
		req.Config["ssl"] = map[string]interface{}{
			"certificate":     cert.CertificatePath,
			"certificate_key": cert.KeyPath,
		}
		delete(req.Config, "certificate_id")
	}

	payload := map[string]interface{}{
		"action": "apply_config",
		"domain": req.Domain,
		"config": req.Config,
	}
	if len(req.Domains) > 0 {
		payload["domains"] = req.Domains
	}
	if len(req.ExtraDomains) > 0 {
		payload["extra_domains"] = req.ExtraDomains
	}

	respondNginxJSONCommand(c, payload, TimeoutLongOperation)
}

// IssueWebsiteCertificate 使用Lego签发证书
func IssueWebsiteCertificate(c *gin.Context) {
	server, ok := loadNginxServer(c)
	if !ok {
		return
	}

	var req DeclarativeSSLRequest
	if err := bindStrictNginxJSON(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_nginx_request", "error": "请求参数无效"})
		return
	}

	if len(req.Domains) == 0 && req.Domain != "" {
		req.Domains = []string{req.Domain}
	}
	if len(req.Domains) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供至少一个域名"})
		return
	}

	var err error
	req.Domains, err = NormalizeNginxDomains(req.Domains, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_domains", "error": err.Error()})
		return
	}
	provider, err := NormalizeCertificateProvider(req.Provider)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_provider", "error": err.Error()})
		return
	}
	if err := ValidateCertificateEmail(req.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_email", "error": err.Error()})
		return
	}

	var dnsConfig map[string]string
	if req.AccountID != nil && *req.AccountID > 0 {
		account, err := models.GetCertificateAccount(server.ID, *req.AccountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("获取DNS账号失败: %v", err)})
			return
		}
		cfg, err := models.ParseAccountConfig(account)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("解析账号配置失败: %v", err)})
			return
		}
		if provider == "http01" {
			provider = account.Provider
		}
		dnsConfig = cfg
	}

	if provider == "http01" && strings.TrimSpace(req.Webroot) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTTP验证需要指定webroot"})
		return
	}
	if provider == "http01" {
		if err := ValidateCertificateWebroot(req.Webroot); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_webroot", "error": err.Error()})
			return
		}
	}
	if provider != "http01" {
		if len(req.DNSConfig) > 0 {
			dnsConfig = req.DNSConfig
		}
		if len(dnsConfig) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "DNS验证需要提供账号配置"})
			return
		}
		if err := ValidateCertificateDNSConfig(dnsConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_dns_config", "error": err.Error()})
			return
		}
	}

	payload := map[string]interface{}{
		"action":      "issue_ssl",
		"domains":     req.Domains,
		"provider":    provider,
		"email":       req.Email,
		"webroot":     req.Webroot,
		"use_staging": req.UseStaging,
	}
	if req.AccountID != nil && *req.AccountID > 0 {
		payload["account_id"] = req.AccountID
	}
	if len(dnsConfig) > 0 {
		payload["dns_config"] = dnsConfig
	}

	response, err := sendNginxAgentCommand(c.Request.Context(), server.ID, payload, TimeoutLongOperation)
	if err != nil {
		writeNginxAgentError(c, err)
		return
	}

	var respData map[string]interface{}
	if err := json.Unmarshal(response, &respData); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "invalid_agent_response", "error": "Agent响应格式无效"})
		return
	}

	// 成功后记录证书信息
	domainList := req.Domains
	if len(domainList) == 0 && req.Domain != "" {
		domainList = []string{req.Domain}
	}
	expiryStr, _ := respData["expiry"].(string)
	expiryTime, _ := time.Parse(time.RFC3339, expiryStr)

	accountID := uint(0)
	if req.AccountID != nil {
		accountID = *req.AccountID
	}

	certRecord := models.ManagedCertificate{
		ServerID:        server.ID,
		PrimaryDomain:   domainList[0],
		Domains:         strings.Join(domainList, ","),
		Provider:        provider,
		Status:          "issued",
		CertificatePath: fmt.Sprintf("%v", respData["certificate_path"]),
		KeyPath:         fmt.Sprintf("%v", respData["key_path"]),
		Expiry:          expiryTime,
	}
	if accountID > 0 {
		certRecord.AccountID = &accountID
	}

	if err := models.CreateManagedCertificate(&certRecord); err == nil {
		respData["certificate_id"] = certRecord.ID
	}

	c.JSON(http.StatusOK, respData)
}

func extractUint(value interface{}) uint {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return uint(v)
		}
	case int:
		if v > 0 {
			return uint(v)
		}
	case int64:
		if v > 0 {
			return uint(v)
		}
	case json.Number:
		if i, err := v.Int64(); err == nil && i > 0 {
			return uint(i)
		}
	case string:
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			return uint(i)
		}
	}
	return 0
}
