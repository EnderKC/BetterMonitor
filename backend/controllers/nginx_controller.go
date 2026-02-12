package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

// NginxConfigsList 获取Nginx配置文件列表
func NginxConfigsList(c *gin.Context) {
	serverId := c.Param("id")
	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	log.Printf("[DEBUG] 获取服务器 %d 的Nginx配置文件列表", id)

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		log.Printf("[ERROR] 服务器 %d 不存在: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 检查服务器在线状态
	models.CheckServerStatus(&server)
	log.Printf("[DEBUG] 服务器 %d 当前在线状态: %t, 状态: %s", id, server.Online, server.Status)

	// 如果服务器离线，直接返回错误
	if !server.Online {
		log.Printf("[WARN] 服务器 %d 当前离线，无法获取Nginx配置文件列表", id)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	// 检查WebSocket连接是否存在于utils的连接池中
	_, err = utils.GetAgentConnectionFromMap(server.ID)
	if err != nil {
		log.Printf("[WARN] 通过旧连接池无法获取服务器 %d 的agent连接: %v", id, err)
	} else {
		log.Printf("[DEBUG] 旧连接池中存在服务器 %d 的agent连接", id)
	}

	// 构建符合Agent期望的请求格式
	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_configs_list",
		},
	}

	log.Printf("[DEBUG] 向服务器 %d 发送nginx_configs_list命令", id)
	log.Printf("[DEBUG] utils.GetAgentConnectionFunc的值: %v", utils.GetAgentConnectionFunc != nil)

	// 使用带有超时的上下文
	ctx, cancel := context.WithTimeout(c.Request.Context(), TimeoutSimpleQuery)
	defer cancel()

	// 创建一个通道来接收响应
	respChan := make(chan struct {
		data string
		err  error
	}, 1)

	// 在单独的goroutine中调用SendCommandToAgent
	go func() {
		resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
		respChan <- struct {
			data string
			err  error
		}{data: resp, err: err}
	}()

	// 等待响应或超时
	select {
	case result := <-respChan:
		if result.err != nil {
			log.Printf("[ERROR] 发送命令到服务器 %d 失败: %v", id, result.err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", result.err)})
			return
		}

		log.Printf("[DEBUG] 收到nginx_configs_list响应: %s", result.data)

		// 解析响应 - 使用json.RawMessage先获取原始数据
		var respData interface{}
		if err := json.Unmarshal([]byte(result.data), &respData); err != nil {
			log.Printf("[ERROR] 解析服务器 %d 的响应失败: %v, 原始数据: %s", id, err, result.data)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
			return
		}

		log.Printf("[DEBUG] 成功获取服务器 %d 的Nginx配置文件列表", id)
		c.JSON(http.StatusOK, respData)

	case <-ctx.Done():
		log.Printf("[ERROR] 获取服务器 %d 的Nginx配置超时", id)
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "获取Nginx配置超时，请稍后重试"})
	}
}

// NginxConfigContent 获取Nginx配置文件内容
func NginxConfigContent(c *gin.Context) {
	serverId := c.Param("id")
	configId := c.Param("config_id") // 修改为正确的参数名

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据 - 需要传递config_id而不是直接使用path
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":    "nginx_config_content",
			"config_id": configId, // 传递配置ID而不是路径
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	c.String(http.StatusOK, resp)
}

