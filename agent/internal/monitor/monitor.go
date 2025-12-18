package monitor

import (
	"context"
	"fmt"
	"io"
	stdnet "net"
	"net/http"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/user/server-ops-agent/pkg/logger"
)

// SystemInfo 系统信息结构
type SystemInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	KernelVersion   string `json:"kernel_version"`
	KernelArch      string `json:"kernel_arch"`
	CPUModel        string `json:"cpu_model"`
	CPUCores        int    `json:"cpu_cores"`
	MemoryTotal     uint64 `json:"memory_total"`
	DiskTotal       uint64 `json:"disk_total"`
	BootTime        uint64 `json:"boot_time"`
	PublicIP        string `json:"public_ip"` // 出口IP
}

// MonitorData 监控数据结构
type MonitorData struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsed     uint64  `json:"memory_used"`
	MemoryTotal    uint64  `json:"memory_total"`
	DiskUsed       uint64  `json:"disk_used"`
	DiskTotal      uint64  `json:"disk_total"`
	NetworkIn      float64 `json:"network_in"`
	NetworkOut     float64 `json:"network_out"`
	LoadAvg1       float64 `json:"load_avg_1"`
	LoadAvg5       float64 `json:"load_avg_5"`
	LoadAvg15      float64 `json:"load_avg_15"`
	SwapUsed       uint64  `json:"swap_used"`
	SwapTotal      uint64  `json:"swap_total"`
	BootTime       uint64  `json:"boot_time"`
	Latency        float64 `json:"latency"`         // 延迟(ms)
	PacketLoss     float64 `json:"packet_loss"`     // 丢包率(%)
	Processes      int     `json:"processes"`       // 进程数
	TCPConnections int     `json:"tcp_connections"` // TCP连接数
	UDPConnections int     `json:"udp_connections"` // UDP连接数
}

// Monitor 系统监控器
type Monitor struct {
	log          *logger.Logger
	lastNetStats []net.IOCountersStat
	lastNetTime  time.Time
	serverURL    string // 后端服务器URL，用于ping检测
}

// New 创建一个新的监控器
func New(log *logger.Logger) *Monitor {
	return &Monitor{
		log: log,
	}
}

// SetServerURL 设置服务器URL用于延迟检测
func (m *Monitor) SetServerURL(url string) {
	m.serverURL = url
}

// GetPublicIP 获取出口IP地址
func (m *Monitor) GetPublicIP() string {
	ipv4 := m.getIP([]string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://api.ip.sb/ip",
	}, "tcp4")

	ipv6 := m.getIP([]string{
		"https://api6.ipify.org",
		"https://icanhazip.com",
		"https://api.ip.sb/ip",
	}, "tcp6")

	if ipv4 != "" && ipv6 != "" {
		return fmt.Sprintf("%s, %s", ipv4, ipv6)
	}
	if ipv4 != "" {
		return ipv4
	}
	return ipv6
}

func (m *Monitor) getIP(services []string, network string) string {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, netw, addr string) (stdnet.Conn, error) {
				return (&stdnet.Dialer{}).DialContext(ctx, network, addr)
			},
		},
		Timeout: 5 * time.Second,
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			m.log.Debug("从 %s 获取%s失败: %v", service, network, err)
			continue
		}
		// 注意：这里不能使用 defer resp.Body.Close()，因为是在循环中
		// 应该显式关闭

		if resp.StatusCode == 200 {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close() // 显式关闭
			if err != nil {
				m.log.Debug("读取%s响应失败: %v", network, err)
				continue
			}

			ip := string(body)
			// 去除可能的换行符和空格
			ip = strings.TrimSpace(ip)

			if ip != "" {
				m.log.Info("成功获取%s: %s (来源: %s)", network, ip, service)
				return ip
			}
		} else {
			resp.Body.Close()
		}
	}

	return ""
}

