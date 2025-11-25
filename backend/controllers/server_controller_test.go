package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/server-ops-backend/models"
)

func TestCreateServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name: "成功创建服务器",
			requestBody: map[string]interface{}{
				"name":  "Test Server",
				"notes": "Test Description",
				"tags":  "test,server",
			},
			expectedStatus: http.StatusCreated,
			setupMocks: func() {
				models.CreateServer = func(server *models.Server) error {
					server.ID = 1
					return nil
				}
			},
		},
		{
			name: "服务器名称为空",
			requestBody: map[string]interface{}{
				"name":  "",
				"notes": "Test Description",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "服务器名称不能为空",
			setupMocks:     func() {},
		},
		{
			name: "无效的请求数据",
			requestBody: map[string]interface{}{
				"invalid_field": "test",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "服务器名称不能为空",
			setupMocks:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 调用创建服务器函数
			CreateServer(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.Equal(t, "服务器创建成功", response["message"])
				assert.NotEmpty(t, response["server"])
			}
		})
	}
}

func TestGetAllServers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		expectedStatus int
		expectedError  string
		setupMocks     func()
		expectedCount  int
	}{
		{
			name:           "成功获取服务器列表",
			expectedStatus: http.StatusOK,
			expectedCount:  2,
			setupMocks: func() {
				models.GetAllServers = func(limit int) ([]models.Server, error) {
					return []models.Server{
						{ID: 1, Name: "Server 1"},
						{ID: 2, Name: "Server 2"},
					}, nil
				}
			},
		},
		{
			name:           "获取服务器列表失败",
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "获取服务器列表失败",
			setupMocks: func() {
				models.GetAllServers = func(limit int) ([]models.Server, error) {
					return nil, assert.AnError
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			req := httptest.NewRequest(http.MethodGet, "/servers", nil)
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 调用获取所有服务器函数
			GetAllServers(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				servers := response["servers"].([]interface{})
				assert.Len(t, servers, tt.expectedCount)
			}
		})
	}
}

func TestGetServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		serverID       string
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name:           "成功获取服务器",
			serverID:       "1",
			expectedStatus: http.StatusOK,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:   1,
						Name: "Test Server",
					}, nil
				}
			},
		},
		{
			name:           "无效的服务器ID",
			serverID:       "invalid",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的服务器ID",
			setupMocks:     func() {},
		},
		{
			name:           "服务器不存在",
			serverID:       "999",
			expectedStatus: http.StatusNotFound,
			expectedError:  "服务器不存在",
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return nil, assert.AnError
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			req := httptest.NewRequest(http.MethodGet, "/servers/"+tt.serverID, nil)
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "id", Value: tt.serverID}}

			// 调用获取服务器函数
			GetServer(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.NotEmpty(t, response["server"])
			}
		})
	}
}

func TestGetServerStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		serverID       string
		expectedStatus int
		expectedOnline bool
		setupMocks     func()
	}{
		{
			name:           "在线服务器状态",
			serverID:       "1",
			expectedStatus: http.StatusOK,
			expectedOnline: true,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:            1,
						Name:          "Test Server",
						Online:        true,
						Status:        "online",
						LastHeartbeat: time.Now(),
					}, nil
				}
			},
		},
		{
			name:           "离线服务器状态",
			serverID:       "1",
			expectedStatus: http.StatusOK,
			expectedOnline: false,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:            1,
						Name:          "Test Server",
						Online:        false,
						Status:        "offline",
						LastHeartbeat: time.Now().Add(-30 * time.Second),
					}, nil
				}
			},
		},
		{
			name:           "服务器不存在",
			serverID:       "999",
			expectedStatus: http.StatusNotFound,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return nil, assert.AnError
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			req := httptest.NewRequest(http.MethodGet, "/servers/"+tt.serverID+"/status", nil)
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "id", Value: tt.serverID}}

			// 调用获取服务器状态函数
			GetServerStatus(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, true, response["success"])
				assert.Equal(t, tt.expectedOnline, response["online"])
			}
		})
	}
}

