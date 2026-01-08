package monitor

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/user/server-ops-agent/internal/nginx"
)

// NginxInfo 存储Nginx信息
type NginxInfo struct {
	Running bool     `json:"running"`
	Version string   `json:"version"`
	Sites   int      `json:"sites"`
	Errors  []string `json:"errors"`
}

// NginxConfigFile 存储Nginx配置文件信息
type NginxConfigFile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModTime      time.Time `json:"mod_time"`
	IsSiteConfig bool      `json:"is_site_config"`
}

// NginxLogFile 存储Nginx日志文件信息
type NginxLogFile struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Type    string    `json:"type"` // access或error
}

// NginxProcess 存储Nginx进程信息
type NginxProcess struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float32 `json:"mem_percent"`
	Status     string  `json:"status"`
	CreateTime int64   `json:"create_time"`
	Command    string  `json:"command"`
}

// NginxPort 存储Nginx端口信息
type NginxPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Status   string `json:"status"`
	PID      int32  `json:"pid"`
}

// SSL证书结构
type SSLCertificate struct {
	Domain       string    `json:"domain"`
	Expiry       time.Time `json:"expiry"`
	IsValid      bool      `json:"is_valid"`
	CertPath     string    `json:"cert_path"`
	KeyPath      string    `json:"key_path"`
	IssueDate    time.Time `json:"issue_date"`
	IssuerName   string    `json:"issuer_name"`
	SerialNumber string    `json:"serial_number"`
	Fingerprint  string    `json:"fingerprint"`
	KeySize      int       `json:"key_size"`
	SignatureAlg string    `json:"signature_algorithm"`
	Source       string    `json:"source"` // "certbot" 或 "system"
	DaysLeft     int       `json:"days_left"`
}

// 存储Certbot安装状态的全局变量
var (
	certbotInstallStatus = struct {
		sync.RWMutex
		IsInstalling bool
		Success      bool
		Output       string
		Error        string
		CompletedAt  time.Time
	}{}
)

// DetectNginxPaths 检测Nginx安装路径
func DetectNginxPaths() (string, string, string) {
	var configPath, nginxBin, nginxConfDir string

	// 常见的Nginx配置路径
	configPaths := []string{
		"/etc/nginx/nginx.conf",
		"/usr/local/nginx/conf/nginx.conf",
		"/usr/local/etc/nginx/nginx.conf",
	}

	// 检查配置文件是否存在
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			nginxConfDir = filepath.Dir(path)
			break
		}
	}

	// 尝试使用which命令查找nginx可执行文件
	cmd := exec.Command("which", "nginx")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		nginxBin = strings.TrimSpace(string(output))
	} else {
		// 尝试常见的nginx可执行文件路径
		binPaths := []string{
			"/usr/sbin/nginx",
			"/usr/local/sbin/nginx",
			"/usr/bin/nginx",
		}
		for _, path := range binPaths {
			if _, err := os.Stat(path); err == nil {
				nginxBin = path
				break
			}
		}
	}

	return configPath, nginxBin, nginxConfDir
}

// GetNginxStatus 获取Nginx运行状态
func GetNginxStatus() (*NginxInfo, error) {
	info := &NginxInfo{
		Running: false,
		Version: "",
		Sites:   0,
		Errors:  []string{},
	}

	// 检测Nginx可执行文件是否存在
	_, nginxBin, _ := DetectNginxPaths()
	nginxExists := nginxBin != ""

	// 获取Nginx版本
	var cmd *exec.Cmd
	if nginxExists {
		cmd = exec.Command(nginxBin, "-v")
	} else {
		cmd = exec.Command("nginx", "-v") // 尝试使用PATH中的nginx
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("获取Nginx版本失败: %s", err))
		// 如果无法获取版本，很可能Nginx没有正确安装
		nginxExists = false
	} else {
		// nginx -v输出到stderr，格式如：nginx version: nginx/1.18.0 (Ubuntu)
		version := string(output)
		versionRegex := regexp.MustCompile(`nginx/(\d+\.\d+\.\d+)`)
		matches := versionRegex.FindStringSubmatch(version)
		if len(matches) > 1 {
			info.Version = matches[1]
		}
	}

	// 仅当Nginx可执行文件存在时，才检查进程
	if nginxExists {
		// 检查Nginx是否正在运行
		cmd = exec.Command("pgrep", "nginx")
		if err := cmd.Run(); err == nil {
			info.Running = true
		}
	}

	// 获取配置文件位置
	configPath, _, confDir := DetectNginxPaths()
	if configPath == "" {
		info.Errors = append(info.Errors, "未找到Nginx配置文件")
		return info, nil
	}

	// 计算配置的网站数量（通过查找server_name指令）
	sitesCount := 0
	err = filepath.Walk(confDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".conf")) {
			content, err := ioutil.ReadFile(path)
			if err == nil {
				matches := regexp.MustCompile(`(?m)^\s*server_name`).FindAllString(string(content), -1)
				sitesCount += len(matches)
			}
		}
		return nil
	})
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("扫描配置文件失败: %s", err))
	}
	info.Sites = sitesCount

	return info, nil
}

// GetNginxConfigsList 获取Nginx配置文件列表
func GetNginxConfigsList() ([]NginxConfigFile, error) {
	var configs []NginxConfigFile

	_, _, confDir := DetectNginxPaths()
	if confDir == "" {
		return configs, fmt.Errorf("未找到Nginx配置目录")
	}

	// 预编译正则表达式来匹配server块
	serverBlockRegex := regexp.MustCompile(`(?m)^\s*server\s*{`)

	err := filepath.Walk(confDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".conf")) {
			// 读取文件内容以检查是否包含server块
			content, err := ioutil.ReadFile(path)
			isSiteConfig := false
			if err == nil {
				isSiteConfig = serverBlockRegex.Match(content)
			}

			// 生成一个基于文件路径的稳定ID，而不是使用随机时间戳
			// 使用MD5哈希算法，确保相同路径总是生成相同的ID
			h := md5.New()
			io.WriteString(h, path)
			id := fmt.Sprintf("%x", h.Sum(nil))

			configs = append(configs, NginxConfigFile{
				ID:           id,
				Name:         filepath.Base(path),
				Path:         path,
				Size:         info.Size(),
				ModTime:      info.ModTime(),
				IsSiteConfig: isSiteConfig,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描配置文件失败: %s", err)
	}

	return configs, nil
}

// GetNginxConfigContent 获取Nginx配置文件内容
func GetNginxConfigContent(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %s", err)
	}
	return string(content), nil
}

// SaveNginxConfig 保存Nginx配置文件内容
func SaveNginxConfig(path, content string) error {
	err := ioutil.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("保存配置文件失败: %s", err)
	}
	return nil
}

// CreateNginxConfig 创建Nginx配置文件
func CreateNginxConfig(path, content string) error {
	// 检查文件是否已存在
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("配置文件已存在")
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %s", err)
	}

	// 写入文件
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("创建配置文件失败: %s", err)
	}

	return nil
}

// DeleteNginxConfig 删除Nginx配置文件
func DeleteNginxConfig(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除配置文件失败: %s", err)
	}
	return nil
}

