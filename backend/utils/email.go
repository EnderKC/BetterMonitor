package utils

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// EmailConfig 邮件配置
type EmailConfig struct {
	SMTPHost   string
	SMTPPort   int
	Username   string
	Password   string
	FromEmail  string
	FromName   string
	ToEmail    string
	UseTLS     bool
}

// SendEmail 发送邮件
func SendEmail(config EmailConfig, subject, body string) error {
	// 设置邮件头
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", config.FromName, config.FromEmail)
	headers["To"] = config.ToEmail
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	// 构建邮件内容
	message := ""
	for key, value := range headers {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + body

	// 设置认证信息
	auth := smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost)
	
	// 设置收件人列表
	toList := []string{config.ToEmail}
	
	// SMTP服务器地址
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	// 发送邮件
	if config.UseTLS {
		// 使用TLS连接
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         config.SMTPHost,
		}

		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("TLS连接失败: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, config.SMTPHost)
		if err != nil {
			return fmt.Errorf("创建SMTP客户端失败: %w", err)
		}
		defer client.Close()

		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP认证失败: %w", err)
		}

		if err = client.Mail(config.FromEmail); err != nil {
			return fmt.Errorf("设置发件人失败: %w", err)
		}

		for _, recipient := range toList {
			if err = client.Rcpt(recipient); err != nil {
				return fmt.Errorf("设置收件人失败: %w", err)
			}
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("获取数据写入器失败: %w", err)
		}

		_, err = w.Write([]byte(message))
		if err != nil {
			return fmt.Errorf("写入邮件内容失败: %w", err)
		}

		err = w.Close()
		if err != nil {
			return fmt.Errorf("关闭数据写入器失败: %w", err)
		}

		return client.Quit()
	}

	// 不使用TLS直接发送
	return smtp.SendMail(addr, auth, config.FromEmail, toList, []byte(message))
}

// ParseEmailConfig 解析配置字符串为EmailConfig
func ParseEmailConfig(config map[string]string) EmailConfig {
	port := 25
	if portStr, ok := config["smtp_port"]; ok {
		fmt.Sscanf(portStr, "%d", &port)
	}
	
	useTLS := false
	if tlsStr, ok := config["use_tls"]; ok {
		useTLS = strings.ToLower(tlsStr) == "true"
	}
	
	return EmailConfig{
		SMTPHost:  config["smtp_host"],
		SMTPPort:  port,
		Username:  config["username"],
		Password:  config["password"],
		FromEmail: config["from_email"],
		FromName:  config["from_name"],
		ToEmail:   config["to_email"],
		UseTLS:    useTLS,
	}
} 