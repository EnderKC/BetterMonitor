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
	CPUUsage        float64 `json:"cpu_usage"`
	MemoryUsed      uint64  `json:"memory_used"`
	MemoryTotal     uint64  `json:"memory_total"`
	DiskUsed        uint64  `json:"disk_used"`
	DiskTotal       uint64  `json:"disk_total"`
	NetworkIn       float64 `json:"network_in"`        // 网络入站速率(bytes/s) - 用于展示曲线
	NetworkOut      float64 `json:"network_out"`       // 网络出站速率(bytes/s) - 用于展示曲线
	NetworkInDelta  uint64  `json:"network_in_delta"`  // 采样窗口内的入站字节增量(bytes) - 用于准确累加总量
	NetworkOutDelta uint64  `json:"network_out_delta"` // 采样窗口内的出站字节增量(bytes) - 用于准确累加总量
	SampleDuration  uint64  `json:"sample_duration"`   // 实际采样时长(ms) - 确保速率计算准确
	LoadAvg1        float64 `json:"load_avg_1"`
	LoadAvg5        float64 `json:"load_avg_5"`
	LoadAvg15       float64 `json:"load_avg_15"`
	SwapUsed        uint64  `json:"swap_used"`
	SwapTotal       uint64  `json:"swap_total"`
	BootTime        uint64  `json:"boot_time"`
	Latency         float64 `json:"latency"`         // 延迟(ms)
	PacketLoss      float64 `json:"packet_loss"`     // 丢包率(%)
	Processes       int     `json:"processes"`       // 进程数
	TCPConnections  int     `json:"tcp_connections"` // TCP连接数
	UDPConnections  int     `json:"udp_connections"` // UDP连接数
}

// Monitor 系统监控器
type Monitor struct {
	log       *logger.Logger
	serverURL string // 后端服务器URL，用于ping检测

	// 用于计算上报周期内的流量增量（准确的总流量统计）
	lastReportBytesRecv uint64    // 上次上报时的系统累计接收字节数
	lastReportBytesSent uint64    // 上次上报时的系统累计发送字节数
	lastReportTime      time.Time // 上次上报时间
	hasLastReport       bool      // 是否有上次上报的基线数据
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

	// 获取网络IO - 基于上报周期计算准确的流量增量和平均速率
	// 重要设计说明：
	// 1. Delta 必须覆盖整个上报周期（例如 30 秒），而不是短暂采样窗口（200ms）
	// 2. 使用系统累计计数器的差值，确保不会遗漏任何流量
	// 3. 速率基于上报周期的平均值，适合折线图展示
	var networkIn, networkOut float64 = 0, 0
	var networkInDelta, networkOutDelta uint64 = 0, 0
	var sampleDuration uint64 = 0

	// 获取当前系统网络累计计数器
	now := time.Now()
	netStats, err := net.IOCounters(false)
	if err != nil {
		m.log.Warn("获取网络IO信息失败: %v，本次上报流量数据为0", err)
		// 获取失败时，delta 和速率保持为 0
	} else if len(netStats) > 0 {
		currentBytesRecv := netStats[0].BytesRecv
		currentBytesSent := netStats[0].BytesSent

		// 检查是否有上次上报的基线数据
		if !m.hasLastReport {
			// 第一次上报：建立基线，但不产生 delta
			// 原因：没有"上一次"可以做差，delta 为 0 是正确的
			m.lastReportBytesRecv = currentBytesRecv
			m.lastReportBytesSent = currentBytesSent
			m.lastReportTime = now
			m.hasLastReport = true
			m.log.Info("初始化网络流量基线 (入站=%d B, 出站=%d B)", currentBytesRecv, currentBytesSent)
			// networkInDelta/OutDelta/sampleDuration 保持为 0
		} else {
			// 计算自上次上报以来的时间间隔
			reportInterval := now.Sub(m.lastReportTime)
			reportIntervalSec := reportInterval.Seconds()

			// 防御性检查：确保时间间隔合理
			if reportIntervalSec <= 0 {
				m.log.Warn("上报时间间隔异常 (<=0)，跳过本次流量计算")
				// 保持 delta 和速率为 0，但不更新基线
			} else if reportIntervalSec > 300 {
				// 超过 5 分钟，可能是断线重连或系统时钟问题
				m.log.Warn("上报时间间隔过长 (%.1f 秒)，重置基线避免异常大值", reportIntervalSec)
				// 重置基线，本次 delta 为 0
				m.lastReportBytesRecv = currentBytesRecv
				m.lastReportBytesSent = currentBytesSent
				m.lastReportTime = now
			} else {
				// 正常情况：计算上报周期内的流量增量
				sampleDuration = uint64(reportInterval / time.Millisecond)

				// 处理计数器回绕/重置
				// 说明：
				// - 操作系统网卡计数器在重启、接口重置、驱动问题时可能回退
				// - 如果检测到回退（当前值 < 上次值），本次 delta 置 0
				// - 同时重置基线到当前值，避免下次继续回退
				if currentBytesRecv >= m.lastReportBytesRecv {
					networkInDelta = currentBytesRecv - m.lastReportBytesRecv
				} else {
					networkInDelta = 0
					m.log.Warn("检测到入站计数器回退 (上次:%d 当前:%d)，可能是网卡重置，本次增量置0",
						m.lastReportBytesRecv, currentBytesRecv)
				}

				if currentBytesSent >= m.lastReportBytesSent {
					networkOutDelta = currentBytesSent - m.lastReportBytesSent
				} else {
					networkOutDelta = 0
					m.log.Warn("检测到出站计数器回退 (上次:%d 当前:%d)，可能是网卡重置，本次增量置0",
						m.lastReportBytesSent, currentBytesSent)
				}

				// 计算上报周期内的平均速率 (字节/秒)
				// 注意：这是平均值，不是瞬时值，但对于 30 秒周期的折线图已经足够平滑
				networkIn = float64(networkInDelta) / reportIntervalSec
				networkOut = float64(networkOutDelta) / reportIntervalSec

				// 更新基线到当前值（无论是否回退都要更新）
				m.lastReportBytesRecv = currentBytesRecv
				m.lastReportBytesSent = currentBytesSent
				m.lastReportTime = now

				m.log.Debug("网络IO统计: 周期=%.1fs, 入站增量=%d B (速率=%.2f B/s), 出站增量=%d B (速率=%.2f B/s)",
					reportIntervalSec, networkInDelta, networkIn, networkOutDelta, networkOut)
			}
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
		CPUUsage:        cpuUsage,
		MemoryUsed:      memoryUsed,
		MemoryTotal:     memoryTotal,
		DiskUsed:        diskUsed,
		DiskTotal:       diskTotal,
		NetworkIn:       networkIn,
		NetworkOut:      networkOut,
		NetworkInDelta:  networkInDelta,
		NetworkOutDelta: networkOutDelta,
		SampleDuration:  sampleDuration,
		LoadAvg1:        loadAvg1,
		LoadAvg5:        loadAvg5,
		LoadAvg15:       loadAvg15,
		SwapUsed:        swapUsed,
		SwapTotal:       swapTotal,
		BootTime:        bootTime,
		Latency:         latency,
		PacketLoss:      packetLoss,
		Processes:       processCount,
		TCPConnections:  tcpCount,
		UDPConnections:  udpCount,
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