// GetNginxLogsList 获取Nginx日志文件列表
func GetNginxLogsList() ([]NginxLogFile, error) {
	var logs []NginxLogFile

	// 常见的Nginx日志目录
	logDirs := []string{
		"/var/log/nginx",
		"/usr/local/nginx/logs",
		"/usr/local/var/log/nginx",
	}

	for _, dir := range logDirs {
		if _, err := os.Stat(dir); err == nil {
			// 目录存在，查找日志文件
			err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && (strings.Contains(path, "access") || strings.Contains(path, "error")) {
					// 确定日志类型
					logType := "other"
					if strings.Contains(path, "access") {
						logType = "access"
					} else if strings.Contains(path, "error") {
						logType = "error"
					}

					// 生成一个基于文件路径的稳定ID
					h := md5.New()
					io.WriteString(h, path)
					id := fmt.Sprintf("%x", h.Sum(nil))

					logs = append(logs, NginxLogFile{
						ID:      id,
						Name:    filepath.Base(path),
						Path:    path,
						Size:    info.Size(),
						ModTime: info.ModTime(),
						Type:    logType,
					})
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("扫描日志文件失败: %s", err)
			}
		}
	}

	return logs, nil
}

// GetNginxLogContent 获取Nginx日志文件内容
func GetNginxLogContent(path string) (string, error) {
	// 对于大文件，仅读取最后1000行
	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("读取日志文件信息失败: %s", err)
	} else if info.Size() > 1024*1024*5 { // 如果超过5MB
		return readLastLines(path, 1000)
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取日志文件失败: %s", err)
	}
	return string(content), nil
}

// readLastLines 读取文件最后的n行
func readLastLines(filePath string, n int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string

	// 循环读取所有行，保持最后n行
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}

// RestartNginx 重启Nginx服务
func RestartNginx() (bool, string, error) {
	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return false, "", fmt.Errorf("初始化OpenResty客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.TestConfig(); err != nil {
		return false, "", fmt.Errorf("配置语法检查失败: %w", err)
	}
	if err := client.ReloadNginx(); err != nil {
		return false, "", fmt.Errorf("重载OpenResty失败: %w", err)
	}
	return true, "OpenResty配置已重载", nil
}

// StopNginx 停止Nginx服务
func StopNginx() (bool, string, error) {
	cmd := exec.Command("systemctl", "stop", "nginx")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 尝试使用nginx -s stop
		_, nginxBin, _ := DetectNginxPaths()
		if nginxBin != "" {
			cmd = exec.Command(nginxBin, "-s", "stop")
			output, err = cmd.CombinedOutput()
		}
	}

	if err != nil {
		return false, string(output), fmt.Errorf("停止Nginx失败: %s", err)
	}
	return true, string(output), nil
}

// StartNginx 启动Nginx服务
func StartNginx() (bool, string, error) {
	cmd := exec.Command("systemctl", "start", "nginx")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 尝试直接使用nginx命令启动
		_, nginxBin, _ := DetectNginxPaths()
		if nginxBin != "" {
			cmd = exec.Command(nginxBin)
			output, err = cmd.CombinedOutput()
		}
	}

	if err != nil {
		return false, string(output), fmt.Errorf("启动Nginx失败: %s", err)
	}
	return true, string(output), nil
}

// TestNginxConfig 测试Nginx配置
func TestNginxConfig() (bool, string, error) {
	_, nginxBin, _ := DetectNginxPaths()
	if nginxBin == "" {
		return false, "", fmt.Errorf("未找到Nginx可执行文件")
	}

	cmd := exec.Command(nginxBin, "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(output), fmt.Errorf("配置测试失败: %s", err)
	}
	return true, string(output), nil
}

// GetNginxProcesses 获取Nginx相关进程
func GetNginxProcesses() ([]NginxProcess, error) {
	var nginxProcesses []NginxProcess

	processes, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("获取进程列表失败: %s", err)
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(name), "nginx") {
			cpu, _ := p.CPUPercent()
			mem, _ := p.MemoryPercent()
			status, _ := p.Status()
			createTime, _ := p.CreateTime()
			cmdline, _ := p.Cmdline()

			// 确保status是字符串
			statusStr := ""
			if len(status) > 0 {
				statusStr = status[0] // 取第一个状态
			}

			nginxProcesses = append(nginxProcesses, NginxProcess{
				PID:        p.Pid,
				Name:       name,
				CPUPercent: cpu,
				MemPercent: mem,
				Status:     statusStr,
				CreateTime: createTime,
				Command:    cmdline,
			})
		}
	}

	return nginxProcesses, nil
}

// GetNginxPorts 获取Nginx占用的端口
func GetNginxPorts() ([]NginxPort, error) {
	var ports []NginxPort

	// 使用ss命令查看Nginx占用的端口
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		cmd = exec.Command("ss", "-tlnp")
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("lsof", "-i", "-P", "-n")
	} else {
		return nil, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取端口信息失败: %s", err)
	}

	// 解析输出，查找Nginx进程
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "nginx") {
			// 解析端口信息
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}

			var address, protocol string
			var port int
			var pid int32

			if runtime.GOOS == "linux" {
				// 解析ss输出
				// LISTEN 0 511 *:80 *:* users:(("nginx",pid=12345,fd=6))
				addrPort := fields[4]
				parts := strings.Split(addrPort, ":")
				if len(parts) > 1 {
					address = parts[0]
					portStr := parts[1]
					if portVal, err := strconv.Atoi(portStr); err == nil {
						port = portVal
					}
				}
				protocol = "tcp"

				// 解析PID
				pidMatch := regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(line)
				if len(pidMatch) > 1 {
					if pidVal, err := strconv.Atoi(pidMatch[1]); err == nil {
						pid = int32(pidVal)
					}
				}
			} else if runtime.GOOS == "darwin" {
				// 解析lsof输出
				// nginx  12345 user   7u  IPv4 0x1234567890abcdef      0t0  TCP *:80 (LISTEN)
				for i, field := range fields {
					if strings.Contains(field, "TCP") || strings.Contains(field, "UDP") {
						protocol = strings.ToLower(field)
						if i+1 < len(fields) {
							addrPort := fields[i+1]
							parts := strings.Split(addrPort, ":")
							if len(parts) > 1 {
								address = parts[0]
								portStr := strings.Split(parts[1], " ")[0] // 去掉可能的状态信息
								if portVal, err := strconv.Atoi(portStr); err == nil {
									port = portVal
								}
							}
						}
						break
					}
				}

				// PID在Darwin下是字段2
				if pidVal, err := strconv.Atoi(fields[1]); err == nil {
					pid = int32(pidVal)
				}
			}

			if port > 0 {
				ports = append(ports, NginxPort{
					Port:     port,
					Protocol: protocol,
					Address:  address,
					Status:   "LISTEN",
					PID:      pid,
				})
			}
		}
	}

	return ports, nil
}

// CheckCertbotInstallation 检查Certbot是否安装
func CheckCertbotInstallation() (bool, error) {
	// 检查certbot命令是否存在
	cmd := exec.Command("which", "certbot")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return false, fmt.Errorf("Certbot未安装或不在PATH中")
	}
	return true, nil
}

