package version

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var (
	// Version 当前版本号（从环境变量或编译时注入）
	Version = ""
	// Commit Git提交哈希
	Commit = ""
	// BuildDate 构建日期
	BuildDate = ""
)

func init() {
	// 尝试从项目根目录的.env文件加载版本信息
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	// 从环境变量读取版本信息（仅在编译期未注入时生效）
	// 生产环境优先使用编译期 -ldflags 注入的值，避免被容器环境变量意外覆盖
	if Version == "" || strings.EqualFold(Version, "latest") || strings.EqualFold(Version, "unknown") {
		if envVersion := strings.TrimSpace(os.Getenv("DASHBOARD_VERSION")); envVersion != "" &&
			!strings.EqualFold(envVersion, "latest") &&
			!strings.EqualFold(envVersion, "unknown") {
			Version = envVersion
		} else if envVersion := strings.TrimSpace(os.Getenv("VERSION")); envVersion != "" &&
			!strings.EqualFold(envVersion, "latest") &&
			!strings.EqualFold(envVersion, "unknown") {
			Version = envVersion
		}
	}

	// 从环境变量读取 commit hash（仅在编译期未注入时生效）
	if Commit == "" || strings.EqualFold(Commit, "unknown") {
		if envCommit := strings.TrimSpace(os.Getenv("COMMIT")); envCommit != "" &&
			!strings.EqualFold(envCommit, "unknown") {
			Commit = envCommit
		}
	}

	// 从环境变量读取构建时间（仅在编译期未注入时生效）
	// 兼容 BUILD_DATE 和 BUILD_TIME 两种环境变量名
	if BuildDate == "" || strings.EqualFold(BuildDate, "unknown") {
		if envBuildDate := strings.TrimSpace(os.Getenv("BUILD_DATE")); envBuildDate != "" &&
			!strings.EqualFold(envBuildDate, "unknown") {
			BuildDate = envBuildDate
		} else if envBuildTime := strings.TrimSpace(os.Getenv("BUILD_TIME")); envBuildTime != "" &&
			!strings.EqualFold(envBuildTime, "unknown") {
			BuildDate = envBuildTime
		}
	}

	// 如果构建时间为空，使用当前时间作为默认值
	if BuildDate == "" {
		BuildDate = time.Now().Format("2006-01-02T15:04:05Z")
	}

	// 避免显示 "latest" 作为版本号
	if Version == "" || strings.EqualFold(Version, "latest") || strings.EqualFold(Version, "unknown") {
		// 如果有 commit hash，使用它作为版本标识
		if Commit != "" && !strings.EqualFold(Commit, "unknown") {
			Version = "dev-" + Commit
		} else {
			Version = "dev"
		}
	}

	// 如果 commit 为空，设置为 unknown
	if Commit == "" {
		Commit = "unknown"
	}
}

// Info 版本信息结构
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
	Arch      string `json:"arch"`
}

// GetVersion 获取版本信息
func GetVersion() *Info {
	return &Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// GetVersionString 获取版本字符串
func GetVersionString() string {
	return fmt.Sprintf("Better-Monitor Dashboard v%s (commit: %s, %s/%s)",
		Version, Commit, runtime.GOOS, runtime.GOARCH)
}