// MeasureLatency 测量到服务器的延迟和丢包率
func (m *Monitor) MeasureLatency() (latency float64, packetLoss float64) {
	if m.serverURL == "" {
		return 0, 0
	}

	// 发送3次ping请求
	const pingCount = 3
	var successCount int
	var totalLatency float64

	for i := 0; i < pingCount; i++ {
		start := time.Now()

		// 使用HTTP HEAD请求模拟ping
		client := &http.Client{
			Timeout: 2 * time.Second,
		}

		req, err := http.NewRequest("HEAD", m.serverURL, nil)
		if err != nil {
			m.log.Warn("创建ping请求失败: %v", err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			m.log.Debug("Ping请求失败 (第%d次): %v", i+1, err)
			continue
		}
		resp.Body.Close()

		// 计算延迟
		elapsed := time.Since(start)
		latencyMs := float64(elapsed.Milliseconds())
		totalLatency += latencyMs
		successCount++

		m.log.Debug("Ping成功 (第%d次): %.2f ms", i+1, latencyMs)

		// 避免请求过快
		if i < pingCount-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// 计算平均延迟
	if successCount > 0 {
		latency = totalLatency / float64(successCount)
	}

	// 计算丢包率
	packetLoss = float64(pingCount-successCount) / float64(pingCount) * 100

	m.log.Debug("延迟检测结果: 平均延迟=%.2f ms, 丢包率=%.2f%%", latency, packetLoss)

	return latency, packetLoss
}

// GetSystemInfo 获取系统信息
func (m *Monitor) GetSystemInfo() (*SystemInfo, error) {
	m.log.Debug("获取系统信息...")

	// 获取主机信息
	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("获取主机信息失败: %w", err)
	}

	// 获取CPU信息 - 使用更健壮的错误处理
	var cpuModel string = "Unknown CPU"
	var cpuCores int = 1

	cpuInfo, err := cpu.Info()
	if err != nil {
		m.log.Warn("获取CPU详细信息失败: %v，将使用默认值", err)
	} else if len(cpuInfo) > 0 {
		cpuModel = cpuInfo[0].ModelName
		cpuCores, err = cpu.Counts(false)
		if err != nil {
			m.log.Warn("获取CPU核心数失败: %v，将使用默认值", err)
		}
		// Windows可能会返回0个核心，在这种情况下尝试通过其他方式获取
		if cpuCores == 0 {
			physicalCores, err := cpu.Counts(false)
			if err != nil {
				cpuCores = 1 // 默认值
				m.log.Warn("获取CPU核心数失败: %v，将使用默认值", err)
			} else {
				cpuCores = physicalCores
			}
		}
	} else {
		// 尝试获取CPU数量
		physicalCores, err := cpu.Counts(false)
		if err != nil {
			cpuCores = 1 // 默认值
			m.log.Warn("获取CPU核心数失败: %v，将使用默认值", err)
		} else {
			cpuCores = physicalCores
		}
	}

	// 获取内存信息
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("获取内存信息失败: %w", err)
	}

	// 获取磁盘总量信息 - 获取根目录磁盘信息
	var diskTotal uint64 = 0
	diskInfo, rootPath, err := diskUsageForHost(hostInfo)
	if err != nil {
		m.log.Warn("获取磁盘信息失败: %v，将使用默认值", err)
	} else {
		diskTotal = diskInfo.Total
		m.log.Debug("系统信息磁盘总量: %d (路径: %s)", diskTotal, rootPath)
	}

	// 获取公网IP
	publicIP := m.GetPublicIP()

	return &SystemInfo{
		Hostname:        hostInfo.Hostname,
		OS:              hostInfo.OS,
		Platform:        hostInfo.Platform,
		PlatformVersion: hostInfo.PlatformVersion,
		KernelVersion:   hostInfo.KernelVersion,
		KernelArch:      hostInfo.KernelArch,
		CPUModel:        cpuModel,
		CPUCores:        cpuCores,
		MemoryTotal:     memInfo.Total,
		DiskTotal:       diskTotal,
		BootTime:        hostInfo.BootTime,
		PublicIP:        publicIP,
	}, nil
}