// InstallCertbot 安装Certbot
func InstallCertbot() (bool, string, error) {
	var cmd *exec.Cmd

	// 检测操作系统和包管理器
	// 检查apt (Debian/Ubuntu)
	aptCmd := exec.Command("which", "apt")
	if err := aptCmd.Run(); err == nil {
		cmd = exec.Command("sh", "-c", "apt-get update && apt-get install -y certbot python3-certbot-nginx")
	} else {
		// 检查yum (CentOS/RHEL)
		yumCmd := exec.Command("which", "yum")
		if err := yumCmd.Run(); err == nil {
			cmd = exec.Command("sh", "-c", "yum install -y epel-release && yum install -y certbot python3-certbot-nginx")
		} else {
			// 检查dnf (Fedora)
			dnfCmd := exec.Command("which", "dnf")
			if err := dnfCmd.Run(); err == nil {
				cmd = exec.Command("sh", "-c", "dnf install -y certbot python3-certbot-nginx")
			} else {
				// 检查snap (通用方法)
				snapCmd := exec.Command("which", "snap")
				if err := snapCmd.Run(); err == nil {
					cmd = exec.Command("sh", "-c", "snap install --classic certbot && ln -sf /snap/bin/certbot /usr/bin/certbot")
				} else {
					return false, "", fmt.Errorf("不支持的操作系统或包管理器")
				}
			}
		}
	}

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(output), fmt.Errorf("安装Certbot失败: %v", err)
	}

	return true, string(output), nil
}

// InstallCertbotAsync 异步安装Certbot
func InstallCertbotAsync() map[string]interface{} {
	// 检查是否已经在安装中
	certbotInstallStatus.RLock()
	isInstalling := certbotInstallStatus.IsInstalling
	certbotInstallStatus.RUnlock()

	if isInstalling {
		return map[string]interface{}{
			"status":  "installing",
			"message": "Certbot正在安装中，请稍后查询安装状态",
		}
	}

	// 更新安装状态为"开始安装"
	certbotInstallStatus.Lock()
	certbotInstallStatus.IsInstalling = true
	certbotInstallStatus.Success = false
	certbotInstallStatus.Output = ""
	certbotInstallStatus.Error = ""
	certbotInstallStatus.CompletedAt = time.Time{}
	certbotInstallStatus.Unlock()

	// 在后台执行安装
	go func() {
		success, output, err := InstallCertbot()

		certbotInstallStatus.Lock()
		certbotInstallStatus.IsInstalling = false
		certbotInstallStatus.Success = success
		certbotInstallStatus.Output = output
		if err != nil {
			certbotInstallStatus.Error = err.Error()
		}
		certbotInstallStatus.CompletedAt = time.Now()
		certbotInstallStatus.Unlock()
	}()

	return map[string]interface{}{
		"status":  "started",
		"message": "Certbot安装已开始，请稍后查询安装状态",
	}
}

// GetCertbotInstallStatus 获取Certbot安装状态
func GetCertbotInstallStatus() map[string]interface{} {
	certbotInstallStatus.RLock()
	defer certbotInstallStatus.RUnlock()

	status := "unknown"
	if certbotInstallStatus.IsInstalling {
		status = "installing"
	} else if !certbotInstallStatus.CompletedAt.IsZero() {
		if certbotInstallStatus.Success {
			status = "success"
		} else {
			status = "failed"
		}
	} else {
		// 检查Certbot是否已安装
		installed, _ := CheckCertbotInstallation()
		if installed {
			status = "installed"
		} else {
			status = "not_installed"
		}
	}

	result := map[string]interface{}{
		"status": status,
	}

	if certbotInstallStatus.IsInstalling {
		result["message"] = "Certbot正在安装中，请稍后再次查询"
	} else if status == "success" {
		result["message"] = "Certbot安装成功"
		result["output"] = certbotInstallStatus.Output
		result["completed_at"] = certbotInstallStatus.CompletedAt
	} else if status == "failed" {
		result["message"] = "Certbot安装失败"
		result["error"] = certbotInstallStatus.Error
		result["output"] = certbotInstallStatus.Output
		result["completed_at"] = certbotInstallStatus.CompletedAt
	}

	return result
}

// RequestCertificate 申请SSL证书
func RequestCertificate(domain, email, webroot string, useStaging bool) (map[string]interface{}, error) {
	// 检查Certbot是否安装
	installed, err := CheckCertbotInstallation()
	if err != nil || !installed {
		// 尝试安装Certbot
		success, output, installErr := InstallCertbot()
		if installErr != nil {
			return map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Certbot未安装且安装失败: %v", installErr),
				"output":  output,
			}, nil
		}

		if !success {
			return map[string]interface{}{
				"success": false,
				"message": "Certbot安装未成功",
				"output":  output,
			}, nil
		}
	}

	// 构建Certbot命令
	cmdArgs := []string{}

	if webroot != "" {
		// 使用webroot插件
		cmdArgs = append(cmdArgs, "certonly", "--webroot", "-w", webroot, "-d", domain, "--email", email, "--agree-tos", "--non-interactive")
	} else {
		// 使用nginx插件
		cmdArgs = append(cmdArgs, "--nginx", "-d", domain, "--email", email, "--agree-tos", "--non-interactive")
	}

	// 如果使用测试环境
	if useStaging {
		cmdArgs = append(cmdArgs, "--staging")
	}

	// 执行命令
	cmd := exec.Command("certbot", cmdArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("申请证书失败: %v", err),
			"output":  string(output),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"message": "证书申请成功",
		"output":  string(output),
		"domain":  domain,
	}, nil
}

// RenewCertificates 续期所有证书
func RenewCertificates() (map[string]interface{}, error) {
	// 检查Certbot是否安装
	installed, err := CheckCertbotInstallation()
	if err != nil || !installed {
		return map[string]interface{}{
			"success": false,
			"message": "Certbot未安装，无法续期证书",
		}, nil
	}

	// 执行续期命令
	cmd := exec.Command("certbot", "renew", "--non-interactive")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("续期证书失败: %v", err),
			"output":  string(output),
		}, nil
	}

	return map[string]interface{}{
		"success": true,
		"message": "证书续期成功或无需续期",
		"output":  string(output),
	}, nil
}

// ListCertificates 列出所有证书
func ListCertificates() ([]SSLCertificate, error) {
	certificates := []SSLCertificate{}

	// 尝试通过Certbot获取证书列表
	certbotCerts, certbotErr := getCertbotCertificates()
	if certbotErr == nil {
		certificates = append(certificates, certbotCerts...)
	}

	// 扫描文件系统中的证书
	systemCerts, systemErr := scanSystemCertificates()
	if systemErr == nil {
		// 去重并合并证书列表
		certificates = mergeCertificates(certificates, systemCerts)
	}

	// 扫描正在运行的HTTPS服务证书
	runningCerts, runningErr := scanRunningHTTPSServices()
	if runningErr == nil {
		certificates = mergeCertificates(certificates, runningCerts)
	}

	// 如果所有方法都失败了，返回错误
	if certbotErr != nil && systemErr != nil && runningErr != nil {
		return nil, fmt.Errorf("无法获取证书列表: certbot错误=%v, 系统扫描错误=%v, 服务扫描错误=%v", certbotErr, systemErr, runningErr)
	}

	return certificates, nil
}

