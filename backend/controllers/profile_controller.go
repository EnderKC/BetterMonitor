package controllers

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
	"gorm.io/gorm"
)

type UpdateProfileRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
}

var phoneRE = regexp.MustCompile(`^[0-9+()\ -]{5,32}$`)

var errAdminSessionChanged = errors.New("administrator session changed")

func UpdateProfile(c *gin.Context) {
	currentAdmin, ok := currentAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var updatedAdmin models.AdminAccount
	usernameChanged := false
	err := models.DB.Transaction(func(tx *gorm.DB) error {
		var admin models.AdminAccount
		if err := tx.First(&admin, models.SingletonAdminID).Error; err != nil {
			return err
		}
		if admin.ID != currentAdmin.ID ||
			admin.SessionVersion != currentAdmin.SessionVersion ||
			admin.Username != currentAdmin.Username {
			return errAdminSessionChanged
		}

		updates := map[string]interface{}{}
		if req.Username != nil {
			username := strings.TrimSpace(*req.Username)
			if err := models.ValidateAdminUsername(username); err != nil {
				return err
			}
			if username != admin.Username {
				updates["username"] = username
				usernameChanged = true
			}
		}
		if req.Email != nil {
			email, err := normalizeAndValidateAdminEmail(*req.Email)
			if err != nil {
				return err
			}
			if email != admin.Email {
				updates["email"] = email
			}
		}
		if req.Phone != nil {
			phone := strings.TrimSpace(*req.Phone)
			if err := validateAdminPhone(phone); err != nil {
				return err
			}
			if phone != admin.Phone {
				updates["phone"] = phone
			}
		}

		if len(updates) > 0 {
			result := tx.Model(&models.AdminAccount{}).
				Where(
					"id = ? AND session_version = ? AND username = ?",
					currentAdmin.ID,
					currentAdmin.SessionVersion,
					currentAdmin.Username,
				).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errAdminSessionChanged
			}
			return tx.First(&updatedAdmin, models.SingletonAdminID).Error
		}

		updatedAdmin = admin
		return nil
	})
	if err != nil {
		if errors.Is(err, errAdminSessionChanged) {
			c.JSON(http.StatusConflict, gin.H{"error": "管理员会话已变更，请重新登录"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{"data": adminProfilePayload(&updatedAdmin)}
	if usernameChanged || currentAdmin.Username != updatedAdmin.Username {
		token, err := utils.GenerateToken(updatedAdmin.ID, updatedAdmin.Username, updatedAdmin.SessionVersion)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh administrator token"})
			return
		}
		response["token"] = token
	}
	c.JSON(http.StatusOK, response)
}

func normalizeAndValidateAdminEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", nil
	}
	if len(email) > 254 {
		return "", &profileValidationError{"email too long"}
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address == "" {
		return "", &profileValidationError{"invalid email format"}
	}
	return strings.ToLower(parsed.Address), nil
}

func validateAdminPhone(phone string) error {
	if phone == "" {
		return nil
	}
	if !phoneRE.MatchString(phone) {
		return &profileValidationError{"invalid phone format"}
	}
	return nil
}

type profileValidationError struct{ message string }

func (e *profileValidationError) Error() string { return e.message }
