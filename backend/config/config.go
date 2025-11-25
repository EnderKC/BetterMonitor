package config

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Config 保存应用程序配置
type Config struct {
	Port            string
	DBPath          string
	JWTSecret       string
	TokenExpiration int
}

// LoadConfig 加载配置
func LoadConfig() *Config {
	// 尝试加载.env文件
	if err := godotenv.Load(); err != nil {
		log.Println("未找到.env文件，使用默认配置或环境变量")
	}

	// 设置默认值或从环境变量获取
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "./data.db")
	jwtSecret := getEnv("JWT_SECRET", "your_secret_key_change_this_in_production")

	return &Config{
		Port:            port,
		DBPath:          dbPath,
		JWTSecret:       jwtSecret,
		TokenExpiration: 24, // 默认24小时
	}
}

// CorsMiddleware 配置CORS中间件
func CorsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	})
}

// 辅助函数从环境变量获取值，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
