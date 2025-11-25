package monitor

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/user/server-ops-agent/pkg/logger"
)

// ProcessInfo 进程信息结构
type ProcessInfo struct {
	PID        int32    `json:"pid"`
	PPID       int32    `json:"ppid"`
	Name       string   `json:"name"`
	Username   string   `json:"username"`
	Status     string   `json:"status"`
	CPUPercent float64  `json:"cpu_percent"`
	MemoryRSS  uint64   `json:"memory_rss"`
	MemoryVMS  uint64   `json:"memory_vms"`
	CreateTime int64    `json:"create_time"`
	Cmd        string   `json:"cmd"`
	Ports      []string `json:"ports"`
	IsSystem   bool     `json:"is_system"`
}

// ProcessManager 进程管理器
type ProcessManager struct {
	log *logger.Logger
}

// NewProcessManager 创建一个新的进程管理器
func NewProcessManager(log *logger.Logger) *ProcessManager {
	return &ProcessManager{
		log: log,
	}
}

// GetProcessList 获取进程列表
func (pm *ProcessManager) GetProcessList() ([]*ProcessInfo, error) {
	pm.log.Debug("获取进程列表...")

	// 获取所有进程
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("获取进程列表失败: %w", err)
	}

	// 获取端口映射
	portMap, err := pm.getPortMap()
	if err != nil {
		pm.log.Warn("获取端口映射失败: %v", err)
	}

	// 获取系统进程标识
	systemProcs, err := pm.getSystemProcesses()
	if err != nil {
		pm.log.Warn("获取系统进程标识失败: %v", err)
	}

	// 构建进程信息列表
	var processList []*ProcessInfo
	for _, p := range procs {
		// 获取基本信息
		procInfo, err := pm.getProcessInfo(p)
		if err != nil {
			pm.log.Debug("获取进程 %d 信息失败，已跳过: %v", p.Pid, err)
			continue
		}

		// 添加端口信息
		if ports, ok := portMap[p.Pid]; ok {
			procInfo.Ports = ports
		}

		// 标记系统进程
		if pm.isSystemProcess(procInfo, systemProcs) {
			procInfo.IsSystem = true
		}

		processList = append(processList, procInfo)
	}

	pm.log.Debug("已获取 %d 个进程", len(processList))
	return processList, nil
}

// getProcessInfo 获取单个进程详细信息
func (pm *ProcessManager) getProcessInfo(p *process.Process) (*ProcessInfo, error) {
	// 创建进程信息对象
	info := &ProcessInfo{
		PID: p.Pid,
	}

	// 获取进程名称
	name, err := p.Name()
	if err != nil {
		name = "未知"
	}
	info.Name = name

	// 获取父进程PID
	ppid, err := p.Ppid()
	if err == nil {
		info.PPID = ppid
	}

	// 获取用户名
	username, err := p.Username()
	if err == nil {
		info.Username = username
	} else {
		info.Username = "未知"
	}

	// 获取进程状态
	status, err := p.Status()
	if err == nil && len(status) > 0 {
		info.Status = status[0]
	} else {
		info.Status = "未知"
	}

	// 获取CPU使用率
	cpuPercent, err := p.CPUPercent()
	if err == nil {
		info.CPUPercent = cpuPercent
	}

	// 获取内存使用
	memInfo, err := p.MemoryInfo()
	if err == nil && memInfo != nil {
		info.MemoryRSS = memInfo.RSS
		info.MemoryVMS = memInfo.VMS
	}

	// 获取创建时间
	createTime, err := p.CreateTime()
	if err == nil {
		info.CreateTime = createTime / 1000 // 转换为秒
	}

	// 获取命令行
	cmdline, err := p.Cmdline()
	if err == nil {
		info.Cmd = cmdline
	}

	return info, nil
}

