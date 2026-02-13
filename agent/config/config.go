package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config 存储agent的配置项
type Config struct {
	// 服务器信息
	ServerURL     string `mapstructure:"server_url"`
	ServerID      uint   `mapstructure:"server_id"`
	SecretKey     string `mapstructure:"secret_key"`
	RegisterToken string `mapstructure:"register_token"`

	// Agent类型: "full" 或 "monitor"
	AgentType string `mapstructure:"agent_type"`

	// 监控设置
	MonitorInterval time.Duration `mapstructure:"monitor_interval"`

	// 日志设置
	LogLevel string `mapstructure:"log_level"`
	LogFile  string `mapstructure:"log_file"`

	// 监控设置
	EnableCPUMonitor     bool `mapstructure:"enable_cpu_monitor"`
	EnableMemMonitor     bool `mapstructure:"enable_mem_monitor"`
	EnableDiskMonitor    bool `mapstructure:"enable_disk_monitor"`
	EnableNetworkMonitor bool `mapstructure:"enable_network_monitor"`

	// 升级设置
	UpdateRepo    string `mapstructure:"update_repo"`
	UpdateChannel string `mapstructure:"update_channel"`
	UpdateMirror  string `mapstructure:"update_mirror"`
}

// LoadConfig 从配置文件加载配置{error: "发送命令失败: Agent错误: 重启Nginx失败: exit status 1"}
func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	// 设置默认值
	v.SetDefault("server_url", "127.0.0.1:8080") // 移除默认协议前缀，避免解析问题
	v.SetDefault("server_id", 0)
	v.SetDefault("secret_key", "")
	v.SetDefault("register_token", "")
	v.SetDefault("monitor_interval", "30s")
	v.SetDefault("log_level", "info")
	v.SetDefault("log_file", "./agent.log")
	v.SetDefault("enable_cpu_monitor", true)
	v.SetDefault("enable_mem_monitor", true)
	v.SetDefault("enable_disk_monitor", true)
	v.SetDefault("enable_network_monitor", true)
	v.SetDefault("update_repo", "EnderKC/BetterMonitor")
	v.SetDefault("update_channel", "stable")
	v.SetDefault("update_mirror", "")
	v.SetDefault("agent_type", "full")

	// 配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 查找配置文件的默认位置
		homeDir, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(filepath.Join(homeDir, ".server-ops"))
		}
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.SetConfigName("agent")
	}

	// 读取环境变量
	v.AutomaticEnv()
	v.SetEnvPrefix("AGENT") // 环境变量前缀 AGENT_*

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		// 如果找不到配置文件，生成默认配置文件
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			fmt.Println("未找到配置文件，将使用默认配置")
		} else {
			return nil, fmt.Errorf("读取配置文件错误: %w", err)
		}
	}

	// 解析配置
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("解析配置错误: %w", err)
	}

	// 解析时间间隔
	monitorInterval, err := time.ParseDuration(v.GetString("monitor_interval"))
	if err == nil {
		config.MonitorInterval = monitorInterval
	} else {
		config.MonitorInterval = 30 * time.Second
	}

	// 兼容旧版配置文件（无 agent_type 字段）
	if config.AgentType == "" {
		config.AgentType = "full"
	}

	// 配置加载完成后输出配置值
	fmt.Println("配置值:")
	fmt.Printf("ServerURL: %s\n", config.ServerURL)
	fmt.Printf("ServerID: %d\n", config.ServerID)
	fmt.Printf("SecretKey: %s\n", config.SecretKey)
	fmt.Printf("RegisterToken: %s\n", config.RegisterToken)
	fmt.Printf("AgentType: %s\n", config.AgentType)
	fmt.Printf("MonitorInterval: %s\n", config.MonitorInterval)
	fmt.Printf("LogLevel: %s\n", config.LogLevel)
	fmt.Printf("LogFile: %s\n", config.LogFile)
	fmt.Printf("EnableCPUMonitor: %t\n", config.EnableCPUMonitor)
	fmt.Printf("EnableMemMonitor: %t\n", config.EnableMemMonitor)
	fmt.Printf("UpdateRepo: %s\n", config.UpdateRepo)
	fmt.Printf("UpdateChannel: %s\n", config.UpdateChannel)
	fmt.Printf("UpdateMirror: %s\n", config.UpdateMirror)

	return &config, nil
}

// SaveConfig 将配置保存到文件
func SaveConfig(config *Config, configPath string) error {
	v := viper.New()

	// 设置配置值
	v.Set("server_url", config.ServerURL)
	v.Set("server_id", config.ServerID)
	v.Set("secret_key", config.SecretKey)
	v.Set("register_token", config.RegisterToken)
	v.Set("agent_type", config.AgentType)
	v.Set("monitor_interval", config.MonitorInterval.String())
	v.Set("log_level", config.LogLevel)
	v.Set("log_file", config.LogFile)
	v.Set("enable_cpu_monitor", config.EnableCPUMonitor)
	v.Set("enable_mem_monitor", config.EnableMemMonitor)
	v.Set("enable_disk_monitor", config.EnableDiskMonitor)
	v.Set("enable_network_monitor", config.EnableNetworkMonitor)
	v.Set("update_repo", config.UpdateRepo)
	v.Set("update_channel", config.UpdateChannel)
	v.Set("update_mirror", config.UpdateMirror)

	// 设置配置文件
	if configPath == "" {
		configPath = "./config/agent.yaml"
	}
	v.SetConfigFile(configPath)

	// 创建配置目录（如果不存在）
	configDir := filepath.Dir(configPath)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("创建配置目录错误: %w", err)
		}
	}

	// 写入配置文件
	return v.WriteConfig()
}
