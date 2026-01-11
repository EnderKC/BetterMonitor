//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/user/server-ops-agent/internal/nginx"
	"github.com/user/server-ops-agent/pkg/logger"
)

func main() {
	// 创建logger
	logger, err := logger.New("", "info")
	if err != nil {
		log.Fatalf("创建logger失败: %v", err)
	}

	// 创建NginxClient
	client, err := nginx.NewNginxClient(logger)
	if err != nil {
		log.Fatalf("创建NginxClient失败: %v", err)
	}
	defer client.Close()

	domain := "test_perm" // 映射到 test_perm.conf (0666权限)
	newContent := `server {
    listen 80;
    server_name test-perm-modified.example.com;
    root /var/www;
}`

	// 测试1: 默认情况下应该拒绝0666权限的文件
	fmt.Println("=== 测试1: 默认拒绝0666权限（group/other可写） ===")
	os.Unsetenv("NGINX_ALLOW_WIDE_PERMISSIONS")

	err = client.SaveRawConfig(domain, newContent)
	if err != nil {
		if contains(err.Error(), "不安全的文件权限") && contains(err.Error(), "0666") {
			fmt.Println("✅ PASS: SaveRawConfig正确拒绝了0666权限文件")
			fmt.Printf("   错误信息: %v\n", err)
		} else {
			fmt.Printf("❌ FAIL: 错误信息不匹配\n   实际: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("❌ FAIL: SaveRawConfig没有拒绝0666权限文件，这是安全漏洞！")
		os.Exit(1)
	}

	// 测试2: 设置环境变量后应该允许，但输出警告
	fmt.Println("\n=== 测试2: 环境变量NGINX_ALLOW_WIDE_PERMISSIONS=1放宽策略 ===")
	os.Setenv("NGINX_ALLOW_WIDE_PERMISSIONS", "1")

	// 重新创建client以应用环境变量
	client.Close()
	client, err = nginx.NewNginxClient(logger)
	if err != nil {
		log.Fatalf("重新创建NginxClient失败: %v", err)
	}
	defer client.Close()

	err = client.SaveRawConfig(domain, newContent)
	if err != nil {
		fmt.Printf("❌ FAIL: 设置环境变量后仍然拒绝\n   错误: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Println("✅ PASS: 环境变量生效，允许保存0666权限文件")
		fmt.Println("   注意: 应该在日志中看到警告信息")
	}

	fmt.Println("\n=== 所有权限策略测试通过 ===")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
