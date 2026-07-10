package models

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const SingletonAdminID uint = 1

// AdminAccount is the only interactive account supported by the dashboard.
type AdminAccount struct {
	ID             uint      `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Username       string    `gorm:"uniqueIndex;not null" json:"username"`
	Password       string    `gorm:"not null" json:"-"`
	Email          string    `gorm:"index" json:"email"`
	Phone          string    `json:"phone"`
	LastLoginAt    time.Time `json:"last_login_at"`
	SessionVersion uint64    `gorm:"not null;default:1" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func (a *AdminAccount) CheckPassword(password string) bool {
	if a == nil || a.Password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(a.Password), []byte(password)) == nil
}

func GetAdminAccount() (*AdminAccount, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	var admin AdminAccount
	if err := DB.First(&admin, SingletonAdminID).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

func GetAdminAccountByUsername(username string) (*AdminAccount, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	var admin AdminAccount
	err := DB.Where("id = ? AND username = ?", SingletonAdminID, strings.TrimSpace(username)).First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

func (a *AdminAccount) UpdateLastLogin() error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if a == nil || a.ID != SingletonAdminID {
		return errors.New("invalid administrator account")
	}

	a.LastLoginAt = time.Now()
	return DB.Model(&AdminAccount{}).
		Where("id = ?", SingletonAdminID).
		Update("last_login_at", a.LastLoginAt).Error
}

func GetAdminNotificationEmails() ([]string, error) {
	admin, err := GetAdminAccount()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	email := strings.ToLower(strings.TrimSpace(admin.Email))
	if email == "" {
		return []string{}, nil
	}
	return []string{email}, nil
}
