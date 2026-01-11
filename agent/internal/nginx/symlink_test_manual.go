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

	// 测试1: 尝试保存到symlink（应该被拒绝）
	fmt.Println("=== 测试1: SaveRawConfig symlink防护 ===")
	domain := "test_symlink" // 这会映射到 test_symlink.conf
	content := `server {
    listen 80;
    server_name modified.example.com;
    root /var/www;
}`

	err = client.SaveRawConfig(domain, content)
	if err != nil {
		if err.Error() == "配置路径不安全: /opt/node/openresty/conf/vhost/test_symlink.conf 是符号链接" {
			fmt.Println("✅ PASS: SaveRawConfig正确拒绝了symlink")
		} else {
			fmt.Printf("❌ FAIL: 错误信息不匹配\n实际: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("❌ FAIL: SaveRawConfig没有拒绝symlink，这是安全漏洞！")
		os.Exit(1)
	}

	// 测试2: 尝试GetRawConfig读取symlink（应该成功，因为读操作不需要symlink防护）
	fmt.Println("\n=== 测试2: GetRawConfig symlink处理 ===")
	_, err = client.GetRawConfig(domain)
	if err != nil {
		fmt.Printf("GetRawConfig返回错误: %v\n", err)
	} else {
		fmt.Println("✅ GetRawConfig可以读取symlink（读操作允许）")
	}

	fmt.Println("\n=== 所有symlink防护测试通过 ===")
}