// getPortMap 获取端口到进程的映射
func (pm *ProcessManager) getPortMap() (map[int32][]string, error) {
	result := make(map[int32][]string)

	// 根据不同操作系统获取端口映射
	switch runtime.GOOS {
	case "windows":
		return pm.getPortMapWindows()
	case "linux", "darwin":
		return pm.getPortMapLinux()
	default:
		return result, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// getPortMapWindows 获取Windows系统的端口映射
func (pm *ProcessManager) getPortMapWindows() (map[int32][]string, error) {
	result := make(map[int32][]string)

	// 执行netstat命令获取端口信息
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return result, fmt.Errorf("执行netstat命令失败: %w", err)
	}

	// 解析输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "LISTENING") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				// 解析进程ID
				pidStr := fields[4]
				pid, err := strconv.ParseInt(pidStr, 10, 32)
				if err != nil {
					continue
				}

				// 解析端口
				addrPort := fields[1]
				parts := strings.Split(addrPort, ":")
				if len(parts) < 2 {
					continue
				}
				port := parts[len(parts)-1]

				// 添加到结果中
				pid32 := int32(pid)
				if _, ok := result[pid32]; !ok {
					result[pid32] = []string{}
				}
				if !pm.containsPort(result[pid32], port) {
					result[pid32] = append(result[pid32], port)
				}
			}
		}
	}

	return result, nil
}

// getPortMapLinux 获取Linux/macOS系统的端口映射
func (pm *ProcessManager) getPortMapLinux() (map[int32][]string, error) {
	result := make(map[int32][]string)

	// 首先尝试使用lsof命令
	lsofResult := pm.tryGetPortMapWithLsof()
	if len(lsofResult) > 0 {
		pm.log.Debug("使用lsof命令成功获取到 %d 个进程的端口信息", len(lsofResult))
		return lsofResult, nil
	}

	// 如果lsof失败，尝试使用ss命令
	ssResult := pm.tryGetPortMapWithSS()
	if len(ssResult) > 0 {
		pm.log.Debug("使用ss命令成功获取到 %d 个进程的端口信息", len(ssResult))
		return ssResult, nil
	}

	// 如果ss也失败，尝试使用netstat命令
	netstatResult := pm.tryGetPortMapWithNetstat()
	if len(netstatResult) > 0 {
		pm.log.Debug("使用netstat命令成功获取到 %d 个进程的端口信息", len(netstatResult))
		return netstatResult, nil
	}

	pm.log.Warn("所有端口获取方法都失败了，将返回空的端口映射")
	return result, nil
}

