package controllers

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/user/server-ops-backend/models"
)

// UpdateProfileRequest 允许用户更新的字段（白名单）
// 使用指针用于区分"未传入"与"传入空字符串"
type UpdateProfileRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

var (
	// 约束：3~32位，允许字母/数字/下划线/点/中划线，首字符必须是字母或数字
	usernameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)
	// 电话：基础字符集与长度校验
	phoneRE = regexp.MustCompile(`^[0-9+()\ -]{5,32}$`)
)

// UpdateProfile 更新当前登录用户的资料信息
func UpdateProfile(c *gin.Context) {
	userID, ok := currentUserIDFromContext(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	db := models.DB
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var (
		updatedUser          models.User
		refreshTokenRequired bool
	)

	err := db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{}

		// username
		if req.Username != nil {
			username := strings.TrimSpace(*req.Username)
			if err := validateUsername(username); err != nil {
				return err
			}
			if username != user.Username {
				var cnt int64
				if err := tx.Model(&models.User{}).
					Where("username = ? AND id <> ?", username, userID).
					Count(&cnt).Error; err != nil {
					return err
				}
				if cnt > 0 {
					return errConflict("username already exists")
				}
				updates["username"] = username
				refreshTokenRequired = true
			}
		}

		// email（存储为小写）
		if req.Email != nil {
			email, err := normalizeAndValidateEmail(strings.TrimSpace(*req.Email))
			if err != nil {
				return err
			}
			if email != "" && strings.ToLower(email) != strings.ToLower(user.Email) {
				var cnt int64
				if err := tx.Model(&models.User{}).
					Where("LOWER(email) = LOWER(?) AND id <> ?", email, userID).
					Count(&cnt).Error; err != nil {
					return err
				}
				if cnt > 0 {
					return errConflict("email already exists")
				}
				updates["email"] = strings.ToLower(email)
			}
			if email == "" && user.Email != "" {
				updates["email"] = ""
			}
		}

		// phone
		if req.Phone != nil {
			phone := strings.TrimSpace(*req.Phone)
			if err := validatePhone(phone); err != nil {
				return err
			}
			if phone != user.Phone {
				updates["phone"] = phone
			}
		}

		// no-op：直接返回现有数据
		if len(updates) == 0 {
			updatedUser = user
			return nil
		}

		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			// 双保险：即使应用层已检查，仍可能存在并发更新导致的唯一性冲突
			if isUniqueConstraintError(err) {
				return errConflict("unique constraint violation")
			}
			return err
		}

		if err := tx.First(&updatedUser, userID).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case isConflictError(err):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":       updatedUser.ID,
			"username": updatedUser.Username,
			"email":    updatedUser.Email,
			"phone":    updatedUser.Phone,
			"role":     updatedUser.Role,
		},
		"refresh_token_required": refreshTokenRequired,
	})
}

func validateUsername(username string) error {
	if username == "" {
		return errBadRequest("username is required")
	}
	if !usernameRE.MatchString(username) {
		return errBadRequest("invalid username format")
	}
	return nil
}

func normalizeAndValidateEmail(email string) (string, error) {
	if email == "" {
		return "", nil
	}
	if len(email) > 254 {
		return "", errBadRequest("email too long")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", errBadRequest("invalid email format")
	}
	return email, nil
}

func validatePhone(phone string) error {
	if phone == "" {
		return nil
	}
	if !phoneRE.MatchString(phone) {
		return errBadRequest("invalid phone format")
	}
	return nil
}

func currentUserIDFromContext(c *gin.Context) (uint, bool) {
	// 常见 key 兼容
	for _, key := range []string{"user_id", "userID", "uid", "id"} {
		if v, ok := c.Get(key); ok {
			if id, ok := toUint(v); ok {
				return id, true
			}
		}
	}
	// 兼容：直接把用户对象塞进 context
	if v, ok := c.Get("user"); ok {
		switch u := v.(type) {
		case models.User:
			if u.ID != 0 {
				return u.ID, true
			}
		case *models.User:
			if u != nil && u.ID != 0 {
				return u.ID, true
			}
		}
	}
	return 0, false
}

func toUint(v interface{}) (uint, bool) {
	switch t := v.(type) {
	case uint:
		return t, true
	case uint64:
		if t > 0 {
			return uint(t), true
		}
		return 0, true
	case int:
		if t > 0 {
			return uint(t), true
		}
		return 0, true
	case int64:
		if t > 0 {
			return uint(t), true
		}
		return 0, true
	case float64:
		// gin/jwt 某些场景会把数字解到 float64
		if t > 0 {
			return uint(t), true
		}
		return 0, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		u64, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(u64), true
	default:
		return 0, false
	}
}

type httpError struct {
	kind string
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func errBadRequest(msg string) error { return &httpError{kind: "bad_request", msg: msg} }
func errConflict(msg string) error   { return &httpError{kind: "conflict", msg: msg} }

func isConflictError(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.kind == "conflict"
	}
	return false
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// SQLite / Postgres / MySQL 常见报错关键词
	if strings.Contains(s, "unique constraint") {
		return true
	}
	if strings.Contains(s, "duplicate entry") {
		return true
	}
	if strings.Contains(s, "duplicate key value") {
		return true
	}
	return false
}
