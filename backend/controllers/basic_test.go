package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 调用健康检查函数
	HealthCheck(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.NotEmpty(t, response["timestamp"])
	assert.NotEmpty(t, response["uptime"])
}

func TestLogin_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建无效JSON的测试请求
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 调用登录函数
	Login(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "无效的请求数据", response["error"])
}

func TestLogin_EmptyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建空字段的测试请求
	loginData := map[string]interface{}{
		"username": "",
		"password": "testpass",
	}
	body, _ := json.Marshal(loginData)
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 调用登录函数
	Login(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "无效的请求数据", response["error"])
}

func TestLogin_RateLimitAfterFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevDB := models.DB
	defer func() { models.DB = prevDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.AdminAccount{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	models.DB = db
	loginAttempts = make(map[string]*loginAttempt)

	hash, err := models.HashPassword("correct")
	if err != nil {
		t.Fatalf("生成测试密码失败: %v", err)
	}
	admin := models.AdminAccount{ID: models.SingletonAdminID, Username: "limited", Password: hash, SessionVersion: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	for i := 0; i < loginMaxFailures; i++ {
		body, _ := json.Marshal(map[string]interface{}{
			"username": "limited",
			"password": "wrong",
		})
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		Login(c)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"username": "limited",
		"password": "correct",
	})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	Login(c)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestGetProfile_BasicUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// GetProfile 从认证中间件写入的单一管理员上下文读取资料
	prevDB := models.DB
	defer func() { models.DB = prevDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.AdminAccount{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	models.DB = db

	admin := models.AdminAccount{ID: models.SingletonAdminID, Username: "testuser", Password: "x", SessionVersion: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("adminAccount", &admin)

	// 调用获取用户资料函数
	GetProfile(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, float64(admin.ID), response["id"])
	assert.Equal(t, "testuser", response["username"])
	assert.NotContains(t, response, "role")
}

func TestCreateServer_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建无效JSON的测试请求
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 调用创建服务器函数
	CreateServer(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "无效的请求数据", response["error"])
}

func TestCreateServer_EmptyName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建空名称的测试请求
	serverData := map[string]interface{}{
		"name":  "",
		"notes": "Test Description",
	}
	body, _ := json.Marshal(serverData)
	req := httptest.NewRequest(http.MethodPost, "/servers", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 调用创建服务器函数
	CreateServer(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "服务器名称不能为空", response["error"])
}

func TestGetServer_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/servers/invalid", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "invalid"}}

	// 调用获取服务器函数
	GetServer(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "无效的服务器ID", response["error"])
}

func TestGetServerStatus_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/servers/invalid/status", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "invalid"}}

	// 调用获取服务器状态函数
	GetServerStatus(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, false, response["success"])
	assert.Equal(t, "无效的服务器ID", response["error"])
}

func TestGetServerStatus_PublicDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevDB := models.DB
	defer func() { models.DB = prevDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.Server{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	models.DB = db

	server := models.Server{Name: "private", AllowPublicView: false}
	if err := db.Create(&server).Error; err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/1/status", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "1"}}

	GetServerStatus(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReportMonitorData_InvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodPost, "/servers/invalid/monitor", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = []gin.Param{{Key: "id", Value: "invalid"}}

	// 调用上报监控数据函数
	ReportMonitorData(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, "无效的服务器ID", response["error"])
}

func TestStructureValidation(t *testing.T) {
	// 测试登录请求结构
	loginReq := LoginRequest{
		Username: "testuser",
		Password: "testpass",
	}
	assert.Equal(t, "testuser", loginReq.Username)
	assert.Equal(t, "testpass", loginReq.Password)

}

func TestResponseTimeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 测试健康检查响应时间
	start := time.Now()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	HealthCheck(c)

	duration := time.Since(start)

	// 验证响应时间在合理范围内（应该小于1秒）
	assert.Less(t, duration, time.Second)
	assert.Equal(t, http.StatusOK, w.Code)
}
