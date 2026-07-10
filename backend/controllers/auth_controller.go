package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
	"github.com/user/server-ops-backend/utils"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}
	if !checkLoginAllowed(c, req.Username) {
		abortLoginRateLimited(c)
		return
	}

	admin, err := models.GetAdminAccountByUsername(req.Username)
	if err != nil || !admin.CheckPassword(req.Password) {
		recordLoginFailure(c, req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	clearLoginFailures(c, req.Username)

	if err := admin.UpdateLastLogin(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新登录状态失败"})
		return
	}

	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.SessionVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"admin": adminProfilePayload(admin),
	})
}

func GetProfile(c *gin.Context) {
	admin, ok := currentAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, adminProfilePayload(admin))
}

func ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=12"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求数据"})
		return
	}

	admin, ok := currentAdminFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !admin.CheckPassword(req.OldPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码不正确"})
		return
	}

	hash, err := models.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码处理失败"})
		return
	}

	result := models.DB.Model(&models.AdminAccount{}).
		Where("id = ? AND session_version = ?", admin.ID, admin.SessionVersion).
		Updates(map[string]interface{}{
			"password":        hash,
			"session_version": gorm.Expr("session_version + 1"),
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码更新失败"})
		return
	}
	if result.RowsAffected != 1 {
		c.JSON(http.StatusConflict, gin.H{"error": "管理员会话已变更，请重新登录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码已更新"})
}

func currentAdminFromContext(c *gin.Context) (*models.AdminAccount, bool) {
	value, ok := c.Get("adminAccount")
	if !ok {
		return nil, false
	}
	admin, ok := value.(*models.AdminAccount)
	return admin, ok && admin != nil && admin.ID == models.SingletonAdminID
}

func adminProfilePayload(admin *models.AdminAccount) gin.H {
	return gin.H{
		"id":            admin.ID,
		"username":      admin.Username,
		"email":         admin.Email,
		"phone":         admin.Phone,
		"last_login_at": admin.LastLoginAt,
		"last_login":    admin.LastLoginAt,
	}
}
