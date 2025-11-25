package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/user/server-ops-agent/pkg/logger"
)

func TestNewMonitor(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 验证监控器创建成功
	assert.NotNil(t, monitor)
	assert.Equal(t, logger, monitor.log)
	assert.Empty(t, monitor.lastNetStats)
	assert.True(t, monitor.lastNetTime.IsZero())
}

func TestGetSystemInfo(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 获取系统信息
	systemInfo, err := monitor.GetSystemInfo()
	
	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, systemInfo)
	
	// 验证基本字段
	assert.NotEmpty(t, systemInfo.Hostname)
	assert.NotEmpty(t, systemInfo.OS)
	assert.NotEmpty(t, systemInfo.Platform)
	assert.NotEmpty(t, systemInfo.KernelArch)
	assert.Greater(t, systemInfo.CPUCores, 0)
	assert.Greater(t, systemInfo.MemoryTotal, uint64(0))
	assert.GreaterOrEqual(t, systemInfo.DiskTotal, uint64(0))
	assert.Greater(t, systemInfo.BootTime, uint64(0))
	
	// 验证CPU信息
	assert.NotEmpty(t, systemInfo.CPUModel)
	assert.GreaterOrEqual(t, systemInfo.CPUCores, 1)
}

func TestGetMonitorData(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 获取监控数据
	monitorData, err := monitor.GetMonitorData()
	
	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, monitorData)
	
	// 验证CPU使用率
	assert.GreaterOrEqual(t, monitorData.CPUUsage, 0.0)
	assert.LessOrEqual(t, monitorData.CPUUsage, 100.0)
	
	// 验证内存信息
	assert.GreaterOrEqual(t, monitorData.MemoryUsed, uint64(0))
	assert.Greater(t, monitorData.MemoryTotal, uint64(0))
	assert.LessOrEqual(t, monitorData.MemoryUsed, monitorData.MemoryTotal)
	
	// 验证磁盘信息
	assert.GreaterOrEqual(t, monitorData.DiskUsed, uint64(0))
	assert.GreaterOrEqual(t, monitorData.DiskTotal, uint64(0))
	
	// 验证网络信息
	assert.GreaterOrEqual(t, monitorData.NetworkIn, 0.0)
	assert.GreaterOrEqual(t, monitorData.NetworkOut, 0.0)
	
	// 验证系统负载
	assert.GreaterOrEqual(t, monitorData.LoadAvg1, 0.0)
	assert.GreaterOrEqual(t, monitorData.LoadAvg5, 0.0)
	assert.GreaterOrEqual(t, monitorData.LoadAvg15, 0.0)
}

func TestGetMonitorDataConsistency(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 多次获取监控数据
	data1, err1 := monitor.GetMonitorData()
	assert.NoError(t, err1)
	assert.NotNil(t, data1)
	
	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)
	
	data2, err2 := monitor.GetMonitorData()
	assert.NoError(t, err2)
	assert.NotNil(t, data2)
	
	// 验证数据一致性
	assert.Equal(t, data1.MemoryTotal, data2.MemoryTotal)
	assert.Equal(t, data1.DiskTotal, data2.DiskTotal)
	
	// CPU使用率可能会有变化，但应该在合理范围内
	assert.GreaterOrEqual(t, data1.CPUUsage, 0.0)
	assert.LessOrEqual(t, data1.CPUUsage, 100.0)
	assert.GreaterOrEqual(t, data2.CPUUsage, 0.0)
	assert.LessOrEqual(t, data2.CPUUsage, 100.0)
}

func TestMonitorDataStructure(t *testing.T) {
	// 测试监控数据结构
	data := &MonitorData{
		CPUUsage:    25.5,
		MemoryUsed:  1024 * 1024 * 1024,  // 1GB
		MemoryTotal: 4 * 1024 * 1024 * 1024, // 4GB
		DiskUsed:    10 * 1024 * 1024 * 1024, // 10GB
		DiskTotal:   100 * 1024 * 1024 * 1024, // 100GB
		NetworkIn:   1024.0,
		NetworkOut:  2048.0,
		LoadAvg1:    1.5,
		LoadAvg5:    1.2,
		LoadAvg15:   0.8,
	}
	
	// 验证结构字段
	assert.Equal(t, 25.5, data.CPUUsage)
	assert.Equal(t, uint64(1024*1024*1024), data.MemoryUsed)
	assert.Equal(t, uint64(4*1024*1024*1024), data.MemoryTotal)
	assert.Equal(t, uint64(10*1024*1024*1024), data.DiskUsed)
	assert.Equal(t, uint64(100*1024*1024*1024), data.DiskTotal)
	assert.Equal(t, 1024.0, data.NetworkIn)
	assert.Equal(t, 2048.0, data.NetworkOut)
	assert.Equal(t, 1.5, data.LoadAvg1)
	assert.Equal(t, 1.2, data.LoadAvg5)
	assert.Equal(t, 0.8, data.LoadAvg15)
}