// GetMonitorData 获取监控数据
func (m *Monitor) GetMonitorData() (*MonitorData, error) {
	m.log.Debug("收集最新监控数据...")

	// 获取CPU使用率 - 使用0作为间隔参数会返回自系统启动以来的平均值
	// 我们使用一个很短的时间窗口来获取更即时的数据
	var cpuUsage float64 = 0
	cpuPercent, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil {
		m.log.Warn("获取CPU使用率失败: %v，将使用默认值", err)
	} else if len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
		m.log.Debug("CPU使用率: %.2f%%", cpuUsage)
	}

	// 获取内存使用率 - 每次都获取最新数据
	var memoryUsed uint64 = 0
	var memoryTotal uint64 = 0
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		m.log.Warn("获取内存信息失败: %v，将使用默认值", err)
	} else {
		memoryUsed = memInfo.Used
		memoryTotal = memInfo.Total
		m.log.Debug("内存信息: 总量=%d 使用=%d 空闲=%d 使用率=%.2f%%",
			memInfo.Total, memInfo.Used, memInfo.Free, memInfo.UsedPercent)
	}

	// 预先获取主机信息，便于重复使用
	hostInfo, hostInfoErr := host.Info()
	if hostInfoErr != nil {
		m.log.Warn("获取主机系统信息失败: %v，将使用默认根目录", hostInfoErr)
	}

	// 获取磁盘使用率 - 每次都获取最新数据
	var diskUsed uint64 = 0
	var diskTotal uint64 = 0
	diskInfo, rootPath, err := diskUsageForHost(hostInfo)
	if err != nil {
		m.log.Warn("获取磁盘信息失败: %v，将使用默认值", err)
	} else {
		if hostInfo != nil {
			m.log.Debug("当前系统平台: %s, OS: %s", hostInfo.Platform, hostInfo.OS)
		}
		m.log.Debug("获取磁盘信息，路径: %s", rootPath)
		diskUsed = diskInfo.Used
		diskTotal = diskInfo.Total
		m.log.Debug("磁盘信息: 总量=%d 使用=%d 空闲=%d 使用率=%.2f%%",
			diskInfo.Total, diskInfo.Used, diskInfo.Free, diskInfo.UsedPercent)
	}

	// 获取负载信息 - 每次都获取最新数据
	var loadAvg1, loadAvg5, loadAvg15 float64 = 0, 0, 0

	loadStat, err := load.Avg()
	if err != nil {
		m.log.Warn("获取系统负载失败: %v，将使用默认值", err)
	} else {
		loadAvg1 = loadStat.Load1
		loadAvg5 = loadStat.Load5
		loadAvg15 = loadStat.Load15
		m.log.Debug("系统负载: 1分钟=%.2f 5分钟=%.2f 15分钟=%.2f",
			loadAvg1, loadAvg5, loadAvg15)
	}

	// 获取网络IO - 需要两次采样计算速率
	// 这里添加一个小的延迟来获取更准确的网络速率
	var networkIn, networkOut float64 = 0, 0

	// 第一次采样
	netStats1, err := net.IOCounters(false)
	if err != nil {
		m.log.Warn("获取网络IO信息失败: %v，将使用默认值或历史值", err)

		// 尝试使用历史值
		if len(m.lastNetStats) > 0 && !m.lastNetTime.IsZero() {
			timeDiff := time.Since(m.lastNetTime).Seconds()
			if timeDiff > 0 && timeDiff < 60 { // 只在合理的时间范围内使用历史值
				networkIn = float64(m.lastNetStats[0].BytesRecv) / timeDiff
				networkOut = float64(m.lastNetStats[0].BytesSent) / timeDiff
			}
		}
	} else if len(netStats1) > 0 {
		// 小延迟后再次采样
		time.Sleep(200 * time.Millisecond)

		// 第二次采样
		netStats2, err := net.IOCounters(false)
		if err != nil {
			m.log.Warn("获取第二次网络IO信息失败: %v", err)
		} else if len(netStats2) > 0 {
			// 计算速率 (字节/秒)
			timeDiff := 0.2 // 200ms = 0.2s
			bytesIn := netStats2[0].BytesRecv - netStats1[0].BytesRecv
			bytesOut := netStats2[0].BytesSent - netStats1[0].BytesSent
			networkIn = float64(bytesIn) / timeDiff
			networkOut = float64(bytesOut) / timeDiff

			// 更新上次的网络信息，用于下次可能的历史值计算
			m.lastNetStats = netStats2
			m.lastNetTime = time.Now()

			m.log.Debug("网络IO: 入站=%.2f 字节/秒, 出站=%.2f 字节/秒",
				networkIn, networkOut)
		}
	}

	// 获取Swap信息
	var swapUsed uint64 = 0
	var swapTotal uint64 = 0
	swapInfo, err := mem.SwapMemory()
	if err != nil {
		m.log.Warn("获取Swap信息失败: %v", err)
	} else {
		swapUsed = swapInfo.Used
		swapTotal = swapInfo.Total
	}

	// 获取启动时间
	var bootTime uint64 = 0
	bootInfo := hostInfo
	if bootInfo == nil {
		if info, err := host.Info(); err == nil {
			bootInfo = info
		} else {
			m.log.Warn("获取系统启动时间失败: %v", err)
		}
	}

	if bootInfo != nil {
		bootTime = bootInfo.BootTime
		m.log.Debug("系统启动时间: %d", bootTime)
	}

	// 测量延迟和丢包率
	latency, packetLoss := m.MeasureLatency()

	// 获取进程数
	var processCount int = 0
	procs, err := process.Processes()
	if err != nil {
		m.log.Warn("获取进程列表失败: %v", err)
		// 尝试使用备用方法统计进程数
		processCount = 0
	} else {
		processCount = len(procs)
		m.log.Debug("进程数: %d", processCount)
	}

	// 获取TCP/UDP连接数 - 分别获取以提高稳定性
	var tcpCount int = 0
	var udpCount int = 0

	// 先尝试获取TCP连接
	tcpConnections, err := net.Connections("tcp")
	if err != nil {
		m.log.Warn("获取TCP连接失败: %v", err)
	} else {
		tcpCount = len(tcpConnections)
		m.log.Debug("TCP连接数: %d", tcpCount)
	}

	// 再获取UDP连接
	udpConnections, err := net.Connections("udp")
	if err != nil {
		m.log.Warn("获取UDP连接失败: %v", err)
	} else {
		udpCount = len(udpConnections)
		m.log.Debug("UDP连接数: %d", udpCount)
	}

	// 构造监控数据
	return &MonitorData{
		CPUUsage:       cpuUsage,
		MemoryUsed:     memoryUsed,
		MemoryTotal:    memoryTotal,
		DiskUsed:       diskUsed,
		DiskTotal:      diskTotal,
		NetworkIn:      networkIn,
		NetworkOut:     networkOut,
		LoadAvg1:       loadAvg1,
		LoadAvg5:       loadAvg5,
		LoadAvg15:      loadAvg15,
		SwapUsed:       swapUsed,
		SwapTotal:      swapTotal,
		BootTime:       bootTime,
		Latency:        latency,
		PacketLoss:     packetLoss,
		Processes:      processCount,
		TCPConnections: tcpCount,
		UDPConnections: udpCount,
	}, nil
}

func resolveRootPath(info *host.InfoStat) string {
	if info != nil && strings.EqualFold(info.OS, "windows") {
		return "C:\\"
	}
	return "/"
}

func diskUsageForHost(info *host.InfoStat) (*disk.UsageStat, string, error) {
	path := resolveRootPath(info)
	usage, err := disk.Usage(path)
	return usage, path, err
}
