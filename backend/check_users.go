//go:build tools
// +build tools

package main

import (
	"fmt"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	ID       uint   `gorm:"primarykey"`
	Username string
	Role     string
}

func main() {
	// 连接数据库
	db, err := gorm.Open(sqlite.Open("data/data.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	// 查询所有用户
	var users []User
	result := db.Find(&users)
	if result.Error != nil {
		log.Fatal("查询失败:", result.Error)
	}

	fmt.Println("=== 当前系统用户列表 ===")
	fmt.Printf("%-5s %-20s %-10s\n", "ID", "用户名", "角色")
	fmt.Println("----------------------------------------")

	for _, user := range users {
		fmt.Printf("%-5d %-20s %-10s\n", user.ID, user.Username, user.Role)
	}

	fmt.Println("\n总用户数:", len(users))
}