// tryGetPortMapWithLsof 尝试使用lsof命令获取端口映射
func (pm *ProcessManager) tryGetPortMapWithLsof() map[int32][]string {
	result := make(map[int32][]string)

	pm.log.Debug("尝试使用lsof命令获取端口信息...")
	cmd := exec.Command("lsof", "-i", "-n", "-P")
	output, err := cmd.Output()
	if err != nil {
		pm.log.Warn("执行lsof命令失败: %v", err)
		return result
	}

	pm.log.Debug("lsof命令执行成功，输出长度: %d 字节", len(output))

	// 解析输出
	lines := strings.Split(string(output), "\n")
	pm.log.Debug("lsof输出共 %d 行", len(lines))

	for i, line := range lines {
		if i == 0 || line == "" { // 跳过标题行和空行
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		if strings.Contains(fields[7], "LISTEN") {
			// 解析进程ID
			pidStr := fields[1]
			pid, err := strconv.ParseInt(pidStr, 10, 32)
			if err != nil {
				continue
			}

			// 解析端口
			addrPort := fields[8]
			parts := strings.Split(addrPort, ":")
			if len(parts) < 2 {
				continue
			}
			port := parts[len(parts)-1]

			// 添加到结果中
			pid32 := int32(pid)
			if _, ok := result[pid32]; !ok {
				result[pid32] = []string{}
			}
			if !pm.containsPort(result[pid32], port) {
				result[pid32] = append(result[pid32], port)
			}
		}
	}

	pm.log.Debug("通过lsof解析得到 %d 个进程的端口信息", len(result))
	return result
}

// tryGetPortMapWithSS 尝试使用ss命令获取端口映射
func (pm *ProcessManager) tryGetPortMapWithSS() map[int32][]string {
	result := make(map[int32][]string)

	pm.log.Debug("尝试使用ss命令获取端口信息...")
	cmd := exec.Command("ss", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		pm.log.Warn("执行ss命令失败: %v", err)
		return result
	}

	pm.log.Debug("ss命令执行成功，输出长度: %d 字节", len(output))

	// 解析输出
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 || line == "" { // 跳过标题行和空行
			continue
		}

		if strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			if len(fields) < 6 {
				continue
			}

			// 解析端口 (在Local Address字段中)
			localAddr := fields[3]
			parts := strings.Split(localAddr, ":")
			if len(parts) < 2 {
				continue
			}
			port := parts[len(parts)-1]

			// 解析进程信息 (在最后一个字段中，格式为 users:(("进程名",pid=进程ID,fd=文件描述符)))
			if len(fields) >= 6 {
				processInfo := fields[5]
				pidMatch := regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(processInfo)
				if len(pidMatch) > 1 {
					if pid, err := strconv.ParseInt(pidMatch[1], 10, 32); err == nil {
						pid32 := int32(pid)
						if _, ok := result[pid32]; !ok {
							result[pid32] = []string{}
						}
						if !pm.containsPort(result[pid32], port) {
							result[pid32] = append(result[pid32], port)
						}
					}
				}
			}
		}
	}

	pm.log.Debug("通过ss解析得到 %d 个进程的端口信息", len(result))
	return result
}

// tryGetPortMapWithNetstat 尝试使用netstat命令获取端口映射
func (pm *ProcessManager) tryGetPortMapWithNetstat() map[int32][]string {
	result := make(map[int32][]string)

	pm.log.Debug("尝试使用netstat命令获取端口信息...")
	cmd := exec.Command("netstat", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		pm.log.Warn("执行netstat命令失败: %v", err)
		return result
	}

	pm.log.Debug("netstat命令执行成功，输出长度: %d 字节", len(output))

	// 解析输出
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if i == 0 || line == "" { // 跳过标题行和空行
			continue
		}

		if strings.Contains(line, "LISTEN") {
			fields := strings.Fields(line)
			if len(fields) < 7 {
				continue
			}

			// 解析端口 (在Local Address字段中)
			localAddr := fields[3]
			parts := strings.Split(localAddr, ":")
			if len(parts) < 2 {
				continue
			}
			port := parts[len(parts)-1]

			// 解析进程信息 (在最后一个字段中，格式为 进程ID/进程名)
			processInfo := fields[6]
			if processInfo != "-" {
				pidMatch := regexp.MustCompile(`^(\d+)/`).FindStringSubmatch(processInfo)
				if len(pidMatch) > 1 {
					if pid, err := strconv.ParseInt(pidMatch[1], 10, 32); err == nil {
						pid32 := int32(pid)
						if _, ok := result[pid32]; !ok {
							result[pid32] = []string{}
						}
						if !pm.containsPort(result[pid32], port) {
							result[pid32] = append(result[pid32], port)
						}
					}
				}
			}
		}
	}

	pm.log.Debug("通过netstat解析得到 %d 个进程的端口信息", len(result))
	return result
}

// containsPort 检查端口列表是否包含特定端口
func (pm *ProcessManager) containsPort(ports []string, port string) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}