func TestHeartbeat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		serverID       string
		secretKey      string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name:      "成功心跳",
			serverID:  "1",
			secretKey: "valid-secret",
			requestBody: map[string]interface{}{
				"status":    "online",
				"timestamp": time.Now().Unix(),
				"version":   "1.0.0",
			},
			expectedStatus: http.StatusOK,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:        1,
						SecretKey: "valid-secret",
					}, nil
				}
				models.UpdateServerHeartbeatAndStatus = func(id uint, status string) error {
					return nil
				}
				models.UpdateServerAgentVersion = func(id uint, version string) error {
					return nil
				}
			},
		},
		{
			name:           "无效的服务器ID",
			serverID:       "invalid",
			secretKey:      "valid-secret",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的服务器ID",
			setupMocks:     func() {},
		},
		{
			name:           "未提供密钥",
			serverID:       "1",
			secretKey:      "",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "未提供密钥",
			setupMocks:     func() {},
		},
		{
			name:           "密钥无效",
			serverID:       "1",
			secretKey:      "invalid-secret",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "密钥无效",
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:        1,
						SecretKey: "valid-secret",
					}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			var body []byte
			if tt.requestBody != nil {
				body, _ = json.Marshal(tt.requestBody)
			}
			req := httptest.NewRequest(http.MethodPost, "/servers/"+tt.serverID+"/heartbeat", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.secretKey != "" {
				req.Header.Set("X-Secret-Key", tt.secretKey)
			}

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "id", Value: tt.serverID}}

			// 调用心跳函数
			Heartbeat(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.Equal(t, "心跳接收成功", response["message"])
				assert.NotEmpty(t, response["serverTime"])
			}
		})
	}
}

func TestReportMonitorData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		serverID       string
		secretKey      string
		requestBody    map[string]interface{}
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name:      "成功上报监控数据",
			serverID:  "1",
			secretKey: "valid-secret",
			requestBody: map[string]interface{}{
				"cpu_usage":    25.5,
				"memory_used":  1073741824,  // 1GB
				"memory_total": 4294967296,  // 4GB
				"disk_used":    10737418240, // 10GB
				"disk_total":   107374182400, // 100GB
				"network_in":   1024.0,
				"network_out":  2048.0,
				"load_avg_1":   1.5,
				"load_avg_5":   1.2,
				"load_avg_15":  0.8,
			},
			expectedStatus: http.StatusOK,
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:        1,
						SecretKey: "valid-secret",
					}, nil
				}
				models.AddMonitorData = func(data *models.ServerMonitor) error {
					return nil
				}
				models.UpdateServerHeartbeatAndStatus = func(id uint, status string) error {
					return nil
				}
			},
		},
		{
			name:           "无效的服务器ID",
			serverID:       "invalid",
			secretKey:      "valid-secret",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的服务器ID",
			setupMocks:     func() {},
		},
		{
			name:           "无效的密钥",
			serverID:       "1",
			secretKey:      "invalid-secret",
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "无效的密钥",
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:        1,
						SecretKey: "valid-secret",
					}, nil
				}
			},
		},
		{
			name:      "无效的监控数据",
			serverID:  "1",
			secretKey: "valid-secret",
			requestBody: map[string]interface{}{
				"invalid_field": "test",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的监控数据",
			setupMocks: func() {
				models.GetServerByID = func(id uint) (*models.Server, error) {
					return &models.Server{
						ID:        1,
						SecretKey: "valid-secret",
					}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/servers/"+tt.serverID+"/monitor", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Secret-Key", tt.secretKey)

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = []gin.Param{{Key: "id", Value: tt.serverID}}

			// 调用上报监控数据函数
			ReportMonitorData(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.Equal(t, "监控数据上报成功", response["message"])
			}
		})
	}
}

