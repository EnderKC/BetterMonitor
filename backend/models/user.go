package models

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	gorm.Model
	Username    string    `gorm:"uniqueIndex;not null" json:"username"`
	Password    string    `gorm:"not null" json:"-"` // json:"-" 确保密码不会在JSON响应中暴露
	Email       string    `gorm:"index" json:"email"`
	Phone       string    `json:"phone"`
	Role        string    `gorm:"default:user" json:"role"`
	LastLoginAt time.Time `json:"last_login_at"`
}

// HashPassword 对密码进行哈希处理
func HashPassword(password string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err) // 在实际应用中应更优雅地处理错误
	}
	return string(hashedPassword)
}

// CheckPassword 验证用户密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// GetUserByUsername 根据用户名查找用户
func GetUserByUsername(username string) (*User, error) {
	var user User
	result := DB.Where("username = ?", username).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, result.Error
	}
	return &user, nil
}

// CreateUser 创建新用户
func CreateUser(username, password, role string) (*User, error) {
	// 检查用户名是否已存在
	var count int64
	DB.Model(&User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, errors.New("用户名已存在")
	}

	// 创建新用户
	user := User{
		Username: username,
		Password: HashPassword(password),
		Role:     role,
	}

	if err := DB.Create(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// UpdateLastLogin 更新用户最后登录时间
func (u *User) UpdateLastLogin() error {
	u.LastLoginAt = time.Now()
	return DB.Model(u).Update("last_login_at", u.LastLoginAt).Error
}

// GetAdminEmails 获取管理员邮箱列表（去重、小写、过滤空值）
func GetAdminEmails() ([]string, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	type row struct {
		Email string
	}

	var rows []row
	if err := DB.Model(&User{}).
		Select("email").
		Where("role = ? AND TRIM(email) <> ''", "admin").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(rows))
	emails := make([]string, 0, len(rows))
	for _, r := range rows {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	return emails, nil
}