// getCertbotCertificates 通过Certbot命令获取证书列表
func getCertbotCertificates() ([]SSLCertificate, error) {
	// 检查Certbot是否安装
	installed, err := CheckCertbotInstallation()
	if err != nil || !installed {
		return nil, fmt.Errorf("Certbot未安装，无法列出证书")
	}

	// 执行列出证书命令
	cmd := exec.Command("certbot", "certificates")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("列出证书失败: %v", err)
	}

	// 解析输出
	certificates := []SSLCertificate{}

	// 使用正则表达式解析certbot certificates的输出
	domainRegex := regexp.MustCompile(`Domain: (.+)`)
	expiryRegex := regexp.MustCompile(`Expiry Date: (.+) \(`)
	pathRegex := regexp.MustCompile(`Certificate Path: (.+)`)
	keyPathRegex := regexp.MustCompile(`Private Key Path: (.+)`)

	// 分割每个证书信息块
	certBlocks := strings.Split(string(output), "  Certificate Name:")
	for _, block := range certBlocks {
		if !strings.Contains(block, "Expiry Date:") {
			continue
		}

		var cert SSLCertificate
		cert.Source = "certbot"

		// 提取域名
		if domainMatches := domainRegex.FindStringSubmatch(block); len(domainMatches) > 1 {
			cert.Domain = strings.TrimSpace(domainMatches[1])
		}

		// 提取过期日期
		if expiryMatches := expiryRegex.FindStringSubmatch(block); len(expiryMatches) > 1 {
			expiryStr := strings.TrimSpace(expiryMatches[1])
			// 解析日期格式，例如 "2023-05-31 23:59:59+00:00"
			expiry, parseErr := time.Parse("2006-01-02 15:04:05-07:00", expiryStr)
			if parseErr == nil {
				cert.Expiry = expiry
				// 检查证书是否有效（未过期）
				cert.IsValid = time.Now().Before(expiry)
				cert.DaysLeft = int(expiry.Sub(time.Now()).Hours() / 24)
			}
		}

		// 提取证书路径
		if pathMatches := pathRegex.FindStringSubmatch(block); len(pathMatches) > 1 {
			cert.CertPath = strings.TrimSpace(pathMatches[1])
		}

		// 提取私钥路径
		if keyPathMatches := keyPathRegex.FindStringSubmatch(block); len(keyPathMatches) > 1 {
			cert.KeyPath = strings.TrimSpace(keyPathMatches[1])
		}

		// 尝试从证书文件中获取更多信息
		if cert.CertPath != "" {
			certInfo, parseErr := GetCertificateInfo(cert.CertPath)
			if parseErr == nil {
				// 使用新的构建函数更新证书信息
				fullCert := buildSSLCertificateFromX509(certInfo, cert.CertPath, cert.KeyPath, "certbot")
				// 保留从certbot命令获取的域名信息（可能更准确）
				if cert.Domain != "" {
					fullCert.Domain = cert.Domain
				}
				cert = fullCert
			}
		}

		if cert.Domain != "" {
			certificates = append(certificates, cert)
		}
	}

	return certificates, nil
}

// scanSystemCertificates 扫描系统中的SSL证书文件
func scanSystemCertificates() ([]SSLCertificate, error) {
	certificates := []SSLCertificate{}

	// 针对HTTPS网站证书的目录 - 排除系统CA证书目录
	certDirs := []string{
		"/etc/nginx/ssl",
		"/etc/nginx/certs",
		"/etc/apache2/ssl",
		"/etc/apache2/certs",
		"/etc/httpd/ssl",
		"/etc/httpd/certs",
		"/etc/letsencrypt/live",
		"/etc/letsencrypt/archive",
		"/usr/local/etc/nginx/ssl",
		"/usr/local/etc/ssl/certs", // 仅此目录可能包含网站证书
		"/opt/ssl",
		"/var/ssl",
		"/home/*/ssl", // 用户目录下的SSL证书
		"/root/ssl",
		"/etc/ssl/private", // 私有证书目录
	}

	// 证书文件扩展名
	certExtensions := []string{".crt", ".cert", ".pem", ".cer"}

	for _, dir := range certDirs {
		// 处理通配符路径
		if strings.Contains(dir, "*") {
			matches, _ := filepath.Glob(dir)
			for _, match := range matches {
				scanCertificatesInDirectory(match, certExtensions, &certificates)
			}
		} else {
			scanCertificatesInDirectory(dir, certExtensions, &certificates)
		}
	}

	return certificates, nil
}

// scanCertificatesInDirectory 扫描指定目录中的证书
func scanCertificatesInDirectory(dir string, certExtensions []string, certificates *[]SSLCertificate) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return // 目录不存在，跳过
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略访问错误，继续扫描
		}

		if info.IsDir() {
			return nil // 跳过目录
		}

		// 检查文件扩展名
		ext := strings.ToLower(filepath.Ext(path))
		isValidExt := false
		for _, validExt := range certExtensions {
			if ext == validExt {
				isValidExt = true
				break
			}
		}

		if !isValidExt {
			return nil // 不是证书文件，跳过
		}

		// 尝试解析证书
		cert, parseErr := GetCertificateInfo(path)
		if parseErr != nil {
			return nil // 解析失败，可能不是有效的证书文件
		}

		// 过滤掉系统CA根证书 - 只保留网站证书
		if !isWebsiteCertificate(cert, path) {
			return nil
		}

		// 尝试找到对应的私钥文件
		keyPath := findPrivateKeyForCert(path)

		// 使用新的构建函数构建证书信息
		sslCert := buildSSLCertificateFromX509(cert, path, keyPath, "system")

		*certificates = append(*certificates, sslCert)
		return nil
	})

	if err != nil {
		fmt.Printf("扫描目录 %s 时出错: %v\n", dir, err)
	}
}

// isWebsiteCertificate 判断是否为网站证书（而非CA根证书）
func isWebsiteCertificate(cert *x509.Certificate, path string) bool {
	// 排除明显的CA根证书目录
	if strings.Contains(path, "/etc/ssl/certs/") && !strings.Contains(path, "localhost") {
		// /etc/ssl/certs/ 目录主要是系统CA证书，但可能有localhost证书
		return false
	}

	// 检查证书是否为CA证书
	if cert.IsCA {
		return false
	}

	// 检查Subject是否为知名CA
	subject := cert.Subject.String()
	caKeywords := []string{
		"Root CA", "Intermediate CA", "Certificate Authority",
		"DigiCert", "Verisign", "Symantec", "GeoTrust", "Thawte",
		"GlobalSign", "Comodo", "Sectigo", "IdenTrust", "Baltimore",
		"AddTrust", "UTN", "USERTrust", "Entrust",
	}

	for _, keyword := range caKeywords {
		if strings.Contains(subject, keyword) {
			return false
		}
	}

	// 检查是否有域名信息（网站证书通常有具体域名）
	hasDomain := false
	if len(cert.DNSNames) > 0 {
		for _, name := range cert.DNSNames {
			// 排除通用的测试域名
			if name != "localhost" && name != "example.com" && name != "test.com" {
				hasDomain = true
				break
			}
		}
	}

	// 检查Subject CommonName是否为具体域名
	if cert.Subject.CommonName != "" {
		cn := cert.Subject.CommonName
		if !strings.Contains(cn, "CA") &&
			!strings.Contains(cn, "Certificate Authority") &&
			!strings.Contains(cn, "Root") &&
			cn != "localhost" {
			hasDomain = true
		}
	}

	// 检查证书用途 - 网站证书通常用于服务器认证
	hasServerAuth := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			hasServerAuth = true
			break
		}
	}

	// 检查文件路径是否表明这是网站证书
	isInWebServerDir := strings.Contains(path, "/nginx/") ||
		strings.Contains(path, "/apache") ||
		strings.Contains(path, "/httpd/") ||
		strings.Contains(path, "/letsencrypt/") ||
		strings.Contains(path, "/ssl/") && !strings.Contains(path, "/etc/ssl/certs/")

	// 如果有域名信息或者在网站服务器目录，且有服务器认证用途，则认为是网站证书
	return (hasDomain || isInWebServerDir) && (hasServerAuth || len(cert.ExtKeyUsage) == 0)
}

