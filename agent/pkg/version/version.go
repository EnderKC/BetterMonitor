package version

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/joho/godotenv"
)

var (
	// Version 当前版本号（从环境变量读取）
	Version = ""
	// BuildDate 构建日期（自动生成）
	BuildDate = time.Now().Format("2006-01-02")
)

func init() {
	// 尝试从项目根目录的.env文件加载版本信息
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	// 从环境变量读取版本信息
	if envVersion := os.Getenv("VERSION"); envVersion != "" {
		Version = envVersion
	} else {
		// 如果没有找到版本信息，使用unknown作为默认值
		Version = "unknown"
	}
}

// Info 版本信息结构
type Info struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
	Arch      string `json:"arch"`
}

// GetVersion 获取版本信息
func GetVersion() *Info {
	return &Info{
		Version:   Version,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// GetVersionString 获取版本字符串
func GetVersionString() string {
	return fmt.Sprintf("Better-Monitor Agent v%s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH)
}