// SaveNginxConfig 保存Nginx配置文件内容
// NOTE: 当前实现需要两次 Agent 往返（先获取配置列表定位文件路径，再保存内容）。
// 如需优化为单次往返，需要 Agent 侧支持通过 config_id 直接保存，暂不做此改动。
func SaveNginxConfig(c *gin.Context) {
	serverId := c.Param("id")
	configId := c.Param("config_id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 获取请求体
	var reqBody struct {
		Content string `json:"content"`
		Path    string `json:"path"` // 可选参数，前端可能提供路径
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 首先通过configId获取配置列表，查找对应的配置文件路径
	// 不直接使用前端传递的path，而是使用通过ID查找到的path，更安全
	configsListReqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_configs_list",
		},
	}

	configsListResp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, configsListReqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取配置列表失败: %v", err)})
		return
	}

	// 解析配置列表响应
	var configsList []map[string]interface{}
	if err := json.Unmarshal([]byte(configsListResp), &configsList); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析配置列表失败: %v", err)})
		return
	}

	// 查找匹配ID的配置
	var configPath string
	for _, config := range configsList {
		if id, ok := config["id"].(string); ok && id == configId {
			if path, ok := config["path"].(string); ok {
				configPath = path
				break
			}
		}
	}

	if configPath == "" {
		// 如果在列表中找不到对应ID的配置，尝试使用请求体中的路径
		if reqBody.Path != "" {
			configPath = reqBody.Path
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("未找到ID为%s的配置文件", configId)})
			return
		}
	}

	// 构建保存配置的请求数据
	saveReqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":  "nginx_save_config",
			"path":    configPath,
			"content": reqBody.Content,
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, saveReqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// CreateNginxConfig 创建Nginx配置文件
func CreateNginxConfig(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 获取请求体
	var reqBody struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":  "nginx_create_config",
			"name":    reqBody.Name,
			"path":    reqBody.Path,
			"content": reqBody.Content,
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteNginxConfig 删除Nginx配置文件
func DeleteNginxConfig(c *gin.Context) {
	serverId := c.Param("id")
	configId := c.Param("config_id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":    "nginx_delete_config",
			"config_id": configId,
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// NginxLogsList 获取Nginx日志文件列表
func NginxLogsList(c *gin.Context) {
	serverId := c.Param("id")
	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_logs_list",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 解析响应
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// NginxLogContent 获取Nginx日志文件内容
func NginxLogContent(c *gin.Context) {
	serverId := c.Param("id")
	logId := c.Param("log_id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_log_content",
			"id":     logId,
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	c.String(http.StatusOK, resp)
}

// DownloadNginxLog 下载Nginx日志文件
func DownloadNginxLog(c *gin.Context) {
	serverId := c.Param("id")
	logId := c.Param("log_id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_log_download",
			"id":     logId,
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 解析响应，提取文件名和内容
	var respData struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(resp), &respData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 设置响应头，以便浏览器下载文件
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", respData.Filename))
	c.Header("Content-Type", "application/octet-stream")
	c.String(http.StatusOK, respData.Content)
}

// RestartNginx 重启Nginx服务
func RestartNginx(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_restart",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// StopNginx 停止Nginx服务
func StopNginx(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_stop",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// StartNginx 启动Nginx服务
func StartNginx(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_start",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// TestNginxConfig 测试Nginx配置
func TestNginxConfig(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_test_config",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 【安全修复】解析并验证响应
	result, err := parseAndValidateNginxResponse(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetNginxProcesses 获取Nginx相关进程
func GetNginxProcesses(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_processes",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 解析响应
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetNginxPorts 获取Nginx占用的端口
func GetNginxPorts(c *gin.Context) {
	serverId := c.Param("id")

	id, err := strconv.Atoi(serverId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 获取服务器信息
	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 构建请求数据
	reqData := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_ports",
		},
	}

	// 通过WebSocket发送命令给Agent
	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, reqData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	// 解析响应
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, result)
}

// checkNginxResponseError 检查解析为 interface{} 的Agent响应是否包含错误指示
// 某些端点将响应解析为 interface{} 因为JSON可能是对象或数组
// 如果响应是包含错误指示的map，返回错误；否则返回nil
func checkNginxResponseError(result interface{}) error {
	respMap, ok := result.(map[string]interface{})
	if !ok || respMap == nil {
		// 不是map类型（可能是数组），不检查错误
		return nil
	}

	hasErrorIndicator := false

	// 检查 type 字段
	if v, ok := respMap["type"].(string); ok && (v == "error" || v == "nginx_error") {
		hasErrorIndicator = true
	}

	// 检查 status 字段
	if v, ok := respMap["status"].(string); ok && (v == "error" || v == "failed" || v == "failure") {
		hasErrorIndicator = true
	}

	// 检查 success 字段
	if v, ok := respMap["success"].(bool); ok && !v {
		hasErrorIndicator = true
	}

	// 检查 error 字段
	errText := ""
	if v, exists := respMap["error"]; exists && v != nil {
		switch t := v.(type) {
		case string:
			errText = t
		default:
			errText = fmt.Sprintf("%v", t)
		}
		if errText != "" {
			hasErrorIndicator = true
		}
	}

	if !hasErrorIndicator {
		return nil
	}

	// 提取错误消息
	msg := errText
	if msg == "" {
		if v, ok := respMap["message"].(string); ok && v != "" {
			msg = v
		} else if v, ok := respMap["msg"].(string); ok && v != "" {
			msg = v
		}
	}
	if msg == "" {
		msg = "未知错误"
	}

	return fmt.Errorf("Nginx Agent响应错误: %s", msg)
}

// ListWebsites 获取网站列表
func ListWebsites(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_sites_list",
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetWebsiteDetail 获取单个网站的详细配置
func GetWebsiteDetail(c *gin.Context) {
	serverID := c.Param("id")
	domain := c.Param("domain")

	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少域名参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	// 向Agent请求网站详情
	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_site_detail",
			"domain": domain,
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取网站详情失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetWebsiteNginxConfig 获取网站的原始nginx配置文件内容
func GetWebsiteNginxConfig(c *gin.Context) {
	serverID := c.Param("id")
	domain := c.Param("domain")

	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少域名参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	// 向Agent请求原始配置文件
	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "nginx_get_raw_config",
			"domain": domain,
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取配置文件失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// SaveWebsiteNginxConfig 保存网站的nginx配置文件
func SaveWebsiteNginxConfig(c *gin.Context) {
	serverID := c.Param("id")
	domain := c.Param("domain")

	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少域名参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	// 获取请求体
	var reqBody struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	if reqBody.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "配置内容不能为空"})
		return
	}

	// 向Agent发送保存配置的命令
	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":  "nginx_save_raw_config",
			"domain":  domain,
			"content": reqBody.Content,
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("保存配置失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// OpenRestyStatus 查看节点OpenResty安装状态
func OpenRestyStatus(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "openresty_status",
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// InstallOpenResty 一键安装OpenResty容器
func InstallOpenResty(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action": "openresty_install",
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetOpenRestyInstallLogs 获取OpenResty安装日志
func GetOpenRestyInstallLogs(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少session_id参数"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	message := map[string]interface{}{
		"type": "nginx_command",
		"payload": map[string]interface{}{
			"action":     "openresty_install_logs",
			"session_id": sessionID,
		},
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取安装日志失败: %v", err)})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := validateNginxResponse(result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ApplyWebsiteConfig 通过声明式配置应用站点
func ApplyWebsiteConfig(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	var req DeclarativeSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求参数无效: %v", err)})
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
	if req.Domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain字段是必须的"})
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

	message := map[string]interface{}{
		"type":    "nginx_command",
		"payload": payload,
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var respData interface{}
	if err := json.Unmarshal([]byte(resp), &respData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误
	if err := checkNginxResponseError(respData); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, respData)
}

// IssueWebsiteCertificate 使用Lego签发证书
func IssueWebsiteCertificate(c *gin.Context) {
	serverID := c.Param("id")
	id, err := strconv.Atoi(serverID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	var server models.Server
	if err := models.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	models.CheckServerStatus(&server)
	if !server.Online {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务器当前离线，无法连接"})
		return
	}

	var req DeclarativeSSLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求参数无效: %v", err)})
		return
	}

	if len(req.Domains) == 0 && req.Domain != "" {
		req.Domains = []string{req.Domain}
	}
	if len(req.Domains) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供至少一个域名"})
		return
	}

	provider := strings.ToLower(req.Provider)
	if provider == "" {
		provider = "http01"
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
	if provider != "http01" {
		if len(req.DNSConfig) > 0 {
			dnsConfig = req.DNSConfig
		}
		if len(dnsConfig) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "DNS验证需要提供账号配置"})
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

	message := map[string]interface{}{
		"type":    "nginx_command",
		"payload": payload,
	}

	resp, err := utils.SendCommandToAgent(server.ID, server.SecretKey, message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送命令失败: %v", err)})
		return
	}

	var respData map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &respData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)})
		return
	}

	// 【安全修复】验证Agent响应是否包含错误，防止在错误响应时创建无效证书记录
	if err := validateNginxResponse(respData); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
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

// 【安全修复】validateNginxResponse 验证Nginx Agent响应是否成功
// 检查多种可能的错误指示字段，确保Agent返回成功状态
func validateNginxResponse(resp map[string]interface{}) error {
	// 检查 status 字段
	if status, ok := resp["status"].(string); ok {
		if status == "error" || status == "failed" || status == "failure" {
			errMsg := extractNginxErrorMessage(resp)
			return fmt.Errorf("Nginx操作失败: %s", errMsg)
		}
	}

	// 检查 success 字段（如果存在）
	if success, ok := resp["success"].(bool); ok && !success {
		errMsg := extractNginxErrorMessage(resp)
		return fmt.Errorf("Nginx操作失败: %s", errMsg)
	}

	// 检查 error 字段是否存在且非空
	if errField, ok := resp["error"].(string); ok && errField != "" {
		return fmt.Errorf("Nginx错误: %s", errField)
	}

	// 检查 type 字段是否为 error
	if respType, ok := resp["type"].(string); ok {
		if respType == "error" || respType == "nginx_error" {
			errMsg := extractNginxErrorMessage(resp)
			return fmt.Errorf("Nginx Agent错误: %s", errMsg)
		}
	}

	return nil
}

// extractNginxErrorMessage 从响应中提取错误信息
func extractNginxErrorMessage(resp map[string]interface{}) string {
	// 优先从 error 字段提取
	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		return errMsg
	}
	// 从 message 字段提取
	if msg, ok := resp["message"].(string); ok && msg != "" {
		return msg
	}
	// 从 msg 字段提取
	if msg, ok := resp["msg"].(string); ok && msg != "" {
		return msg
	}
	// 从 data.error 字段提取
	if data, ok := resp["data"].(map[string]interface{}); ok {
		if errMsg, ok := data["error"].(string); ok && errMsg != "" {
			return errMsg
		}
		if msg, ok := data["message"].(string); ok && msg != "" {
			return msg
		}
	}
	return "未知错误"
}

// parseAndValidateNginxResponse 解析并验证Nginx响应
// 返回解析后的数据和可能的错误
func parseAndValidateNginxResponse(respStr string) (map[string]interface{}, error) {
	if respStr == "" {
		return nil, fmt.Errorf("Agent返回空响应")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(respStr), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	// 验证响应是否包含错误
	if err := validateNginxResponse(result); err != nil {
		return nil, err
	}

	return result, nil
}
