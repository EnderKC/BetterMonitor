package config

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"sync"

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

var (
	instance *Config
	once     sync.Once
)

// generateRandomSecret 生成随机的JWT密钥
func generateRandomSecret() string {
	bytes := make([]byte, 32) // 256位密钥
	if _, err := rand.Read(bytes); err != nil {
		log.Fatalf("生成随机密钥失败: %v", err)
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// LoadConfig 加载配置（单例模式）
func LoadConfig() *Config {
	once.Do(func() {
		// 尝试加载.env文件
		if err := godotenv.Load(); err != nil {
			log.Println("未找到.env文件，使用默认配置或环境变量")
		}

		// 设置默认值或从环境变量获取
		port := getEnv("PORT", "8085")
		dbPath := getEnv("DB_PATH", "./data/data.db")

		// 如果没有设置JWT_SECRET，自动生成一个随机密钥
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = generateRandomSecret()
			log.Println("未设置JWT_SECRET环境变量，已自动生成随机密钥")
			log.Printf("警告: 使用随机生成的JWT密钥，重启后所有token将失效")
		}

		instance = &Config{
			Port:            port,
			DBPath:          dbPath,
			JWTSecret:       jwtSecret,
			TokenExpiration: 24, // 默认24小时
		}
	})

	return instance
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