func TestSystemInfoStructure(t *testing.T) {
	// 测试系统信息结构
	info := &SystemInfo{
		Hostname:        "test-host",
		OS:              "linux",
		Platform:        "ubuntu",
		PlatformVersion: "20.04",
		KernelVersion:   "5.4.0",
		KernelArch:      "x86_64",
		CPUModel:        "Intel Core i5",
		CPUCores:        4,
		MemoryTotal:     8 * 1024 * 1024 * 1024, // 8GB
		DiskTotal:       500 * 1024 * 1024 * 1024, // 500GB
		BootTime:        1640995200, // 2022-01-01 00:00:00 UTC
	}
	
	// 验证结构字段
	assert.Equal(t, "test-host", info.Hostname)
	assert.Equal(t, "linux", info.OS)
	assert.Equal(t, "ubuntu", info.Platform)
	assert.Equal(t, "20.04", info.PlatformVersion)
	assert.Equal(t, "5.4.0", info.KernelVersion)
	assert.Equal(t, "x86_64", info.KernelArch)
	assert.Equal(t, "Intel Core i5", info.CPUModel)
	assert.Equal(t, 4, info.CPUCores)
	assert.Equal(t, uint64(8*1024*1024*1024), info.MemoryTotal)
	assert.Equal(t, uint64(500*1024*1024*1024), info.DiskTotal)
	assert.Equal(t, uint64(1640995200), info.BootTime)
}

func TestNetworkStatsCaching(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 第一次获取监控数据
	data1, err1 := monitor.GetMonitorData()
	assert.NoError(t, err1)
	assert.NotNil(t, data1)
	
	// 验证网络统计信息被缓存
	assert.False(t, monitor.lastNetTime.IsZero())
	
	// 再次获取监控数据
	data2, err2 := monitor.GetMonitorData()
	assert.NoError(t, err2)
	assert.NotNil(t, data2)
	
	// 验证网络数据有效
	assert.GreaterOrEqual(t, data2.NetworkIn, 0.0)
	assert.GreaterOrEqual(t, data2.NetworkOut, 0.0)
}

// 集成测试：验证监控器在多次调用时的稳定性
func TestMonitorStability(t *testing.T) {
	// 创建真实的日志记录器
	logger, err := logger.New("", "info")
	assert.NoError(t, err)
	
	// 创建监控器
	monitor := New(logger)
	
	// 多次调用获取监控数据
	for i := 0; i < 10; i++ {
		data, err := monitor.GetMonitorData()
		assert.NoError(t, err)
		assert.NotNil(t, data)
		
		// 验证数据有效性
		assert.GreaterOrEqual(t, data.CPUUsage, 0.0)
		assert.LessOrEqual(t, data.CPUUsage, 100.0)
		assert.Greater(t, data.MemoryTotal, uint64(0))
		assert.GreaterOrEqual(t, data.MemoryUsed, uint64(0))
		assert.LessOrEqual(t, data.MemoryUsed, data.MemoryTotal)
		
		// 短暂等待
		time.Sleep(50 * time.Millisecond)
	}
	
	// 验证系统信息的稳定性
	for i := 0; i < 5; i++ {
		info, err := monitor.GetSystemInfo()
		assert.NoError(t, err)
		assert.NotNil(t, info)
		
		// 验证系统信息的基本字段
		assert.NotEmpty(t, info.Hostname)
		assert.NotEmpty(t, info.OS)
		assert.Greater(t, info.CPUCores, 0)
		assert.Greater(t, info.MemoryTotal, uint64(0))
	}
}