// findPrivateKeyForCert 为证书文件寻找对应的私钥文件
func findPrivateKeyForCert(certPath string) string {
	dir := filepath.Dir(certPath)
	basename := strings.TrimSuffix(filepath.Base(certPath), filepath.Ext(certPath))

	// 常见的私钥文件命名模式
	keyPatterns := []string{
		basename + ".key",
		basename + ".pem",
		basename + "-key.pem",
		basename + "_key.pem",
		"privkey.pem", // Let's Encrypt 模式
		"private.key",
		"server.key",
	}

	for _, pattern := range keyPatterns {
		keyPath := filepath.Join(dir, pattern)
		if _, err := os.Stat(keyPath); err == nil {
			// 验证是否确实是私钥文件
			if isPrivateKeyFile(keyPath) {
				return keyPath
			}
		}
	}

	return ""
}

// isPrivateKeyFile 检查文件是否是私钥文件
func isPrivateKeyFile(path string) bool {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}

	// 检查是否包含私钥标识
	contentStr := string(content)
	return strings.Contains(contentStr, "-----BEGIN PRIVATE KEY-----") ||
		strings.Contains(contentStr, "-----BEGIN RSA PRIVATE KEY-----") ||
		strings.Contains(contentStr, "-----BEGIN EC PRIVATE KEY-----") ||
		strings.Contains(contentStr, "-----BEGIN ENCRYPTED PRIVATE KEY-----")
}

// mergeCertificates 合并证书列表，去除重复项
func mergeCertificates(existingCerts, newCerts []SSLCertificate) []SSLCertificate {
	merged := make([]SSLCertificate, 0, len(existingCerts)+len(newCerts))
	seen := make(map[string]bool)

	// 添加现有证书
	for _, cert := range existingCerts {
		key := generateCertKey(cert)
		if !seen[key] {
			merged = append(merged, cert)
			seen[key] = true
		}
	}

	// 添加新证书，避免重复
	for _, cert := range newCerts {
		key := generateCertKey(cert)
		if !seen[key] {
			merged = append(merged, cert)
			seen[key] = true
		}
	}

	return merged
}

// generateCertKey 生成证书的唯一标识
func generateCertKey(cert SSLCertificate) string {
	// 使用证书路径和指纹作为唯一标识
	if cert.Fingerprint != "" {
		return cert.Fingerprint
	}
	// 如果没有指纹，使用路径和域名
	return cert.CertPath + "|" + cert.Domain
}

// GetCertificateInfo 从证书文件中提取信息
func GetCertificateInfo(certPath string) (*x509.Certificate, error) {
	// 读取证书文件
	certPEM, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}

	// 解码PEM格式
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("无法解码PEM证书")
	}

	// 解析证书
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// buildSSLCertificateFromX509 从x509证书构建SSLCertificate结构
func buildSSLCertificateFromX509(cert *x509.Certificate, certPath, keyPath, source string) SSLCertificate {
	now := time.Now()
	daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)

	// 计算证书指纹 (SHA1)
	h := sha1.New()
	h.Write(cert.Raw)
	fingerprint := hex.EncodeToString(h.Sum(nil))

	// 获取密钥大小
	keySize := 0
	switch pubKey := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		keySize = pubKey.N.BitLen()
	case *ecdsa.PublicKey:
		keySize = pubKey.Params().BitSize
	}

	sslCert := SSLCertificate{
		CertPath:     certPath,
		KeyPath:      keyPath,
		Expiry:       cert.NotAfter,
		IsValid:      now.Before(cert.NotAfter),
		IssueDate:    cert.NotBefore,
		IssuerName:   cert.Issuer.CommonName,
		SerialNumber: cert.SerialNumber.String(),
		Fingerprint:  fingerprint,
		KeySize:      keySize,
		SignatureAlg: cert.SignatureAlgorithm.String(),
		Source:       source,
		DaysLeft:     daysLeft,
	}

	// 尝试提取域名
	if len(cert.DNSNames) > 0 {
		sslCert.Domain = strings.Join(cert.DNSNames, ", ")
	} else if cert.Subject.CommonName != "" {
		sslCert.Domain = cert.Subject.CommonName
	} else {
		sslCert.Domain = "未知域名"
	}

	return sslCert
}

// UninstallCertbot 卸载Certbot
func UninstallCertbot() (bool, string, error) {
	var cmd *exec.Cmd

	// 检测操作系统和包管理器
	// 检查apt (Debian/Ubuntu)
	aptCmd := exec.Command("which", "apt")
	if err := aptCmd.Run(); err == nil {
		cmd = exec.Command("sh", "-c", "apt-get remove -y certbot python3-certbot-nginx")
	} else {
		// 检查yum (CentOS/RHEL)
		yumCmd := exec.Command("which", "yum")
		if err := yumCmd.Run(); err == nil {
			cmd = exec.Command("sh", "-c", "yum remove -y certbot python3-certbot-nginx")
		} else {
			// 检查dnf (Fedora)
			dnfCmd := exec.Command("which", "dnf")
			if err := dnfCmd.Run(); err == nil {
				cmd = exec.Command("sh", "-c", "dnf remove -y certbot python3-certbot-nginx")
			} else {
				// 检查snap (通用方法)
				snapCmd := exec.Command("which", "snap")
				if err := snapCmd.Run(); err == nil {
					cmd = exec.Command("sh", "-c", "snap remove certbot && rm -f /usr/bin/certbot")
				} else {
					return false, "", fmt.Errorf("不支持的操作系统或包管理器")
				}
			}
		}
	}

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(output), fmt.Errorf("卸载Certbot失败: %v", err)
	}

	return true, string(output), nil
}

