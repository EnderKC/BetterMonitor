package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
	"gorm.io/gorm"
)

// MockUser 模拟用户结构
type MockUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (m *MockUser) CheckPassword(password string) bool {
	return m.Password == password
}

func (m *MockUser) UpdateLastLogin() {
	// 模拟更新最后登录时间
}

func TestLogin(t *testing.T) {
	// 设置Gin为测试模式
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    LoginRequest
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name: "成功登录",
			requestBody: LoginRequest{
				Username: "testuser",
				Password: "testpass",
			},
			expectedStatus: http.StatusOK,
			setupMocks: func() {
				// 模拟用户查找成功
				models.GetUserByUsername = func(username string) (*models.User, error) {
					return &models.User{
						ID:       1,
						Username: "testuser",
						Password: "testpass",
						Role:     "admin",
					}, nil
				}
				
				// 模拟生成token成功
				utils.GenerateToken = func(userID uint, username, role string) (string, error) {
					return "test-token", nil
				}
			},
		},
		{
			name: "用户名或密码错误",
			requestBody: LoginRequest{
				Username: "testuser",
				Password: "wrongpass",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "用户名或密码错误",
			setupMocks: func() {
				models.GetUserByUsername = func(username string) (*models.User, error) {
					return &models.User{
						ID:       1,
						Username: "testuser",
						Password: "testpass",
						Role:     "admin",
					}, nil
				}
			},
		},
		{
			name: "无效的请求数据",
			requestBody: LoginRequest{
				Username: "",
				Password: "testpass",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的请求数据",
			setupMocks:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 调用登录函数
			Login(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.NotEmpty(t, response["token"])
				assert.NotEmpty(t, response["user"])
			}
		})
	}
}

func TestRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    RegisterRequest
		userRole       string
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name: "成功注册",
			requestBody: RegisterRequest{
				Username: "newuser",
				Password: "newpass",
				Role:     "user",
			},
			userRole:       "admin",
			expectedStatus: http.StatusCreated,
			setupMocks: func() {
				models.CreateUser = func(username, password, role string) (*models.User, error) {
					return &models.User{
						ID:       2,
						Username: username,
						Role:     role,
					}, nil
				}
			},
		},
		{
			name: "非管理员无权限",
			requestBody: RegisterRequest{
				Username: "newuser",
				Password: "newpass",
				Role:     "user",
			},
			userRole:       "user",
			expectedStatus: http.StatusForbidden,
			expectedError:  "只有管理员可以创建新用户",
			setupMocks:     func() {},
		},
		{
			name: "无效的请求数据",
			requestBody: RegisterRequest{
				Username: "",
				Password: "newpass",
				Role:     "user",
			},
			userRole:       "admin",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的请求数据",
			setupMocks:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Set("role", tt.userRole)

			// 调用注册函数
			Register(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.Equal(t, "用户创建成功", response["message"])
				assert.NotEmpty(t, response["user"])
			}
		})
	}
}

func TestGetProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 创建测试请求
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	w := httptest.NewRecorder()

	// 创建Gin上下文
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("userId", uint(1))
	c.Set("username", "testuser")
	c.Set("role", "admin")

	// 调用获取用户资料函数
	GetProfile(c)

	// 验证响应状态码
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证响应内容
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, float64(1), response["id"])
	assert.Equal(t, "testuser", response["username"])
	assert.Equal(t, "admin", response["role"])
}

func TestChangePassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    map[string]string
		expectedStatus int
		expectedError  string
		setupMocks     func()
	}{
		{
			name: "成功修改密码",
			requestBody: map[string]string{
				"old_password": "oldpass",
				"new_password": "newpass123",
			},
			expectedStatus: http.StatusOK,
			setupMocks: func() {
				models.DB = &MockDB{
					user: &models.User{
						Model: gorm.Model{ID: 1},
						Username: "testuser",
						Password: models.HashPassword("oldpass"),
					},
				}
			},
		},
		{
			name: "旧密码错误",
			requestBody: map[string]string{
				"old_password": "wrongpass",
				"new_password": "newpass123",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "旧密码不正确",
			setupMocks: func() {
				models.DB = &MockDB{
					user: &models.User{
						Model: gorm.Model{ID: 1},
						Username: "testuser",
						Password: models.HashPassword("oldpass"),
					},
				}
			},
		},
		{
			name: "新密码太短",
			requestBody: map[string]string{
				"old_password": "oldpass",
				"new_password": "123",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "无效的请求数据",
			setupMocks:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置模拟
			tt.setupMocks()

			// 创建测试请求
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/change-password", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 创建Gin上下文
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Set("userId", uint(1))

			// 调用修改密码函数
			ChangePassword(c)

			// 验证响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code)

			// 验证响应内容
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, response["error"])
			} else {
				assert.Equal(t, "密码已更新", response["message"])
			}
		})
	}
}

// MockDB 用于模拟数据库操作
type MockDB struct {
	user *models.User
	err  error
}

func (m *MockDB) First(dest interface{}, conds ...interface{}) *MockDB {
	if m.err != nil {
		return m
	}
	if user, ok := dest.(*models.User); ok && m.user != nil {
		*user = *m.user
	}
	return m
}

func (m *MockDB) Save(value interface{}) *MockDB {
	return m
}

func (m *MockDB) Error() error {
	return m.err
}