package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// Config 保存应用程序配置
type Config struct {
	Port                             string
	DBPath                           string
	JWTSecret                        string
	TokenExpiration                  int
	CORSAllowOrigins                 []string
	AdminUsername                    string
	AdminPassword                    string
	AdminPasswordFile                string
	TrustedProxies                   []string
	LifeIngestMasterKey              string
	LifeIngestMasterKeyFile          string
	LifeIngestMaxBodyBytes           int64
	LifeIngestClockSkewSeconds       int
	LifeIngestIPRequestsPerMinute    int
	LifeIngestProbeRequestsPerMinute int
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
		corsAllowOrigins := parseCSVEnv("CORS_ALLOW_ORIGINS")
		adminPasswordFile := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_FILE"))
		if adminPasswordFile == "" {
			adminPasswordFile = filepath.Join(filepath.Dir(dbPath), "admin-password")
		}
		lifeIngestMasterKeyFile := strings.TrimSpace(os.Getenv("LIFE_INGEST_MASTER_KEY_FILE"))
		if lifeIngestMasterKeyFile == "" {
			lifeIngestMasterKeyFile = filepath.Join(filepath.Dir(dbPath), "life-ingest-master-key")
		}

		// 如果没有设置JWT_SECRET，自动生成一个随机密钥
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = generateRandomSecret()
			log.Println("未设置JWT_SECRET环境变量，已自动生成随机密钥")
			log.Printf("警告: 使用随机生成的JWT密钥，重启后所有token将失效")
		}

		instance = &Config{
			Port:                             port,
			DBPath:                           dbPath,
			JWTSecret:                        jwtSecret,
			TokenExpiration:                  24, // 默认24小时
			CORSAllowOrigins:                 corsAllowOrigins,
			AdminUsername:                    getEnv("ADMIN_USERNAME", "admin"),
			AdminPassword:                    os.Getenv("ADMIN_PASSWORD"),
			AdminPasswordFile:                adminPasswordFile,
			TrustedProxies:                   parseCSVEnv("TRUSTED_PROXIES"),
			LifeIngestMasterKey:              strings.TrimSpace(os.Getenv("LIFE_INGEST_MASTER_KEY")),
			LifeIngestMasterKeyFile:          lifeIngestMasterKeyFile,
			LifeIngestMaxBodyBytes:           getInt64Env("LIFE_INGEST_MAX_BODY_BYTES", 4<<20),
			LifeIngestClockSkewSeconds:       getIntEnv("LIFE_INGEST_CLOCK_SKEW_SECONDS", 300),
			LifeIngestIPRequestsPerMinute:    getIntEnv("LIFE_INGEST_IP_REQUESTS_PER_MINUTE", 240),
			LifeIngestProbeRequestsPerMinute: getIntEnv("LIFE_INGEST_PROBE_REQUESTS_PER_MINUTE", 120),
		}
	})

	return instance
}

func ConfigureTrustedProxies(engine *gin.Engine, proxies []string) error {
	if engine == nil {
		return errors.New("gin engine is required")
	}
	if err := engine.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}
	return nil
}

// CorsMiddleware 配置CORS中间件
func CorsMiddleware() gin.HandlerFunc {
	cfg := LoadConfig()
	allowOrigins := cfg.CORSAllowOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"*"}
	}
	return cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Secret-Key", "X-Register-Token", "X-Chunk-Hash", "X-Chunk-Compressed"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: len(cfg.CORSAllowOrigins) > 0,
	})
}

// HasAllowedOrigins 是否配置了 CORS 白名单
func HasAllowedOrigins() bool {
	return len(LoadConfig().CORSAllowOrigins) > 0
}

func IsAllowedOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	allowed := LoadConfig().CORSAllowOrigins
	for _, item := range allowed {
		if item == "*" || strings.EqualFold(item, origin) {
			return true
		}
	}
	return false
}

// 辅助函数从环境变量获取值，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func parseCSVEnv(key string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func getIntEnv(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("配置 %s 无效，使用默认值 %d", key, defaultValue)
		return defaultValue
	}
	return value
}

func getInt64Env(key string, defaultValue int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		log.Printf("配置 %s 无效，使用默认值 %d", key, defaultValue)
		return defaultValue
	}
	return value
}