// UninstallCertbotAsync 异步卸载Certbot
func UninstallCertbotAsync() map[string]interface{} {
	// 检查是否已经在安装/卸载中
	certbotInstallStatus.RLock()
	isInstalling := certbotInstallStatus.IsInstalling
	certbotInstallStatus.RUnlock()

	if isInstalling {
		return map[string]interface{}{
			"status":  "processing",
			"message": "Certbot正在安装或卸载中，请稍后查询状态",
		}
	}

	// 更新状态为"开始卸载"
	certbotInstallStatus.Lock()
	certbotInstallStatus.IsInstalling = true
	certbotInstallStatus.Success = false
	certbotInstallStatus.Output = ""
	certbotInstallStatus.Error = ""
	certbotInstallStatus.CompletedAt = time.Time{}
	certbotInstallStatus.Unlock()

	// 在后台执行卸载
	go func() {
		success, output, err := UninstallCertbot()

		certbotInstallStatus.Lock()
		certbotInstallStatus.IsInstalling = false
		certbotInstallStatus.Success = success
		certbotInstallStatus.Output = output
		if err != nil {
			certbotInstallStatus.Error = err.Error()
		}
		certbotInstallStatus.CompletedAt = time.Now()
		certbotInstallStatus.Unlock()
	}()

	return map[string]interface{}{
		"status":  "started",
		"message": "Certbot卸载已开始，请稍后查询卸载状态",
	}
}

// HandleNginxCommand 处理Nginx相关的命令
func HandleNginxCommand(action string, params map[string]interface{}) (string, error) {
	var result interface{}
	var err error

	switch action {
	case "apply_config":
		result, err = handleApplyConfigAction(params)

	case "issue_ssl":
		result, err = handleIssueSSLAction(params)

	case "nginx_status":
		// 获取Nginx状态并返回完整信息，包括可能的错误
		nginxInfo, getErr := GetNginxStatus()
		result = nginxInfo // 即使有错误也返回信息对象

		// 仅当发生严重错误导致无法获取任何信息时才返回错误
		if getErr != nil && nginxInfo == nil {
			err = getErr
		}

	case "nginx_configs_list":
		result, err = GetNginxConfigsList()

	case "nginx_config_content":
		configId, ok := params["config_id"].(string)
		if !ok {
			// 兼容旧版API，尝试获取path参数
			path, pathOk := params["path"].(string)
			if !pathOk {
				return "", fmt.Errorf("缺少config_id或path参数")
			}
			result, err = GetNginxConfigContent(path)
		} else {
			// 使用config_id查找对应的配置文件路径
			configs, listErr := GetNginxConfigsList()
			if listErr != nil {
				return "", fmt.Errorf("获取配置列表失败: %s", listErr)
			}

			var configPath string
			for _, config := range configs {
				if config.ID == configId {
					configPath = config.Path
					break
				}
			}

			if configPath == "" {
				return "", fmt.Errorf("未找到ID为%s的配置文件", configId)
			}

			result, err = GetNginxConfigContent(configPath)
		}

	case "nginx_save_config":
		// 支持通过config_id或直接path参数保存配置
		var configPath string

		// 先检查是否提供了config_id
		configId, hasConfigId := params["config_id"].(string)
		if hasConfigId {
			// 通过ID查找配置文件路径
			configs, listErr := GetNginxConfigsList()
			if listErr != nil {
				return "", fmt.Errorf("获取配置列表失败: %s", listErr)
			}

			for _, config := range configs {
				if config.ID == configId {
					configPath = config.Path
					break
				}
			}

			if configPath == "" {
				return "", fmt.Errorf("未找到ID为%s的配置文件", configId)
			}
		} else {
			// 直接使用path参数
			path, ok := params["path"].(string)
			if !ok {
				return "", fmt.Errorf("缺少path或config_id参数")
			}
			configPath = path
		}

		// 获取内容参数
		content, ok := params["content"].(string)
		if !ok {
			return "", fmt.Errorf("缺少内容参数")
		}

		// 保存配置文件
		err = SaveNginxConfig(configPath, content)
		result = map[string]interface{}{
			"success": err == nil,
			"message": "配置保存成功",
		}

	case "nginx_create_config":
		name, ok := params["name"].(string)
		if !ok {
			return "", fmt.Errorf("缺少名称参数")
		}
		path, ok := params["path"].(string)
		if !ok {
			return "", fmt.Errorf("缺少路径参数")
		}
		content, ok := params["content"].(string)
		if !ok {
			content = ""
		}
		err = CreateNginxConfig(path, content)
		result = map[string]interface{}{
			"success": err == nil,
			"message": "配置创建成功",
			"name":    name,
			"path":    path,
		}

	case "nginx_delete_config":
		configId, ok := params["config_id"].(string)
		if !ok {
			// 兼容旧版API，检查id参数
			id, idOk := params["id"].(string)
			if !idOk {
				return "", fmt.Errorf("缺少config_id参数")
			}
			configId = id
		}
		// 需要先获取配置列表，然后根据ID找到对应的路径
		configs, _ := GetNginxConfigsList()
		var configPath string
		for _, config := range configs {
			if config.ID == configId {
				configPath = config.Path
				break
			}
		}
		if configPath == "" {
			return "", fmt.Errorf("未找到ID为%s的配置文件", configId)
		}
		err = DeleteNginxConfig(configPath)
		result = map[string]interface{}{
			"success": err == nil,
			"message": "配置删除成功",
		}

	case "nginx_logs_list":
		result, err = GetNginxLogsList()

	case "nginx_log_content":
		id, ok := params["id"].(string)
		if !ok {
			// 尝试获取path参数
			path, pathOk := params["path"].(string)
			if !pathOk {
				return "", fmt.Errorf("缺少id或path参数")
			}
			result, err = GetNginxLogContent(path)
		} else {
			// 使用id查找对应的日志文件路径
			logs, listErr := GetNginxLogsList()
			if listErr != nil {
				return "", fmt.Errorf("获取日志列表失败: %s", listErr)
			}

			var logPath string
			for _, log := range logs {
				if log.ID == id {
					logPath = log.Path
					break
				}
			}

			if logPath == "" {
				return "", fmt.Errorf("未找到ID为%s的日志文件", id)
			}

			result, err = GetNginxLogContent(logPath)
		}

	case "nginx_log_download":
		id, ok := params["id"].(string)
		if !ok {
			// 尝试获取path参数
			path, pathOk := params["path"].(string)
			if !pathOk {
				return "", fmt.Errorf("缺少id或path参数")
			}
			// 如果直接提供了path，需要获取文件名
			name := filepath.Base(path)
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("读取日志文件失败: %s", err)
			}
			result = map[string]interface{}{
				"filename": name,
				"content":  string(content),
			}
		} else {
			// 使用id查找对应的日志文件路径
			logs, listErr := GetNginxLogsList()
			if listErr != nil {
				return "", fmt.Errorf("获取日志列表失败: %s", listErr)
			}

			var logPath, logName string
			for _, log := range logs {
				if log.ID == id {
					logPath = log.Path
					logName = log.Name
					break
				}
			}

			if logPath == "" {
				return "", fmt.Errorf("未找到ID为%s的日志文件", id)
			}

			content, err := ioutil.ReadFile(logPath)
			if err != nil {
				return "", fmt.Errorf("读取日志文件失败: %s", err)
			}
			result = map[string]interface{}{
				"filename": logName,
				"content":  string(content),
			}
		}

	case "nginx_restart":
		success, output, restartErr := RestartNginx()
		if restartErr != nil {
			err = restartErr
		}
		result = map[string]interface{}{
			"success": success,
			"output":  output,
		}

	case "nginx_stop":
		success, output, stopErr := StopNginx()
		if stopErr != nil {
			err = stopErr
		}
		result = map[string]interface{}{
			"success": success,
			"output":  output,
		}

	case "nginx_start":
		success, output, startErr := StartNginx()
		if startErr != nil {
			err = startErr
		}
		result = map[string]interface{}{
			"success": success,
			"output":  output,
		}

	case "nginx_test_config":
		success, output, testErr := TestNginxConfig()
		if testErr != nil {
			err = testErr
		}
		result = map[string]interface{}{
			"success": success,
			"output":  output,
		}

	case "nginx_processes":
		result, err = GetNginxProcesses()

	case "nginx_ports":
		result, err = GetNginxPorts()

	case "nginx_sites_list":
		result, err = handleSitesListAction()

	case "openresty_status":
		result, err = handleOpenRestyStatusAction()

	case "openresty_install":
		result, err = handleOpenRestyInstallAction()

	case "openresty_install_logs":
		sessionID := getStringParam(params["session_id"])
		result, err = handleInstallLogsAction(sessionID)

	case "certificate_content":
		result, err = handleCertificateContentAction(params)

	case "certbot_check_installation":
		installed, installErr := CheckCertbotInstallation()
		result = map[string]interface{}{
			"installed": installed,
			"error":     installErr != nil,
		}
		if installErr != nil {
			result.(map[string]interface{})["message"] = installErr.Error()
		}

	case "certbot_install":
		// 改为调用异步安装方法
		result = InstallCertbotAsync()

	case "certbot_install_status":
		// 获取安装/卸载状态
		result = GetCertbotInstallStatus()

	case "certbot_request":
		domain, _ := params["domain"].(string)
		email, _ := params["email"].(string)
		webroot, _ := params["webroot"].(string)
		useStaging, _ := params["useStaging"].(bool)

		if domain == "" || email == "" {
			return "", fmt.Errorf("域名和邮箱是必须的")
		}

		result, err = RequestCertificate(domain, email, webroot, useStaging)

	case "certbot_renew":
		result, err = RenewCertificates()

	case "certbot_list":
		result, err = ListCertificates()

	case "ssl_scan_certificates":
		// 新增的专门证书扫描命令，提供更详细的扫描结果
		certificates, scanErr := ListCertificates()
		if scanErr != nil {
			err = scanErr
		} else {
			// 统计证书信息
			stats := map[string]interface{}{
				"total_certificates":   len(certificates),
				"valid_certificates":   0,
				"expired_certificates": 0,
				"expiring_soon":        0, // 30天内过期
				"certbot_certificates": 0,
				"system_certificates":  0,
			}

			validCerts := 0
			expiredCerts := 0
			expiringSoon := 0
			certbotCerts := 0
			systemCerts := 0

			for _, cert := range certificates {
				if cert.IsValid {
					validCerts++
				} else {
					expiredCerts++
				}

				if cert.DaysLeft <= 30 && cert.DaysLeft >= 0 {
					expiringSoon++
				}

				if cert.Source == "certbot" {
					certbotCerts++
				} else {
					systemCerts++
				}
			}

			stats["valid_certificates"] = validCerts
			stats["expired_certificates"] = expiredCerts
			stats["expiring_soon"] = expiringSoon
			stats["certbot_certificates"] = certbotCerts
			stats["system_certificates"] = systemCerts

			result = map[string]interface{}{
				"certificates": certificates,
				"statistics":   stats,
				"scan_time":    time.Now().Unix(),
			}
		}

	case "certbot_uninstall":
		// 调用异步卸载方法
		result = UninstallCertbotAsync()

	default:
		return "", fmt.Errorf("未知的Nginx命令: %s", action)
	}

	if err != nil {
		// 使用logger包中的函数记录错误
		fmt.Printf("执行Nginx命令失败: action=%s, error=%v\n", action, err)
		return "", err
	}

	// 将结果转换为JSON字符串
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %s", err)
	}

	return string(jsonResult), nil
}

