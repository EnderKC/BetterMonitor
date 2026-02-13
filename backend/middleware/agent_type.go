package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/user/server-ops-backend/models"
)

// MonitorOnlyGuard 拦截监控模式服务器的操作类请求。
// 当 Server.AgentType == "monitor" 时，该中间件返回 403 Forbidden。
// 仅适用于挂载在 /servers/:id 下的操作路由组（terminal、file、process、docker、nginx 等）。
func MonitorOnlyGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		if idStr == "" {
			c.Next()
			return
		}

		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			// ID 解析失败交给后续 handler 处理
			c.Next()
			return
		}

		server, err := models.GetServerByID(uint(id))
		if err != nil {
			// 服务器不存在交给后续 handler 处理
			c.Next()
			return
		}

		if server.AgentType == "monitor" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "该服务器为监控模式，不支持此操作",
			})
			return
		}

		c.Next()
	}
}