// getSystemProcesses 获取系统进程列表
func (pm *ProcessManager) getSystemProcesses() (map[string]bool, error) {
	result := make(map[string]bool)

	// 常见系统进程名称
	systemProcNames := []string{
		"System", "systemd", "svchost", "smss", "csrss", "wininit", "services",
		"lsass", "winlogon", "spoolsv", "explorer", "dwm", "conhost",
		"fontdrvhost", "Registry", "devenv", "MsMpEng", "NisSrv", "Memory Compression",
		"powershell", "bash", "zsh", "fish", "launchd", "WindowServer", "kernel_task",
		"kthreadd", "kworker", "rcu_sched", "migration", "ksoftirqd", "kdevtmpfs",
		"kauditd", "khungtaskd", "oom_reaper", "writeback", "kcompactd", "init",
	}

	for _, name := range systemProcNames {
		result[strings.ToLower(name)] = true
	}

	return result, nil
}

// isSystemProcess 判断进程是否是系统进程
func (pm *ProcessManager) isSystemProcess(procInfo *ProcessInfo, systemProcs map[string]bool) bool {
	// 检查进程名称是否在系统进程列表中
	if _, ok := systemProcs[strings.ToLower(procInfo.Name)]; ok {
		return true
	}

	// 判断PID是否很小（通常系统进程PID较小）
	if procInfo.PID < 10 {
		return true
	}

	// 判断是否是Windows服务宿主进程
	if strings.Contains(strings.ToLower(procInfo.Name), "svchost") {
		return true
	}

	// 判断命令行是否包含系统路径
	cmdLower := strings.ToLower(procInfo.Cmd)
	systemPaths := []string{"/sbin/", "/bin/", "/usr/sbin/", "/usr/bin/", "\\windows\\system32\\"}
	for _, path := range systemPaths {
		if strings.Contains(cmdLower, strings.ToLower(path)) {
			return true
		}
	}

	return false
}

// KillProcess 终止进程
func (pm *ProcessManager) KillProcess(pid int32) error {
	pm.log.Info("尝试终止进程 %d...", pid)

	// 获取进程对象
	p, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("获取进程 %d 失败: %w", pid, err)
	}

	// 获取进程名称和命令行用于记录
	name, _ := p.Name()
	cmdline, _ := p.Cmdline()
	pm.log.Info("终止进程: PID=%d, 名称=%s, 命令行=%s", pid, name, cmdline)

	// 终止进程
	if err := p.Kill(); err != nil {
		// 如果使用正常方法终止失败，根据操作系统采用备用方法
		pm.log.Warn("使用正常方法终止进程 %d 失败: %v, 尝试使用强制方法", pid, err)

		var cmdErr error
		if runtime.GOOS == "windows" {
			// Windows使用taskkill强制结束进程
			cmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
			_, cmdErr = cmd.Output()
		} else {
			// Linux/macOS使用kill -9
			cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
			_, cmdErr = cmd.Output()
		}

		if cmdErr != nil {
			return fmt.Errorf("强制终止进程 %d 失败: %w", pid, cmdErr)
		}
	}

	pm.log.Info("成功终止进程 %d", pid)
	return nil
}

// 辅助方法：查找进程的socket连接
func (pm *ProcessManager) findProcessConnections(pid int32) ([]net.Conn, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}

	_, err = p.Connections()
	if err != nil {
		return nil, err
	}

	// 这里需要进一步转换gopsutil的connection为net.Conn，但gopsutil不直接提供这个功能
	// 实际使用中我们通常只需要端口信息，而不是连接对象本身
	// 为避免unused变量警告，使用匿名变量

	return nil, nil
}

// GetProcess 获取单个进程信息
func (pm *ProcessManager) GetProcess(pid int32) (*ProcessInfo, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("获取进程 %d 失败: %w", pid, err)
	}

	info, err := pm.getProcessInfo(p)
	if err != nil {
		return nil, err
	}

	// 获取端口信息
	portMap, _ := pm.getPortMap()
	if ports, ok := portMap[pid]; ok {
		info.Ports = ports
	}

	// 判断是否为系统进程
	systemProcs, _ := pm.getSystemProcesses()
	if pm.isSystemProcess(info, systemProcs) {
		info.IsSystem = true
	}

	return info, nil
}