func handleApplyConfigAction(params map[string]interface{}) (interface{}, error) {
	configPayload, ok := params["config"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("缺少config配置")
	}

	var site nginx.SiteConfig
	configBytes, err := json.Marshal(configPayload)
	if err != nil {
		return nil, fmt.Errorf("序列化站点配置失败: %w", err)
	}
	if err := json.Unmarshal(configBytes, &site); err != nil {
		return nil, fmt.Errorf("解析站点配置失败: %w", err)
	}

	if site.PrimaryDomain == "" {
		site.PrimaryDomain = getStringParam(params["domain"])
	}
	if site.PrimaryDomain == "" {
		return nil, fmt.Errorf("站点配置缺少primary_domain")
	}

	if len(site.ExtraDomains) == 0 {
		if extras := getStringSlice(params["extra_domains"]); len(extras) > 0 {
			site.ExtraDomains = extras
		} else if domains := getStringSlice(params["domains"]); len(domains) > 0 {
			site.ExtraDomains = filterDomains(domains, site.PrimaryDomain)
		}
	}

	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configPath, err := client.CreateWebsite(site)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":     true,
		"config_path": configPath,
		"site":        site,
	}, nil
}

func handleIssueSSLAction(params map[string]interface{}) (interface{}, error) {
	domains := getStringSlice(params["domains"])
	if len(domains) == 0 {
		if primary := getStringParam(params["domain"]); primary != "" {
			domains = []string{primary}
		}
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("缺少域名列表")
	}

	req := nginx.CertificateRequest{
		Domains: domains,
		Email:   getStringParam(params["email"]),
	}

	provider := strings.ToLower(getStringParam(params["provider"]))
	if provider == "" {
		provider = "http01"
	}
	req.Provider = provider

	if staging, ok := params["use_staging"].(bool); ok {
		req.UseStaging = staging
	}

	switch provider {
	case "http01":
		req.Webroot = getStringParam(params["webroot"])
	case "alidns", "aliyun", "cloudflare", "cf":
		req.DNSConfig = getStringMap(params["dns_config"])
		if len(req.DNSConfig) == 0 {
			return nil, fmt.Errorf("DNS验证需要提供凭证配置")
		}
	default:
		return nil, fmt.Errorf("暂不支持provider: %s", provider)
	}

	if staging, ok := params["use_staging"].(bool); ok {
		req.UseStaging = staging
	}

	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	result, err := client.IssueCertificate(req)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":          true,
		"domains":          result.Domains,
		"certificate_path": result.CertificatePath,
		"key_path":         result.KeyPath,
		"expiry":           result.Expiry.Format(time.RFC3339),
	}, nil
}

func handleSitesListAction() (interface{}, error) {
	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	sites, err := client.ListSites()
	if err != nil {
		return nil, err
	}

	return sites, nil
}

func handleOpenRestyStatusAction() (interface{}, error) {
	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	installed, running, err := client.GetRuntimeState()
	if err != nil {
		return nil, err
	}

	var containerVersion string
	if installed {
		if ver, verErr := client.GetVersion(); verErr == nil {
			containerVersion = ver
		}
	}

	var nativeRunning bool
	var nativeVersion string
	nativeInfo, nativeErr := GetNginxStatus()
	if nativeErr == nil && nativeInfo != nil {
		nativeRunning = nativeInfo.Running
		nativeVersion = nativeInfo.Version
	}

	mode := "container"
	if !installed {
		if nativeRunning {
			mode = "native"
		} else {
			mode = "missing"
		}
	}

	return map[string]interface{}{
		"installed":      installed,
		"running":        running,
		"version":        containerVersion,
		"mode":           mode,
		"native_running": nativeRunning,
		"native_version": nativeVersion,
	}, nil
}

