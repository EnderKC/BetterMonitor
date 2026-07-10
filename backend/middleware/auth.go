package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/utils"
)

func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无授权信息"})
			c.Abort()
			return
		}

		claims, admin, err := utils.ValidateAdminToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的管理员会话"})
			c.Abort()
			return
		}

		c.Set("adminId", claims.AdminID)
		c.Set("adminUsername", claims.Username)
		c.Set("adminAccount", admin)
		c.Next()
	}
}
