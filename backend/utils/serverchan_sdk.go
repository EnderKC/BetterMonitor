package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// ServerChanAPIURL ServerChan API地址
	ServerChanAPIURL = "https://sctapi.ftqq.com"
)

// ServerChanResponse ServerChan API返回结果
type ServerChanResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		PushID  string `json:"pushid"`
		ReadKey string `json:"readkey"`
		Error   string `json:"error"`
		SealID  string `json:"sealid"`
	} `json:"data"`
}

// ServerChanSend 使用ServerChan发送消息
func ServerChanSend(sendKey, title, content string) (*ServerChanResponse, error) {
	if sendKey == "" {
		return nil, fmt.Errorf("SendKey不能为空")
	}

	// 标题不能为空，最大100字符
	if title == "" {
		return nil, fmt.Errorf("消息标题不能为空")
	}
	if len(title) > 100 {
		title = title[:100]
	}

	// 构建请求体
	reqBody := strings.NewReader(fmt.Sprintf("title=%s&desp=%s", title, content))

	// 发送请求
	apiURL := fmt.Sprintf("%s/%s.send", ServerChanAPIURL, sendKey)
	req, err := http.NewRequest(http.MethodPost, apiURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var response ServerChanResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查响应状态
	if response.Code != 0 {
		return &response, fmt.Errorf("发送失败: %s", response.Message)
	}

	return &response, nil
}

// ServerChanSendWithImage 发送带图片的消息
func ServerChanSendWithImage(sendKey, title, content string, imageURL string) (*ServerChanResponse, error) {
	if sendKey == "" {
		return nil, fmt.Errorf("SendKey不能为空")
	}

	// 标题不能为空，最大100字符
	if title == "" {
		return nil, fmt.Errorf("消息标题不能为空")
	}
	if len(title) > 100 {
		title = title[:100]
	}

	// 如果提供了图片URL，则在内容中添加图片
	if imageURL != "" {
		content = fmt.Sprintf("%s\n\n![图片](%s)", content, imageURL)
	}

	// 构建请求体
	reqBody := strings.NewReader(fmt.Sprintf("title=%s&desp=%s", title, content))

	// 发送请求
	apiURL := fmt.Sprintf("%s/%s.send", ServerChanAPIURL, sendKey)
	req, err := http.NewRequest(http.MethodPost, apiURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var response ServerChanResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查响应状态
	if response.Code != 0 {
		return &response, fmt.Errorf("发送失败: %s", response.Message)
	}

	return &response, nil
} 