func handleOpenRestyInstallAction() (interface{}, error) {
	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	installed, running, err := client.GetRuntimeState()
	if err != nil {
		return nil, err
	}

	nativeInfo, _ := GetNginxStatus()
	nativeRunning := nativeInfo != nil && nativeInfo.Running
	nativeVersion := ""
	if nativeInfo != nil {
		nativeVersion = nativeInfo.Version
	}

	if installed {
		return map[string]interface{}{
			"installed":      true,
			"running":        running,
			"mode":           "container",
			"native_running": nativeRunning,
			"native_version": nativeVersion,
			"message":        "OpenResty容器已经存在",
		}, nil
	}

	if nativeRunning {
		return nil, fmt.Errorf("检测到系统Nginx正在运行，请先停止或卸载系统中的Nginx服务，以避免与容器占用80/443端口冲突")
	}

	// 生成安装会话ID
	sessionID := fmt.Sprintf("install-%d", time.Now().Unix())
	logger := NewInstallLogger(sessionID)

	go func() {
		defer logger.Close()

		bgClient, err := nginx.NewNginxClient(nil)
		if err != nil {
			logger.Log(fmt.Sprintf("❌ 初始化客户端失败: %v", err))
			return
		}
		defer bgClient.Close()

		logger.Log("开始安装 OpenResty...")
		logger.Log("")

		if err := bgClient.InstallOpenRestyWithLogger(logger.Log); err != nil {
			logger.Log(fmt.Sprintf("❌ 安装失败: %v", err))
			return
		}

		logger.Log("")
		logger.Log("安装完成！您可以关闭此窗口并刷新页面。")
	}()

	return map[string]interface{}{
		"installed":      false,
		"running":        false,
		"mode":           "installing",
		"native_running": nativeRunning,
		"native_version": nativeVersion,
		"session_id":     sessionID,
		"message":        "OpenResty安装任务已启动",
	}, nil
}

func handleInstallLogsAction(sessionID string) (interface{}, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("缺少session_id参数")
	}

	logger := GetInstallLogger(sessionID)
	if logger == nil {
		return map[string]interface{}{
			"logs":   []string{},
			"status": "not_found",
		}, nil
	}

	// 获取日志和完成状态
	logs := logger.GetLogs()
	completed := logger.IsCompleted()

	status := "running"
	if completed {
		status = "completed"
	}

	return map[string]interface{}{
		"logs":   logs,
		"status": status,
	}, nil
}

func handleCertificateContentAction(params map[string]interface{}) (interface{}, error) {
	certPath := getStringParam(params["certificate_path"])
	keyPath := getStringParam(params["key_path"])

	if certPath == "" && keyPath == "" {
		return nil, fmt.Errorf("缺少证书或私钥路径")
	}

	client, err := nginx.NewNginxClient(nil)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	result := make(map[string]interface{})

	if certPath != "" {
		hostPath, err := client.ResolveHostPath(certPath)
		if err != nil {
			return nil, fmt.Errorf("解析证书路径失败: %w", err)
		}
		data, err := os.ReadFile(hostPath)
		if err != nil {
			return nil, fmt.Errorf("读取证书失败: %w", err)
		}
		result["certificate"] = string(data)
		result["certificate_path"] = hostPath
	}

	if keyPath != "" {
		hostPath, err := client.ResolveHostPath(keyPath)
		if err != nil {
			return nil, fmt.Errorf("解析私钥路径失败: %w", err)
		}
		data, err := os.ReadFile(hostPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥失败: %w", err)
		}
		result["private_key"] = string(data)
		result["key_path"] = hostPath
	}

	return result, nil
}

func getStringParam(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func getStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		var items []string
		for _, item := range v {
			str := getStringParam(item)
			if str != "" {
				items = append(items, str)
			}
		}
		return items
	case string:
		if v == "" {
			return nil
		}
		// 允许通过逗号分隔
		parts := strings.Split(v, ",")
		var items []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				items = append(items, part)
			}
		}
		return items
	default:
		return nil
	}
}

func getStringMap(value interface{}) map[string]string {
	result := make(map[string]string)
	switch v := value.(type) {
	case map[string]string:
		return v
	case map[string]interface{}:
		for key, val := range v {
			if str := getStringParam(val); str != "" {
				result[key] = str
			}
		}
	}
	return result
}

func filterDomains(domains []string, primary string) []string {
	primaryLower := strings.ToLower(primary)
	seen := make(map[string]struct{})
	var filtered []string
	for _, d := range domains {
		value := strings.TrimSpace(d)
		if value == "" {
			continue
		}
		if strings.ToLower(value) == primaryLower {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return filtered
}

// scanRunningHTTPSServices 扫描正在运行的HTTPS服务的证书
func scanRunningHTTPSServices() ([]SSLCertificate, error) {
	certificates := []SSLCertificate{}

	// 扫描Nginx配置文件中的SSL证书配置
	nginxCerts, err := scanNginxSSLConfig()
	if err == nil {
		certificates = append(certificates, nginxCerts...)
	}

	// 可以添加其他Web服务器的扫描（Apache等）
	// apacheCerts, err := scanApacheSSLConfig()
	// if err == nil {
	//     certificates = append(certificates, apacheCerts...)
	// }

	return certificates, nil
}

// scanNginxSSLConfig 扫描Nginx配置文件中的SSL证书
func scanNginxSSLConfig() ([]SSLCertificate, error) {
	certificates := []SSLCertificate{}

	// 获取Nginx配置文件列表
	configs, err := GetNginxConfigsList()
	if err != nil {
		return nil, err
	}

	for _, config := range configs {
		// 读取配置文件内容
		content, err := GetNginxConfigContent(config.Path)
		if err != nil {
			continue
		}

		// 解析SSL证书路径
		certPaths := extractSSLCertPathsFromConfig(content)

		for _, certPath := range certPaths {
			// 检查证书文件是否存在
			if _, err := os.Stat(certPath); os.IsNotExist(err) {
				continue
			}

			// 解析证书
			cert, err := GetCertificateInfo(certPath)
			if err != nil {
				continue
			}

			// 寻找对应的私钥
			keyPath := findPrivateKeyForCert(certPath)

			// 构建证书信息
			sslCert := buildSSLCertificateFromX509(cert, certPath, keyPath, "nginx")
			certificates = append(certificates, sslCert)
		}
	}

	return certificates, nil
}

// extractSSLCertPathsFromConfig 从Nginx配置内容中提取SSL证书路径
func extractSSLCertPathsFromConfig(content string) []string {
	var certPaths []string

	// 正则表达式匹配ssl_certificate指令
	certRegex := regexp.MustCompile(`ssl_certificate\s+([^;]+);`)
	matches := certRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			path := strings.TrimSpace(match[1])
			// 移除引号
			path = strings.Trim(path, `"'`)
			certPaths = append(certPaths, path)
		}
	}

	return certPaths
}
